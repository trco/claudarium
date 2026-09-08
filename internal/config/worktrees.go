package config

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Worktree is one linked git worktree of a known repo, plus the Claude
// sessions that ran inside it.
type Worktree struct {
	Repo           string // main checkout path
	Path           string
	Branch         string
	Head           string // short sha
	Detached       bool
	Dirty          int    // changed files (git status --porcelain lines)
	LastCommit     string // relative, e.g. "2 hours ago"
	Subject        string // last commit subject
	Claude         bool   // created by Claude Code (<repo>/.claude/worktrees/…)
	Stale          bool   // git lists it but the directory is gone
	Sessions       int
	LastActive     string // most recent session activity in this worktree
	LastActiveUnix int64  `json:"-"`
}

var (
	wtMu         sync.Mutex
	wtCache      []Worktree
	wtAt         time.Time
	wtRefreshing bool
	wtOnce       sync.Once
)

const (
	wtTTL      = 30 * time.Second
	gitTimeout = 5 * time.Second
)

// Worktrees lists every linked worktree of every known repo, most recently
// active first. The first call builds the list (single-flight); after that a
// request never waits on git — the last list is served while a refresh runs
// in the background.
func Worktrees() []Worktree {
	wtOnce.Do(refreshWorktrees)
	wtMu.Lock()
	if time.Since(wtAt) > wtTTL && !wtRefreshing {
		wtRefreshing = true
		go refreshWorktrees()
	}
	c := wtCache
	wtMu.Unlock()
	return c
}

func refreshWorktrees() {
	list := scanWorktrees()
	wtMu.Lock()
	wtCache, wtAt, wtRefreshing = list, time.Now(), false
	wtMu.Unlock()
}

// scanWorktrees runs git over every known repo (8 at a time) and cross-refs
// the sessions that ran in each worktree. Paths are compared symlink-resolved
// (git reports resolved paths; Claude records what you typed).
func scanWorktrees() []Worktree {
	repos := RepoPaths()
	per := make([][]Worktree, len(repos))
	var wg sync.WaitGroup
	sem := make(chan struct{}, 8)
	for i, r := range repos {
		wg.Add(1)
		sem <- struct{}{}
		go func(i int, r string) {
			defer wg.Done()
			defer func() { <-sem }()
			per[i] = gitWorktrees(r)
		}(i, r)
	}
	wg.Wait()

	var out []Worktree
	seen := map[string]bool{}
	for _, list := range per {
		for _, wt := range list {
			if !seen[wt.Path] {
				seen[wt.Path] = true
				out = append(out, wt)
			}
		}
	}
	if len(out) == 0 {
		return out
	}
	sessions := Sessions()
	for i := range out {
		wt := resolvePath(out[i].Path)
		var last time.Time
		for _, s := range sessions {
			if underDir(resolvePath(s.Project), wt) {
				out[i].Sessions++
				if s.Ended.After(last) {
					last = s.Ended
				}
			}
		}
		if !last.IsZero() {
			out[i].LastActive = last.Local().Format(minuteLayout)
			out[i].LastActiveUnix = last.Unix()
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastActiveUnix > out[j].LastActiveUnix })
	return out
}

func resolvePath(p string) string {
	if r, err := filepath.EvalSymlinks(p); err == nil {
		return r
	}
	return filepath.Clean(p)
}

// gitWorktrees lists a repo's linked worktrees and fills in their state.
func gitWorktrees(repo string) []Worktree {
	raw, err := git(repo, "worktree", "list", "--porcelain")
	if err != nil {
		return nil
	}
	out := parseWorktreeList(repo, raw)
	for i := range out {
		fillWorktree(&out[i])
	}
	return out
}

// parseWorktreeList parses `git worktree list --porcelain`: blank-line
// separated blocks, the main checkout always first — so it is skipped by
// position, which is right even when repo is itself a linked worktree.
func parseWorktreeList(repo, raw string) []Worktree {
	var out []Worktree
	for i, block := range strings.Split(strings.TrimSpace(raw), "\n\n") {
		lines := strings.Split(block, "\n")
		path, ok := strings.CutPrefix(lines[0], "worktree ")
		if !ok || i == 0 {
			continue
		}
		wt := Worktree{Repo: repo, Path: path, Claude: isClaudeWorktree(path)}
		for _, line := range lines[1:] {
			switch {
			case strings.HasPrefix(line, "HEAD "):
				wt.Head = strings.TrimPrefix(line, "HEAD ")
				if len(wt.Head) > 7 {
					wt.Head = wt.Head[:7]
				}
			case strings.HasPrefix(line, "branch "):
				wt.Branch = strings.TrimPrefix(strings.TrimPrefix(line, "branch "), "refs/heads/")
			case line == "detached":
				wt.Detached = true
			}
		}
		out = append(out, wt)
	}
	return out
}

func fillWorktree(w *Worktree) {
	if !dirExists(w.Path) {
		w.Stale = true
		return
	}
	if st, err := git(w.Path, "status", "--porcelain"); err == nil && strings.TrimSpace(st) != "" {
		w.Dirty = len(strings.Split(strings.TrimSpace(st), "\n"))
	}
	if lg, err := git(w.Path, "log", "-1", "--format=%cr%n%s"); err == nil {
		parts := strings.SplitN(strings.TrimSpace(lg), "\n", 2)
		w.LastCommit = parts[0]
		if len(parts) > 1 {
			w.Subject = parts[1]
		}
	}
}

// git runs a read-only git command with a deadline, and without taking git's
// optional locks so it never races the user's own git in that worktree.
func git(dir string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), gitTimeout)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	cmd.Env = append(os.Environ(), "GIT_OPTIONAL_LOCKS=0")
	b, err := cmd.Output()
	return string(b), err
}
