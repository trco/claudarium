package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
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

	bak, err := SetPluginEnabled("foo@mkt", false)
	if err != nil {
		t.Fatal(err)
	}
	if bak == "" {
		t.Fatal("expected a backup path")
	}
	if cur, _ := os.ReadFile(sp); !strings.Contains(string(cur), `"foo@mkt":false`) {
		t.Errorf("want foo@mkt=false, got %s", cur)
	}
	if prev, _ := os.ReadFile(bak); !strings.Contains(string(prev), `"foo@mkt":true`) {
		t.Errorf("backup should hold the previous value, got %s", prev)
	}
	if _, err := SetPluginEnabled("bar@mkt", true); err != nil {
		t.Fatal(err)
	}
	if cur, _ := os.ReadFile(sp); !strings.Contains(string(cur), `"bar@mkt":true`) {
		t.Errorf("want bar@mkt=true, got %s", cur)
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
