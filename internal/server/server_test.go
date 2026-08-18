package server

import (
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"

	"github.com/trco/claudarium/internal/config"
)

func newTestApp(t *testing.T) (*fiber.App, string) {
	t.Helper()
	home := t.TempDir()
	old := config.Home
	config.Home = func() string { return home }
	t.Cleanup(func() { config.Home = old })

	write(t, filepath.Join(home, ".claude/settings.json"),
		`{"permissions":{"allow":["Read(x)"]},"theme":"dark","customKeep":123}`)
	write(t, filepath.Join(home, ".claude/CLAUDE.md"), "# Title\n\nhello world")
	write(t, filepath.Join(home, ".claude/agents/foo.md"), "---\nname: foo\ndescription: does foo\n---\n")
	write(t, filepath.Join(home, ".claude/plugins/installed_plugins.json"),
		`{"plugins":{"p@m":[{"scope":"user","version":"1.0","installPath":"/x"}]}}`)
	write(t, filepath.Join(home, ".claude/plugins/known_marketplaces.json"),
		`{"m":{"source":{"source":"github","repo":"o/r"}}}`)
	write(t, filepath.Join(home, ".claude.json"),
		`{"mcpServers":{"db":{"command":"psql","env":{"PGPASSWORD":"secret"}}},"projects":{}}`)

	return New(false), home // embedded assets — works regardless of test CWD
}

func TestGetPagesRender(t *testing.T) {
	app, _ := newTestApp(t)
	cases := map[string]string{
		"/memory":       "<h1",                    // markdown rendered
		"/capabilities": "foo",                    // agent listed
		"/plugins":      `data-marketplace="m"`,   // plugin split into plugin+marketplace
		"/marketplaces": "o/r",                    // marketplace location listed
		"/mcp":          "db",                     // server listed
	}
	for path, want := range cases {
		body, code := do(t, app, "GET", path, nil)
		if code != 200 {
			t.Errorf("%s -> %d", path, code)
		}
		if !strings.Contains(body, want) {
			t.Errorf("%s body missing %q", path, want)
		}
	}
	// Secret env value must never reach the MCP page.
	if body, _ := do(t, app, "GET", "/mcp", nil); strings.Contains(body, "secret") {
		t.Error("secret env value leaked to /mcp page")
	}
}

func TestMemoryPreviewShowsDiff(t *testing.T) {
	app, _ := newTestApp(t)
	body, code := do(t, app, "POST", "/memory/preview", url.Values{"content": {"# Title\n\nchanged"}})
	if code != 200 || !strings.Contains(body, "Review changes") {
		t.Fatalf("preview code=%d body=%.80q", code, body)
	}
}

func TestMemoryApplyWritesAndBacksUp(t *testing.T) {
	app, home := newTestApp(t)
	_, code := do(t, app, "POST", "/memory/apply", url.Values{"content": {"# New\n\nbrand new memory"}})
	if code != 200 {
		t.Fatalf("apply code=%d", code)
	}
	b, err := os.ReadFile(filepath.Join(home, ".claude/CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), "brand new memory") {
		t.Errorf("CLAUDE.md not updated: %s", b)
	}
	if !hasBackup(t, filepath.Join(home, ".claude"), "CLAUDE.md.bak.") {
		t.Error("no backup written")
	}
}

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func do(t *testing.T, app *fiber.App, method, path string, form url.Values) (string, int) {
	t.Helper()
	var req *http.Request
	if form != nil {
		req, _ = http.NewRequest(method, path, strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	} else {
		req, _ = http.NewRequest(method, path, nil)
	}
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	b, _ := io.ReadAll(resp.Body)
	return string(b), resp.StatusCode
}

func hasBackup(t *testing.T, dir, prefix string) bool {
	t.Helper()
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), prefix) {
			return true
		}
	}
	return false
}
