package config

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Session is one Claude Code session, indexed from its transcript
// (~/.claude/projects/<cwd>/<id>.jsonl) plus the sub-agent transcripts it
// spawned (<id>/subagents/*.jsonl), which bill to it.
type Session struct {
	ID       string
	Path     string // transcript path
	Project  string // cwd the session ran in
	Repo     string `json:"-"` // repo name; a Claude worktree maps to its repo
	Branch   string
	Title    string    // your rename, else Claude's title, else the first prompt
	Prompt   string    `json:"First prompt"`
	Model    string    // model of the last assistant turn
	Started  time.Time `json:"-"`
	Ended    time.Time `json:"-"`
	Active   bool      // activity in the last 5 minutes
	Worktree bool      // ran inside a .claude/worktrees checkout

	Prompts       int // messages you typed (tool results excluded)
	AssistantMsgs int // distinct API responses
	ToolCalls     int
	InputTokens   int64
	OutputTokens  int64
	CacheWrite    int64
	CacheRead     int64
	Tokens        int64               `json:"-"` // everything billed: in + out + cache write + cache read
	Cost          float64             `json:"-"` // USD, approximate (see prices)
	ActiveTime    time.Duration       `json:"-"` // time spent; idle gaps over 30 min don't count
	Daily         map[string]DayUsage `json:"-"` // billed usage per local calendar day

	// display — what the drawer shows instead of the raw fields above
	StartedFmt  string `json:"Started"`
	EndedFmt    string `json:"Ended"`
	DurationFmt string `json:"Active time"`
	TokensFmt   string `json:"Tokens"`
	CostFmt     string `json:"Cost"`
}

// DayUsage is one session's billed usage on one day.
type DayUsage struct {
	Tokens int64
	Cost   float64
}

// ---- pricing (USD per MTok) ----
//
// Source: Anthropic API price list (cached 2026-06-24). Cache write is 1.25x
// input (5-min ephemeral), cache read 0.1x input — except Fable 5.1, whose
// cache reads are $0.25. Costs are an estimate; edit freely.
type modelPrice struct{ In, Out, CacheWrite, CacheRead float64 }

var prices = map[string]modelPrice{
	"claude-fable-5-1":  {10, 50, 12.5, 0.25},
	"claude-fable-5":    {10, 50, 12.5, 1.0},
	"claude-opus-5":     {5, 25, 6.25, 0.5},
	"claude-opus-4-8":   {5, 25, 6.25, 0.5},
	"claude-opus-4-7":   {5, 25, 6.25, 0.5},
	"claude-opus-4-6":   {5, 25, 6.25, 0.5},
	"claude-sonnet-5":   {2, 10, 2.5, 0.2},
	"claude-sonnet-4-6": {3, 15, 3.75, 0.3},
	"claude-haiku-4-5":  {1, 5, 1.25, 0.1},
}

// Bare aliases Claude Code writes for sub-agent / model overrides.
var modelAliases = map[string]string{"opus": "claude-opus-5", "sonnet": "claude-sonnet-5", "haiku": "claude-haiku-4-5"}

// priceFor resolves aliases, then exact ids, then dated variants like
// claude-haiku-4-5-20251001 by longest key prefix.
func priceFor(model string) (modelPrice, bool) {
	if a, ok := modelAliases[model]; ok {
		model = a
	}
	if p, ok := prices[model]; ok {
		return p, true
	}
	best := ""
	for k := range prices {
		if strings.HasPrefix(model, k) && len(k) > len(best) {
			best = k
		}
	}
	if best == "" {
		return modelPrice{}, false
	}
	return prices[best], true
}

func cost(model string, in, out, cw, cr int64) float64 {
	p, ok := priceFor(model)
	if !ok {
		return 0
	}
	return (float64(in)*p.In + float64(out)*p.Out + float64(cw)*p.CacheWrite + float64(cr)*p.CacheRead) / 1e6
}

// ---- transcript parsing ----

// tRecord is the subset of a transcript line we care about.
type tRecord struct {
	Type        string `json:"type"`
	Timestamp   string `json:"timestamp"`
	Cwd         string `json:"cwd"`
	GitBranch   string `json:"gitBranch"`
	Summary     string `json:"summary"`
	CustomTitle string `json:"customTitle"` // type=custom-title: what you renamed the session to
	AITitle     string `json:"aiTitle"`     // type=ai-title: Claude's own name for it
	IsMeta      bool   `json:"isMeta"`
	Message     struct {
		ID      string          `json:"id"`
		Model   string          `json:"model"`
		Content json.RawMessage `json:"content"`
		Usage   struct {
			Input      int64 `json:"input_tokens"`
			Output     int64 `json:"output_tokens"`
			CacheWrite int64 `json:"cache_creation_input_tokens"`
			CacheRead  int64 `json:"cache_read_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

type tBlock struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

const (
	maxLine = 64 << 20         // transcripts can carry multi-MB tool results on one line
	idleGap = 30 * time.Minute // a longer pause between records doesn't count as time spent
)

// ponytail: decode only the fields we read, and cache by (size, mtime) so a
// page load re-parses nothing that hasn't changed.
func parseSession(path string) (Session, error) {
	st := &parseState{
		s:    Session{ID: strings.TrimSuffix(filepath.Base(path), ".jsonl"), Path: path, Daily: map[string]DayUsage{}},
		seen: map[string]bool{},
	}
	if err := st.ingest(path, true); err != nil {
		return Session{}, err
	}
	for _, sub := range subagentPaths(path) {
		if err := st.ingest(sub, false); err != nil {
			log.Printf("sessions: %s: %v", sub, err)
		}
	}
	return st.finish(), nil
}

// subagentPaths lists the sub-agent transcripts spawned by a session.
func subagentPaths(transcript string) []string {
	subs, _ := filepath.Glob(filepath.Join(strings.TrimSuffix(transcript, ".jsonl"), "subagents", "*.jsonl"))
	return subs
}

type parseState struct {
	s          Session
	seen       map[string]bool // message.id already billed
	custom, ai string          // last custom-title / ai-title seen (later records win)
	summary    string
	fallback   string // a slash command, used as the title only if nothing was typed
}

// ingest folds one transcript into the session. Only the main transcript
// contributes cwd/branch/prompts/title/active time; sub-agents bill usage.
func (st *parseState) ingest(path string, main bool) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	s := &st.s
	var prev time.Time

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), maxLine)
	for sc.Scan() {
		var r tRecord
		if json.Unmarshal(sc.Bytes(), &r) != nil {
			continue
		}
		if main {
			if r.Cwd != "" && s.Project == "" {
				s.Project = r.Cwd
			}
			if r.GitBranch != "" && s.Branch == "" {
				s.Branch = r.GitBranch
			}
		}
		var t time.Time
		if r.Timestamp != "" {
			if tt, err := time.Parse(time.RFC3339Nano, r.Timestamp); err == nil {
				t = tt
				if s.Started.IsZero() || t.Before(s.Started) {
					s.Started = t
				}
				if t.After(s.Ended) {
					s.Ended = t
				}
				if main {
					if gap := t.Sub(prev); !prev.IsZero() && gap > 0 && gap < idleGap {
						s.ActiveTime += gap
					}
					prev = t
				}
			}
		}
		switch r.Type {
		case "summary":
			if main && r.Summary != "" {
				st.summary = r.Summary
			}
		case "custom-title":
			if main && r.CustomTitle != "" {
				st.custom = r.CustomTitle
			}
		case "ai-title":
			if main && r.AITitle != "" {
				st.ai = r.AITitle
			}
		case "user":
			if !main || r.IsMeta {
				continue
			}
			raw := rawText(r.Message.Content)
			if raw == "" {
				continue // tool results only
			}
			if text := cleanTitle(raw); text != "" {
				s.Prompts++
				if s.Prompt == "" {
					s.Prompt = text
				}
			} else if st.fallback == "" {
				st.fallback = commandName(raw)
			}
		case "assistant":
			// Claude Code writes one line per content block of a response, each
			// repeating that response's usage — bill every message.id once.
			if id := r.Message.ID; id == "" || !st.seen[id] {
				st.seen[id] = true
				s.AssistantMsgs++
				u := r.Message.Usage
				s.InputTokens += u.Input
				s.OutputTokens += u.Output
				s.CacheWrite += u.CacheWrite
				s.CacheRead += u.CacheRead
				if m := r.Message.Model; m != "" && m != "<synthetic>" {
					if main {
						s.Model = m
					}
					c := cost(m, u.Input, u.Output, u.CacheWrite, u.CacheRead)
					s.Cost += c
					if !t.IsZero() {
						d := s.Daily[t.Local().Format(dayLayout)]
						d.Tokens += u.Input + u.Output + u.CacheWrite + u.CacheRead
						d.Cost += c
						s.Daily[t.Local().Format(dayLayout)] = d
					}
				}
			}
			var blocks []tBlock
			if json.Unmarshal(r.Message.Content, &blocks) == nil {
				for _, b := range blocks {
					if b.Type == "tool_use" {
						s.ToolCalls++
					}
				}
			}
		}
	}
	return sc.Err()
}

func (st *parseState) finish() Session {
	s := st.s
	s.Title = first(st.custom, st.ai, st.summary, s.Prompt, st.fallback, "(no prompt)")
	if s.Project == "" {
		s.Project = filepath.Base(filepath.Dir(s.Path))
	}
	s.Worktree = isClaudeWorktree(s.Project)
	s.Repo = repoName(s.Project)
	s.Tokens = s.InputTokens + s.OutputTokens + s.CacheWrite + s.CacheRead
	s.StartedFmt = s.Started.Local().Format(minuteLayout)
	s.EndedFmt = s.Ended.Local().Format(minuteLayout)
	s.DurationFmt = fmtDuration(s.ActiveTime)
	s.TokensFmt = fmtTokens(s.Tokens)
	s.CostFmt = fmtCost(s.Cost)
	return s
}

// repoName is the repo a cwd belongs to: for <repo>/.claude/worktrees/<wt>
// that's <repo>, so worktree sessions roll up to their project.
func repoName(cwd string) string {
	marker := string(filepath.Separator) + filepath.Join(".claude", "worktrees") + string(filepath.Separator)
	if i := strings.Index(cwd, marker); i >= 0 {
		return filepath.Base(cwd[:i])
	}
	return filepath.Base(cwd)
}

// rawText returns the human-typed text of a user message — a plain string, or
// the first text block — untouched. Tool-result-only messages yield "".
func rawText(raw json.RawMessage) string {
	var str string
	if json.Unmarshal(raw, &str) == nil {
		return str
	}
	var blocks []tBlock
	if json.Unmarshal(raw, &blocks) == nil {
		for _, b := range blocks {
			if b.Type == "text" && strings.TrimSpace(b.Text) != "" {
				return b.Text
			}
		}
	}
	return ""
}

// Claude Code prefixes prompts with harness context (IDE selection, slash
// command wrappers, system reminders). Known wrappers are stripped by name (they
// may nest other tags); anything else tag-shaped is stripped generically.
var (
	knownWrapperRes = func() []*regexp.Regexp {
		var out []*regexp.Regexp
		for _, tag := range []string{"system-reminder", "ide_selection", "ide_opened_file", "command-name", "command-message", "command-args", "local-command-stdout", "local-command-stderr", "local-command-caveat", "task-notification", "bash-stdout", "bash-stderr"} {
			out = append(out, regexp.MustCompile(`(?s)<`+tag+`>.*?</`+tag+`>`))
		}
		return out
	}()
	anyTagBlockRe = regexp.MustCompile(`(?s)<[a-z][a-z0-9_-]*>.*?</[a-z][a-z0-9_-]*>`)
	commandNameRe = regexp.MustCompile(`<command-name>(.*?)</command-name>`)
)

// commandName returns the slash command a message wraps, e.g. "/model", or "".
func commandName(s string) string {
	if m := commandNameRe.FindStringSubmatch(s); m != nil {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// cleanTitle strips harness wrappers and returns the first real line, or "".
func cleanTitle(s string) string {
	for _, re := range knownWrapperRes {
		s = re.ReplaceAllString(s, "")
	}
	s = anyTagBlockRe.ReplaceAllString(s, "")
	for _, line := range strings.Split(s, "\n") {
		if l := strings.TrimSpace(line); l != "" {
			if r := []rune(l); len(r) > 140 {
				l = string(r[:140]) + "…"
			}
			return l
		}
	}
	return ""
}

// ---- index + cache ----

// sig identifies a transcript plus its sub-agent files; any change re-parses.
type sig struct {
	size  int64
	mtime time.Time
}

type cachedSession struct {
	sig sig
	s   Session
}

var (
	sessMu    sync.Mutex
	sessCache = map[string]cachedSession{}
)

// first returns the first non-empty string.
func first(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// liveSessionIDs reads ~/.claude/sessions/<pid>.json — one per running Claude
// process — and returns the session ids whose process is still alive.
func liveSessionIDs() map[string]bool {
	out := map[string]bool{}
	files, _ := filepath.Glob(filepath.Join(ClaudeDir(), "sessions", "*.json"))
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			continue
		}
		var meta struct {
			PID       int    `json:"pid"`
			SessionID string `json:"sessionId"`
		}
		if json.Unmarshal(b, &meta) != nil || meta.SessionID == "" {
			continue
		}
		if err := syscall.Kill(meta.PID, 0); meta.PID > 0 && err != nil && err != syscall.EPERM {
			continue // the process is gone; the file is stale
		}
		out[meta.SessionID] = true
	}
	return out
}

// SessionsDir is where Claude Code keeps transcripts.
func SessionsDir() string { return filepath.Join(ClaudeDir(), "projects") }

// transcriptPaths lists every session transcript under projects/ at any
// depth — Claude nests project dirs (…/<cwd>/<other-cwd>/<id>.jsonl) — while
// skipping sub-agent files, which hang off their session. If the same session
// id exists at two paths, the most recently modified copy wins.
func transcriptPaths() []string {
	newest := map[string]string{}
	newestAt := map[string]time.Time{}
	filepath.WalkDir(SessionsDir(), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if d.Name() == "subagents" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		id := strings.TrimSuffix(d.Name(), ".jsonl")
		if at, ok := newestAt[id]; !ok || info.ModTime().After(at) {
			newest[id], newestAt[id] = p, info.ModTime()
		}
		return nil
	})
	out := make([]string, 0, len(newest))
	for _, p := range newest {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func signatureOf(transcript string) (sig, bool) {
	fi, err := os.Stat(transcript)
	if err != nil {
		return sig{}, false
	}
	sg := sig{fi.Size(), fi.ModTime()}
	for _, sub := range subagentPaths(transcript) {
		if fi, err := os.Stat(sub); err == nil {
			sg.size += fi.Size()
			if fi.ModTime().After(sg.mtime) {
				sg.mtime = fi.ModTime()
			}
		}
	}
	return sg, true
}

// SessionCount is the number of transcripts on disk (cheap — no parsing).
func SessionCount() int { return len(transcriptPaths()) }

// Sessions returns every session: running ones first, then the rest, each
// newest-started first. Unchanged
// transcripts come from the cache; new/changed ones are parsed 8 at a time.
// The cache is rebuilt each call, so deleted transcripts drop out.
func Sessions() []Session {
	sessMu.Lock()
	defer sessMu.Unlock()

	type job struct {
		path string
		sig  sig
	}
	next := map[string]cachedSession{}
	var out []Session
	var todo []job
	for _, p := range transcriptPaths() {
		sg, ok := signatureOf(p)
		if !ok {
			continue
		}
		if c, ok := sessCache[p]; ok && c.sig == sg {
			next[p] = c
			out = append(out, c.s)
			continue
		}
		todo = append(todo, job{p, sg})
	}

	parsed := make([]cachedSession, len(todo))
	okay := make([]bool, len(todo))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, j := range todo {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, j job) {
			defer wg.Done()
			defer func() { <-sem }()
			s, err := parseSession(j.path)
			if err != nil {
				log.Printf("sessions: skipping %s: %v", j.path, err)
				return
			}
			parsed[i] = cachedSession{sig: j.sig, s: s}
			okay[i] = true
		}(i, j)
	}
	wg.Wait()
	for i, j := range todo {
		if okay[i] {
			next[j.path] = parsed[i]
			out = append(out, parsed[i].s)
		}
	}
	sessCache = next

	live := liveSessionIDs()
	for i := range out {
		out[i].Active = live[out[i].ID] || (!out[i].Ended.IsZero() && time.Since(out[i].Ended) < 5*time.Minute)
	}
	sort.Slice(out, func(i, j int) bool { return sessionBefore(out[i], out[j]) })
	return out
}

// sessionBefore orders running sessions first, then everything else — each
// group newest-started first.
func sessionBefore(a, b Session) bool {
	if a.Active != b.Active {
		return a.Active
	}
	return a.Started.After(b.Started)
}

// Warm builds the session and worktree indexes; run once at startup in a
// goroutine so the first page load doesn't pay for them.
func Warm() {
	Sessions()
	Worktrees()
}

// ---- formatting ----

func fmtTokens(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%dk", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

func fmtCost(c float64) string {
	if c == 0 {
		return "—"
	}
	return fmt.Sprintf("≈ $%.2f", c)
}

func fmtDuration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	h, m := int(d.Hours()), int(d.Minutes())%60
	switch {
	case h > 0:
		return fmt.Sprintf("%dh %02dm", h, m)
	case m > 0:
		return fmt.Sprintf("%dm", m)
	default:
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
}
