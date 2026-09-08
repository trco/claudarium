package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/tidwall/gjson"
)

func TestOlderThan(t *testing.T) {
	old := time.Now().Add(-10 * 24 * time.Hour).Format(time.RFC3339)
	recent := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	if !olderThan(old, 3) {
		t.Error("10 days ago should be older than 3 days")
	}
	if olderThan(recent, 3) {
		t.Error("1 hour ago should not be older than 3 days")
	}
	if olderThan("not-a-date", 3) {
		t.Error("unparseable timestamp should be treated as not-old")
	}
}

func TestSaveFile_BacksUpThenWrites(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(p, []byte("old"), 0o600); err != nil {
		t.Fatal(err)
	}
	bak, err := SaveFile(p, []byte("new"))
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "new" {
		t.Errorf("file = %q, want new", got)
	}
	if got, _ := os.ReadFile(bak); string(got) != "old" {
		t.Errorf("backup = %q, want old", got)
	}
}

func TestLineDiff(t *testing.T) {
	d := LineDiff("a\nb\nc", "a\nx\nc")
	if !HasChanges(d) {
		t.Fatal("expected changes")
	}
	var del, add, ctx bool
	for _, l := range d {
		switch {
		case l.Op == "-" && l.Text == "b":
			del = true
		case l.Op == "+" && l.Text == "x":
			add = true
		case l.Op == " " && l.Text == "a":
			ctx = true
		}
	}
	if !del || !add || !ctx {
		t.Errorf("diff missing lines: %+v", d)
	}
	if HasChanges(LineDiff("same", "same")) {
		t.Error("identical input should report no changes")
	}
}

func TestCapabilitiesAndMCP_FromFixtures(t *testing.T) {
	tmp := t.TempDir()
	old := Home
	Home = func() string { return tmp }
	defer func() { Home = old }()

	writeFile(t, filepath.Join(tmp, ".claude/agents/foo.md"), "---\nname: foo\ndescription: does foo\n---\nbody")
	writeFile(t, filepath.Join(tmp, ".claude/skills/bar/SKILL.md"), "---\nname: bar\ndescription: does bar\n---\nbody")
	writeFile(t, filepath.Join(tmp, ".claude.json"), `{
  "mcpServers": { "db": {"command":"psql","args":["-h","local"],"env":{"PGPASSWORD":"secret","PGUSER":"me"}} },
  "projects": { "/repo": {"mcpServers":{"api":{"type":"http","url":"https://x"}}, "disabledMcpServers":["api"]} }
}`)

	caps := Capabilities()
	if !hasCap(caps, "foo", "agent", "does foo") {
		t.Errorf("missing foo agent: %+v", caps)
	}
	if !hasCap(caps, "bar", "skill", "does bar") {
		t.Errorf("missing bar skill: %+v", caps)
	}

	servers := MCPServers()
	db := findServer(servers, "db")
	if db == nil {
		t.Fatalf("db server not found: %+v", servers)
	}
	if db.Kind != "stdio" || db.Target != "psql -h local" {
		t.Errorf("db server wrong: %+v", db)
	}
	if strings.Contains(strings.Join(db.EnvKeys, ","), "secret") {
		t.Errorf("secret leaked into env keys: %v", db.EnvKeys)
	}
	if len(db.EnvKeys) != 2 || db.EnvKeys[0] != "PGPASSWORD" {
		t.Errorf("env keys wrong (want sorted names only): %v", db.EnvKeys)
	}
	api := findServer(servers, "api")
	if api == nil || api.Scope != "repo:/repo" || api.Enabled {
		t.Errorf("api server wrong (want disabled repo server): %+v", api)
	}
}

func TestRepoPaths_DiscoversUnusedSiblings(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()

	work := filepath.Join(home, "work")
	used := filepath.Join(work, "usedrepo")     // recorded in projects
	unused := filepath.Join(work, "unusedrepo") // never opened in Claude
	writeFile(t, filepath.Join(used, ".claude/agents/used.md"), "---\ndescription: u\n---\n")
	writeFile(t, filepath.Join(unused, ".claude/commands/unused.md"), "---\ndescription: n\n---\n")
	writeFile(t, filepath.Join(home, ".claude.json"), `{"projects":{"`+used+`":{}}}`)

	paths := RepoPaths()
	if !contains(paths, used) {
		t.Errorf("recorded repo missing: %v", paths)
	}
	if !contains(paths, unused) {
		t.Errorf("unused sibling not discovered: %v", paths)
	}

	caps := Capabilities()
	if !hasSource(caps, "unused", "repo:"+unused) {
		t.Errorf("unused repo capability not listed: %+v", caps)
	}
}

func TestCapabilities_IncludesFilePath(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()
	writeFile(t, filepath.Join(home, ".claude/agents/foo.md"), "---\ndescription: d\n---\n")
	writeFile(t, filepath.Join(home, ".claude.json"), `{"projects":{}}`)

	for _, c := range Capabilities() {
		if c.Name == "foo" {
			if !strings.HasSuffix(c.Path, filepath.Join("agents", "foo.md")) {
				t.Errorf("foo.Path = %q, want …/agents/foo.md", c.Path)
			}
			return
		}
	}
	t.Fatal("foo capability not found")
}

func TestHealthChecks(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()

	writeFile(t, filepath.Join(home, ".claude/settings.json"),
		`{"permissions":{"allow":["Read(x)","Read(x)"]},"enabledPlugins":{"ghost@mkt":true}}`)
	writeFile(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"),
		`{"plugins":{"real@mkt":[{"scope":"user","version":"1"}]}}`)
	writeFile(t, filepath.Join(home, ".claude.json"),
		`{"projects":{"/no/such/repo":{"mcpServers":{"srv":{"command":"definitely-not-real-binary-xyz-123"}}}}}`)

	issues := HealthChecks()
	want := []struct{ level, sub string }{
		{"warn", "duplicate rule in allow"},
		{"error", "enabled but not installed: ghost@mkt"},
		{"info", "installed but not enabled: real@mkt"},
		{"warn", "missing repo"},
		{"warn", "command not found"},
	}
	for _, w := range want {
		if !hasIssue(issues, w.level, w.sub) {
			t.Errorf("missing %s issue containing %q; got %+v", w.level, w.sub, issues)
		}
	}
}

func TestHealthChecks_ExtraChecks(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()

	proj := filepath.Join(home, "proj")
	writeFile(t, filepath.Join(home, ".claude/settings.json"),
		`{"enabledPlugins":{"foo@ghostmkt":true},"statusLine":{"command":"definitely-not-real-binary-xyz-999"}}`)
	writeFile(t, filepath.Join(home, ".claude/agents/dup.md"), "---\ndescription: g\n---\n")
	writeFile(t, filepath.Join(proj, ".claude/agents/dup.md"), "---\ndescription: r\n---\n")
	writeFile(t, filepath.Join(home, ".claude.json"),
		`{"mcpServers":{"srv":{"command":"echo"}},"projects":{"`+proj+`":{"mcpServers":{"srv":{"command":"echo"}}}}}`)

	issues := HealthChecks()
	for _, w := range []struct{ level, sub string }{
		{"warn", "unknown marketplace: ghostmkt"},
		{"warn", "statusLine command not found"},
		{"info", "agent dup defined in multiple sources"},
		{"info", "srv defined in multiple scopes"},
	} {
		if !hasIssue(issues, w.level, w.sub) {
			t.Errorf("missing %s issue containing %q; got %+v", w.level, w.sub, issues)
		}
	}
}

func TestSetPluginEnabled(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()

	sp := filepath.Join(home, ".claude/settings.json")
	writeFile(t, sp, `{"enabledPlugins":{"foo@mkt":true}}`)

	if err := SetPluginEnabled("foo@mkt", false); err != nil {
		t.Fatal(err)
	}
	if cur, _ := os.ReadFile(sp); !strings.Contains(string(cur), `"foo@mkt":false`) {
		t.Errorf("want foo@mkt=false, got %s", cur)
	}
	if err := SetPluginEnabled("bar@mkt", true); err != nil {
		t.Fatal(err)
	}
	if cur, _ := os.ReadFile(sp); !strings.Contains(string(cur), `"bar@mkt":true`) {
		t.Errorf("want bar@mkt=true, got %s", cur)
	}
}

func TestSetMCPEnabled(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()

	cj := filepath.Join(home, ".claude.json")
	writeFile(t, cj, `{"mcpServers":{"g":{"command":"echo"}},"projects":{"/repo":{"mcpServers":{"r":{"command":"echo"}}}}}`)

	// Global disable: moved out of mcpServers into the stash, not deleted.
	if err := SetMCPEnabled("global", "g", false); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(cj)
	if gjson.GetBytes(b, "mcpServers.g").Exists() {
		t.Error("g should be moved out of mcpServers")
	}
	if !gjson.GetBytes(b, globalDisabledMCPKey+".g").Exists() {
		t.Error("g should be stashed, not deleted")
	}
	if s := findServer(MCPServers(), "g"); s == nil || s.Enabled {
		t.Errorf("g should still be listed as disabled: %+v", s)
	}
	// Global enable: moved back.
	if err := SetMCPEnabled("global", "g", true); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(cj)
	if !gjson.GetBytes(b, "mcpServers.g").Exists() {
		t.Error("g should be restored to mcpServers")
	}

	// Repo disable: added to disabledMcpServers; the definition is preserved.
	if err := SetMCPEnabled("repo:/repo", "r", false); err != nil {
		t.Fatal(err)
	}
	b, _ = os.ReadFile(cj)
	if !gjson.GetBytes(b, `projects./repo.mcpServers.r`).Exists() {
		t.Error("r definition should be preserved")
	}
	if s := findServer(MCPServers(), "r"); s == nil || s.Enabled {
		t.Errorf("r should be disabled: %+v", s)
	}
	// Repo enable: removed from the list.
	if err := SetMCPEnabled("repo:/repo", "r", true); err != nil {
		t.Fatal(err)
	}
	if s := findServer(MCPServers(), "r"); s == nil || !s.Enabled {
		t.Errorf("r should be enabled again: %+v", s)
	}
}

func TestAuditLog(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()

	mem := filepath.Join(home, ".claude", "CLAUDE.md")
	writeFile(t, mem, "v2\n")             // current memory
	writeFile(t, mem+".bak.1000", "v1\n") // snapshot before the edit

	AppendAudit("plugin", "Enabled plugin foo@mkt", "")
	AppendAudit("memory", "Edited CLAUDE.md", mem+".bak.1000")
	AppendAudit("mcp", "Turned MCP srv on (global)", "")

	es := AuditEntries()
	if len(es) != 3 {
		t.Fatalf("want 3 entries, got %d: %+v", len(es), es)
	}
	if es[0].Kind != "mcp" || es[2].Kind != "plugin" {
		t.Errorf("want newest-first (mcp .. plugin), got %s .. %s", es[0].Kind, es[2].Kind)
	}
	// the memory entry carries a diff (v1 -> current v2)
	memEntry := es[1]
	if memEntry.Kind != "memory" || (memEntry.Adds == 0 && memEntry.Dels == 0) {
		t.Errorf("memory entry should have a diff: %+v", memEntry)
	}

	// delete the memory entry -> its backup is removed too
	if err := DeleteAuditEntry(memEntry.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(mem + ".bak.1000"); !os.IsNotExist(err) {
		t.Error("deleting the memory entry should remove its .bak file")
	}
	if len(AuditEntries()) != 2 {
		t.Errorf("want 2 entries after delete, got %d", len(AuditEntries()))
	}

	// clear everything
	if _, err := ClearAudit(); err != nil {
		t.Fatal(err)
	}
	if len(AuditEntries()) != 0 {
		t.Error("audit log should be empty after clear")
	}
}

func hasIssue(issues []Issue, level, sub string) bool {
	for _, i := range issues {
		if i.Level == level && strings.Contains(i.Message, sub) {
			return true
		}
	}
	return false
}

func contains(s []string, want string) bool {
	for _, v := range s {
		if v == want {
			return true
		}
	}
	return false
}

func hasSource(caps []Capability, name, source string) bool {
	for _, c := range caps {
		if c.Name == name && c.Source == source {
			return true
		}
	}
	return false
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func hasCap(caps []Capability, name, kind, desc string) bool {
	for _, c := range caps {
		if c.Name == name && c.Kind == kind && c.Description == desc {
			return true
		}
	}
	return false
}

func findServer(servers []MCPServer, name string) *MCPServer {
	for i := range servers {
		if servers[i].Name == name {
			return &servers[i]
		}
	}
	return nil
}

func TestParseSession(t *testing.T) {
	p := filepath.Join(t.TempDir(), "abc.jsonl")
	writeFile(t, p, strings.Join([]string{
		`{"type":"user","timestamp":"2026-08-19T10:00:00.000Z","cwd":"/repo/x","gitBranch":"main","message":{"role":"user","content":"Fix the runner crash\nmore detail"}}`,
		`{"type":"assistant","timestamp":"2026-08-19T10:00:05.000Z","message":{"id":"m1","model":"claude-opus-4-8","content":[{"type":"text","text":"ok"},{"type":"tool_use","name":"Bash"}],"usage":{"input_tokens":1000,"output_tokens":500,"cache_creation_input_tokens":200,"cache_read_input_tokens":4000}}}`,
		`{"type":"user","timestamp":"2026-08-19T10:01:00.000Z","message":{"role":"user","content":[{"type":"tool_result","content":"done"}]}}`,
		`{"type":"assistant","timestamp":"2026-08-19T10:12:00.000Z","message":{"id":"m2","model":"claude-opus-4-8","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":100,"output_tokens":50}}}`,
	}, "\n"))
	s, err := parseSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.ID != "abc" || s.Project != "/repo/x" || s.Branch != "main" || s.Repo != "x" {
		t.Errorf("meta wrong: %+v", s)
	}
	if s.Title != "Fix the runner crash" || s.Prompt != s.Title {
		t.Errorf("title = %q prompt = %q, want first line of first prompt", s.Title, s.Prompt)
	}
	if s.Prompts != 1 || s.AssistantMsgs != 2 || s.ToolCalls != 1 {
		t.Errorf("counts wrong: prompts=%d (tool results must not count) assistant=%d tools=%d", s.Prompts, s.AssistantMsgs, s.ToolCalls)
	}
	if want := int64(1000 + 500 + 200 + 4000 + 100 + 50); s.Tokens != want {
		t.Errorf("tokens = %d, want %d", s.Tokens, want)
	}
	if s.Model != "claude-opus-4-8" || s.Cost <= 0 {
		t.Errorf("model/cost wrong: %s %f", s.Model, s.Cost)
	}
	if s.DurationFmt != "12m" {
		t.Errorf("active time = %q, want 12m", s.DurationFmt)
	}
	if len(s.Daily) != 1 {
		t.Errorf("daily buckets = %d, want 1 (all on one day)", len(s.Daily))
	}
}

func TestParseSession_BillsEachResponseOnce(t *testing.T) {
	// Claude Code writes one line per content block, each repeating the response usage.
	p := filepath.Join(t.TempDir(), "d.jsonl")
	line := `{"type":"assistant","timestamp":"2026-08-19T10:00:0%d.000Z","message":{"id":"same","model":"claude-opus-5","content":[{"type":"%s"}],"usage":{"input_tokens":10,"output_tokens":20,"cache_read_input_tokens":1000}}}`
	writeFile(t, p, strings.Join([]string{
		fmt.Sprintf(line, 1, "thinking"), fmt.Sprintf(line, 2, "tool_use"), fmt.Sprintf(line, 3, "text"),
	}, "\n"))
	s, err := parseSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.AssistantMsgs != 1 || s.Tokens != 1030 {
		t.Errorf("want 1 response billed once (1030 tokens), got %d responses / %d tokens", s.AssistantMsgs, s.Tokens)
	}
	if s.ToolCalls != 1 {
		t.Errorf("tool calls = %d, want 1 (blocks are still counted per line)", s.ToolCalls)
	}
}

func TestParseSession_ActiveTimeSkipsIdleGaps(t *testing.T) {
	p := filepath.Join(t.TempDir(), "g.jsonl")
	u := `{"type":"user","timestamp":"%s","message":{"role":"user","content":"hi"}}`
	a := `{"type":"assistant","timestamp":"%s","message":{"id":"%s","model":"claude-opus-5","content":[],"usage":{"input_tokens":100}}}`
	writeFile(t, p, strings.Join([]string{
		fmt.Sprintf(u, "2026-08-18T10:00:00Z"),
		fmt.Sprintf(a, "2026-08-18T10:05:00Z", "a"), // +5m
		fmt.Sprintf(u, "2026-08-19T10:00:00Z"),      // a day idle: not counted, new day
		fmt.Sprintf(a, "2026-08-19T10:10:00Z", "b"), // +10m
	}, "\n"))
	s, err := parseSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.ActiveTime != 15*time.Minute {
		t.Errorf("active time = %s, want 15m (idle gap excluded)", s.ActiveTime)
	}
	if len(s.Daily) != 2 {
		t.Errorf("usage should land on both days it happened, got %v", s.Daily)
	}
}

func TestParseSession_SubagentsBillToParent(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "parent.jsonl")
	writeFile(t, p, `{"type":"user","timestamp":"2026-08-19T10:00:00Z","cwd":"/r","message":{"role":"user","content":"do it"}}`+"\n"+
		`{"type":"assistant","timestamp":"2026-08-19T10:00:01Z","message":{"id":"p1","model":"claude-opus-5","content":[],"usage":{"input_tokens":100}}}`)
	writeFile(t, filepath.Join(dir, "parent", "subagents", "agent-1.jsonl"),
		`{"type":"user","timestamp":"2026-08-19T10:00:02Z","message":{"role":"user","content":"sub prompt"}}`+"\n"+
			`{"type":"assistant","timestamp":"2026-08-19T10:00:03Z","message":{"id":"s1","model":"claude-haiku-4-5","content":[],"usage":{"input_tokens":900}}}`)
	s, err := parseSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Tokens != 1000 || s.AssistantMsgs != 2 {
		t.Errorf("sub-agent usage should bill to the parent: tokens=%d responses=%d", s.Tokens, s.AssistantMsgs)
	}
	if s.Prompts != 1 || s.Title != "do it" {
		t.Errorf("sub-agent prompts must not count as yours: prompts=%d title=%q", s.Prompts, s.Title)
	}
	if s.Model != "claude-opus-5" {
		t.Errorf("model should come from the main transcript, got %q", s.Model)
	}
}

func TestParseSession_TitleSkipsHarnessWrappers(t *testing.T) {
	p := filepath.Join(t.TempDir(), "w.jsonl")
	writeFile(t, p, strings.Join([]string{
		`{"type":"user","timestamp":"2026-08-19T10:00:00Z","cwd":"/r","message":{"role":"user","content":"<command-name>/model</command-name><command-message>model</command-message><command-args>opus</command-args>"}}`,
		`{"type":"user","timestamp":"2026-08-19T10:00:01Z","message":{"role":"user","content":[{"type":"text","text":"<ide_selection>The user selected lines 1 to 2</ide_selection>\n<some_new_tag>x</some_new_tag>why is the build failing?"}]}}`,
	}, "\n"))
	s, err := parseSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "why is the build failing?" {
		t.Errorf("title = %q, want the first real prompt", s.Title)
	}
	if s.Prompts != 1 {
		t.Errorf("prompts = %d, want 1 (a slash command isn't a prompt)", s.Prompts)
	}
	// a session that only ever ran a slash command shows the command as its title
	q := filepath.Join(t.TempDir(), "c.jsonl")
	writeFile(t, q, `{"type":"user","timestamp":"2026-08-19T10:00:00Z","message":{"role":"user","content":"<command-name>/model</command-name><command-message>model</command-message>"}}`)
	if c, _ := parseSession(q); c.Title != "/model" || c.Prompts != 0 {
		t.Errorf("command-only session: title=%q prompts=%d, want /model and 0", c.Title, c.Prompts)
	}
}

func TestPriceFor(t *testing.T) {
	for _, id := range []string{"claude-opus-4-8", "claude-haiku-4-5-20251001", "sonnet", "haiku", "opus"} {
		if _, ok := priceFor(id); !ok {
			t.Errorf("%s should resolve to a price", id)
		}
	}
	if _, ok := priceFor("<synthetic>"); ok {
		t.Error("<synthetic> must not price")
	}
}

func TestRepoName(t *testing.T) {
	if got := repoName("/Users/me/github/foo/.claude/worktrees/fix-auth"); got != "foo" {
		t.Errorf("worktree cwd should roll up to its repo, got %q", got)
	}
	if got := repoName("/Users/me/github/foo"); got != "foo" {
		t.Errorf("plain cwd = %q", got)
	}
}

func TestUsageOf(t *testing.T) {
	day := func(d int) string { return time.Now().AddDate(0, 0, -d).Format(dayLayout) }
	sessions := []Session{
		{Repo: "a", Model: "claude-opus-5", Tokens: 300, Cost: 3, Daily: map[string]DayUsage{day(0): {100, 1}, day(10): {200, 2}}},
		{Repo: "b", Model: "claude-opus-5", Tokens: 50, Cost: 0.5, Daily: map[string]DayUsage{day(40): {50, 0.5}}},
	}
	u := UsageOf(sessions)
	today, week, month := u.Windows[0], u.Windows[1], u.Windows[2]
	if today.Tokens != 100 || today.Sessions != 1 {
		t.Errorf("today = %+v, want 100 tokens / 1 session", today)
	}
	if week.Tokens != 100 || month.Tokens != 300 || month.Sessions != 1 {
		t.Errorf("week=%+v month=%+v", week, month)
	}
	if len(u.ByDay) != 2 {
		t.Errorf("by-day should only hold the last 30 days, got %d rows", len(u.ByDay))
	}
	if u.ByProject[0].Key != "a" || u.ByProject[0].Tokens != 300 || len(u.ByProject) != 2 {
		t.Errorf("by-project = %+v", u.ByProject)
	}
}

func TestParseWorktreeList(t *testing.T) {
	raw := "worktree /r\nHEAD aaaaaaaaaaaa\nbranch refs/heads/main\n\nworktree /r/.claude/worktrees/wt\nHEAD bbbbbbbbbbbb\nbranch refs/heads/feat/x\n\nworktree /tmp/other\nHEAD cccccccccccc\ndetached\n"
	// invoked from the linked worktree: git still lists the main checkout first
	got := parseWorktreeList("/r/.claude/worktrees/wt", raw)
	if len(got) != 2 {
		t.Fatalf("want 2 linked worktrees (main skipped), got %+v", got)
	}
	if got[0].Path != "/r/.claude/worktrees/wt" || got[0].Branch != "feat/x" || got[0].Head != "bbbbbbb" || !got[0].Claude {
		t.Errorf("first = %+v", got[0])
	}
	if !got[1].Detached || got[1].Claude {
		t.Errorf("second = %+v", got[1])
	}
}

func TestTranscriptPaths_NestedAndDeduped(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()
	p := filepath.Join(home, ".claude", "projects")
	writeFile(t, filepath.Join(p, "a", "x.jsonl"), "")
	writeFile(t, filepath.Join(p, "a", "b", "y.jsonl"), "")                    // nested project dir
	writeFile(t, filepath.Join(p, "a", "x", "subagents", "agent-1.jsonl"), "") // belongs to x, not a session
	writeFile(t, filepath.Join(p, "a", "z.jsonl"), "")                         // duplicate id, older copy
	writeFile(t, filepath.Join(p, "a", "b", "z.jsonl"), "")                    // duplicate id, newer copy
	oldT := time.Now().Add(-time.Hour)
	os.Chtimes(filepath.Join(p, "a", "z.jsonl"), oldT, oldT)

	got := transcriptPaths()
	if len(got) != 3 {
		t.Fatalf("want x, y, z (nested found, sub-agent excluded, duplicate collapsed); got %v", got)
	}
	if !contains(got, filepath.Join(p, "a", "b", "z.jsonl")) {
		t.Errorf("duplicate id should resolve to the newest copy, got %v", got)
	}
}

func TestParseSession_TitlePrecedence(t *testing.T) {
	p := filepath.Join(t.TempDir(), "t.jsonl")
	writeFile(t, p, strings.Join([]string{
		`{"type":"user","timestamp":"2026-08-19T10:00:00Z","cwd":"/r","message":{"role":"user","content":"look at ticket 42"}}`,
		`{"type":"ai-title","aiTitle":"Investigate ticket 42"}`,
		`{"type":"custom-title","customTitle":"Support ticket: Draft"}`,
		`{"type":"custom-title","customTitle":"Support ticket: LinkedInPersonal"}`,
	}, "\n"))
	s, err := parseSession(p)
	if err != nil {
		t.Fatal(err)
	}
	if s.Title != "Support ticket: LinkedInPersonal" {
		t.Errorf("title = %q, want the latest rename", s.Title)
	}
	if s.Prompt != "look at ticket 42" {
		t.Errorf("first prompt should be kept separately, got %q", s.Prompt)
	}
	q := filepath.Join(t.TempDir(), "u.jsonl")
	writeFile(t, q, `{"type":"user","timestamp":"2026-08-19T10:00:00Z","message":{"role":"user","content":"hi"}}`+"\n"+`{"type":"ai-title","aiTitle":"Claude's name"}`)
	if c, _ := parseSession(q); c.Title != "Claude's name" {
		t.Errorf("without a rename, Claude's title wins over the prompt; got %q", c.Title)
	}
}

func TestLiveSessionIDs(t *testing.T) {
	home := t.TempDir()
	old := Home
	Home = func() string { return home }
	defer func() { Home = old }()
	writeFile(t, filepath.Join(home, ".claude", "sessions", "1.json"), fmt.Sprintf(`{"pid":%d,"sessionId":"live"}`, os.Getpid()))
	writeFile(t, filepath.Join(home, ".claude", "sessions", "2.json"), `{"pid":2147483000,"sessionId":"dead"}`)
	live := liveSessionIDs()
	if !live["live"] || live["dead"] {
		t.Errorf("want only the session whose process is alive, got %v", live)
	}
}

func TestSessionBefore_RunningFirstThenNewest(t *testing.T) {
	d := func(day int) time.Time { return time.Date(2026, 9, day, 0, 0, 0, 0, time.UTC) }
	in := []Session{
		{ID: "old-idle", Started: d(1)},
		{ID: "new-idle", Started: d(8)},
		{ID: "old-live", Started: d(2), Active: true},
		{ID: "new-live", Started: d(7), Active: true},
	}
	sort.Slice(in, func(i, j int) bool { return sessionBefore(in[i], in[j]) })
	var got []string
	for _, s := range in {
		got = append(got, s.ID)
	}
	if want := "new-live old-live new-idle old-idle"; strings.Join(got, " ") != want {
		t.Errorf("order = %v, want %s", got, want)
	}
}
