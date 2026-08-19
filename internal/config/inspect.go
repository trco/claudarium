package config

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/tidwall/gjson"
)

// fileDate returns a file's modification date as YYYY-MM-DD (empty if missing).
func fileDate(path string) string {
	if fi, err := os.Stat(path); err == nil {
		return fi.ModTime().Format("2006-01-02")
	}
	return ""
}

// shortDate trims an ISO timestamp ("2026-03-06T21:22:15Z") to its date part.
func shortDate(iso string) string {
	if len(iso) >= 10 {
		return iso[:10]
	}
	return iso
}

// olderThan reports whether an ISO timestamp is more than `days` days in the past.
func olderThan(iso string, days int) bool {
	t, err := time.Parse(time.RFC3339, iso)
	if err != nil {
		return false
	}
	return time.Since(t) > time.Duration(days)*24*time.Hour
}

// ---- Capabilities (agents / skills / commands) ----

type Capability struct {
	Name         string
	Kind         string // agent | skill | command
	Description  string
	Model        string // frontmatter: model (if pinned)
	Tools        string // frontmatter: tools / allowed-tools
	ArgumentHint string // frontmatter: argument-hint (commands)
	Source       string // global | plugin:<name> | repo:<path>
	Path         string // absolute file path (SKILL.md / *.md)
	Modified     string // file mtime (YYYY-MM-DD)
}

// ExtraRoots are additional directories to scan one level deep for repos with
// a .claude dir (set from the --scan flag). Covers roots you've never opened
// Claude in, so they don't share a parent with any recorded project.
var ExtraRoots []string

// Capabilities aggregates agents/skills/commands from global config,
// installed plugins, and every known project repo.
func Capabilities() []Capability {
	var out []Capability
	out = append(out, scanRoot(ClaudeDir(), "global")...)

	// Plugins: each install root holds agents/commands/skills like a .claude dir.
	if b, err := os.ReadFile(InstalledPluginsPath()); err == nil {
		gjson.GetBytes(b, "plugins").ForEach(func(name, installs gjson.Result) bool {
			installs.ForEach(func(_, inst gjson.Result) bool {
				if p := inst.Get("installPath").String(); p != "" {
					out = append(out, scanRoot(p, "plugin:"+name.String())...)
				}
				return true
			})
			return true
		})
	}

	// Repos: recorded projects + any sibling repo with a .claude dir on disk.
	claudeDir := filepath.Clean(ClaudeDir())
	for _, repo := range RepoPaths() {
		if filepath.Clean(filepath.Join(repo, ".claude")) == claudeDir {
			continue // this "repo" is ~/.claude itself; already covered as "global"
		}
		out = append(out, scanRoot(filepath.Join(repo, ".claude"), "repo:"+repo)...)
	}
	return out
}

// scanRoot reads root/agents/*.md, root/commands/*.md, root/skills/*/SKILL.md.
func scanRoot(root, source string) []Capability {
	var out []Capability
	out = append(out, scanMdDir(filepath.Join(root, "agents"), "agent", source)...)
	out = append(out, scanMdDir(filepath.Join(root, "commands"), "command", source)...)

	entries, _ := os.ReadDir(filepath.Join(root, "skills"))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		skillMd := filepath.Join(root, "skills", e.Name(), "SKILL.md")
		if _, err := os.Stat(skillMd); err != nil {
			continue
		}
		fm := frontmatterMap(skillMd)
		out = append(out, Capability{
			Name:        e.Name(),
			Kind:        "skill",
			Description: fm["description"],
			Model:       fm["model"],
			Tools:       firstNonEmpty(fm["tools"], fm["allowed-tools"]),
			Source:      source,
			Path:        skillMd,
			Modified:    fileDate(skillMd),
		})
	}
	return out
}

func scanMdDir(dir, kind, source string) []Capability {
	entries, _ := os.ReadDir(dir)
	var out []Capability
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		fm := frontmatterMap(path)
		out = append(out, Capability{
			Name:         strings.TrimSuffix(e.Name(), ".md"),
			Kind:         kind,
			Description:  fm["description"],
			Model:        fm["model"],
			Tools:        firstNonEmpty(fm["tools"], fm["allowed-tools"]),
			ArgumentHint: fm["argument-hint"],
			Source:       source,
			Path:         path,
			Modified:     fileDate(path),
		})
	}
	return out
}

// frontmatterMap parses a file's top-level YAML frontmatter into a flat map
// (best-effort). Nested/indented keys are skipped — we only surface scalars.
func frontmatterMap(path string) map[string]string {
	m := map[string]string{}
	b, err := os.ReadFile(path)
	if err != nil {
		return m
	}
	lines := strings.Split(string(b), "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return m
	}
	for _, l := range lines[1:] {
		if strings.TrimSpace(l) == "---" {
			break
		}
		if l == "" || l[0] == ' ' || l[0] == '\t' {
			continue // blank or nested (indented) line
		}
		if k, v, ok := strings.Cut(l, ":"); ok {
			m[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return m
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// ---- Plugins & marketplaces ----

type Plugin struct {
	Name        string // full id: plugin@marketplace
	Base        string // plugin part (before @)
	Marketplace string // marketplace part (after @)
	Version     string
	Scope       string
	ProjectPath string
	Enabled     bool
	InstallPath  string
	Installed    string // installedAt date (YYYY-MM-DD)
	Updated      string // lastUpdated date (YYYY-MM-DD)
	UpdatedStale bool   // lastUpdated more than 3 days ago
	Description  string // from the plugin's manifest
	Author       string
	Homepage     string
	License      string
	Components   string // e.g. "3 agents · 2 skills" (what it contributes)
	Path         string // install dir (for Reveal in Finder)
}

type Marketplace struct {
	Name       string
	SourceType string
	Location     string
	AutoUpdate   bool
	Updated      string // lastUpdated date (YYYY-MM-DD)
	UpdatedStale bool   // lastUpdated more than 3 days ago
	Owner        string // manifest owner (best-effort)
	Description  string // manifest description (best-effort)
	Installed    int    // plugins you've installed from this marketplace
}

func Plugins() []Plugin {
	enabled := map[string]bool{}
	if b, err := ReadSettingsRaw(); err == nil {
		gjson.GetBytes(b, "enabledPlugins").ForEach(func(k, v gjson.Result) bool {
			enabled[k.String()] = v.Bool()
			return true
		})
	}
	var out []Plugin
	if b, err := os.ReadFile(InstalledPluginsPath()); err == nil {
		gjson.GetBytes(b, "plugins").ForEach(func(name, installs gjson.Result) bool {
			base, market := splitPluginID(name.String())
			installs.ForEach(func(_, inst gjson.Result) bool {
				installPath := inst.Get("installPath").String()
				mani := pluginManifest(installPath)
				out = append(out, Plugin{
					Name:        name.String(),
					Base:        base,
					Marketplace: market,
					Version:     inst.Get("version").String(),
					Scope:       inst.Get("scope").String(),
					ProjectPath: inst.Get("projectPath").String(),
					Enabled:     enabled[name.String()],
					InstallPath: installPath,
					Installed:    shortDate(inst.Get("installedAt").String()),
					Updated:      shortDate(inst.Get("lastUpdated").String()),
					UpdatedStale: olderThan(inst.Get("lastUpdated").String(), 3),
					Description:  mani.Get("description").String(),
					Author:       manifestAuthor(mani),
					Homepage:     firstNonEmpty(mani.Get("homepage").String(), mani.Get("repository").String()),
					License:      mani.Get("license").String(),
					Components:   countComponents(installPath),
					Path:         installPath,
				})
				return true
			})
			return true
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// splitPluginID splits "plugin@marketplace" into its two parts.
func splitPluginID(id string) (base, marketplace string) {
	if parts := strings.SplitN(id, "@", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}
	return id, ""
}

// pluginManifest reads a plugin's manifest (.claude-plugin/plugin.json, or a
// bare plugin.json) from its install dir. Returns a zero Result if absent.
func pluginManifest(installPath string) gjson.Result {
	if installPath == "" {
		return gjson.Result{}
	}
	for _, rel := range []string{".claude-plugin/plugin.json", "plugin.json"} {
		if b, err := os.ReadFile(filepath.Join(installPath, rel)); err == nil {
			return gjson.ParseBytes(b)
		}
	}
	return gjson.Result{}
}

// manifestAuthor reads an author that may be a string or an {name,...} object.
func manifestAuthor(m gjson.Result) string {
	if a := m.Get("author"); a.IsObject() {
		return a.Get("name").String()
	} else {
		return a.String()
	}
}

// countComponents summarises what a plugin/repo contributes, e.g.
// "3 agents · 2 skills · 1 command". Empty if it ships none.
func countComponents(root string) string {
	if root == "" {
		return ""
	}
	var parts []string
	for _, c := range []struct{ dir, label string }{{"agents", "agent"}, {"commands", "command"}, {"skills", "skill"}} {
		n := 0
		entries, _ := os.ReadDir(filepath.Join(root, c.dir))
		for _, e := range entries {
			if c.dir == "skills" {
				if e.IsDir() {
					n++
				}
			} else if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
				n++
			}
		}
		if n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, plural(c.label, n)))
		}
	}
	return strings.Join(parts, " · ")
}

func plural(word string, n int) string {
	if n == 1 {
		return word
	}
	return word + "s"
}

func Marketplaces() []Marketplace {
	var out []Marketplace
	b, err := os.ReadFile(KnownMarketplacesPath())
	if err != nil {
		return out
	}
	installed := installedPerMarketplace()
	gjson.ParseBytes(b).ForEach(func(name, m gjson.Result) bool {
		src := m.Get("source")
		loc := src.Get("repo").String()
		if loc == "" {
			loc = src.Get("url").String()
		}
		out = append(out, Marketplace{
			Name:       name.String(),
			SourceType: src.Get("source").String(),
			Location:   loc,
			AutoUpdate:   m.Get("autoUpdate").Bool(),
			Updated:      shortDate(m.Get("lastUpdated").String()),
			UpdatedStale: olderThan(m.Get("lastUpdated").String(), 3),
			Owner:        firstNonEmpty(m.Get("owner.name").String(), m.Get("owner").String()),
			Description:  firstNonEmpty(m.Get("metadata.description").String(), m.Get("description").String()),
			Installed:    installed[name.String()],
		})
		return true
	})
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// installedPerMarketplace counts installed plugins grouped by their marketplace
// (the "@market" half of each plugin id).
func installedPerMarketplace() map[string]int {
	counts := map[string]int{}
	b, err := os.ReadFile(InstalledPluginsPath())
	if err != nil {
		return counts
	}
	gjson.GetBytes(b, "plugins").ForEach(func(name, _ gjson.Result) bool {
		if _, market := splitPluginID(name.String()); market != "" {
			counts[market]++
		}
		return true
	})
	return counts
}

// ---- MCP servers ----

type MCPServer struct {
	Name    string
	Scope   string // global | repo:<path>
	Kind    string // stdio | http/sse
	Target  string // command+args or url
	EnvKeys []string
	Enabled bool
	Stale   bool // repo scope whose directory no longer exists on disk
}

func MCPServers() []MCPServer {
	var out []MCPServer
	b, err := os.ReadFile(ClaudeJSONPath())
	if err != nil {
		return out
	}
	root := gjson.ParseBytes(b)

	root.Get("mcpServers").ForEach(func(name, def gjson.Result) bool {
		out = append(out, mcpFrom(name.String(), def, "global", true, false))
		return true
	})
	// Global servers we've disabled (stashed) — surfaced so they can be re-enabled.
	root.Get(globalDisabledMCPKey).ForEach(func(name, def gjson.Result) bool {
		out = append(out, mcpFrom(name.String(), def, "global", false, false))
		return true
	})

	root.Get("projects").ForEach(func(path, proj gjson.Result) bool {
		disabled := map[string]bool{}
		proj.Get("disabledMcpServers").ForEach(func(_, v gjson.Result) bool {
			disabled[v.String()] = true
			return true
		})
		stale := !dirExists(path.String())
		proj.Get("mcpServers").ForEach(func(name, def gjson.Result) bool {
			out = append(out, mcpFrom(name.String(), def, "repo:"+path.String(), !disabled[name.String()], stale))
			return true
		})
		return true
	})
	return out
}

func dirExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && fi.IsDir()
}

func mcpFrom(name string, def gjson.Result, scope string, enabled, stale bool) MCPServer {
	s := MCPServer{Name: name, Scope: scope, Enabled: enabled, Stale: stale}
	if url := def.Get("url").String(); url != "" {
		s.Kind = def.Get("type").String()
		if s.Kind == "" {
			s.Kind = "http/sse"
		}
		s.Target = url
	} else {
		s.Kind = "stdio"
		s.Target = strings.TrimSpace(def.Get("command").String() + " " + joinArgs(def.Get("args")))
	}
	def.Get("env").ForEach(func(k, _ gjson.Result) bool { // keys only — values are secrets
		s.EnvKeys = append(s.EnvKeys, k.String())
		return true
	})
	sort.Strings(s.EnvKeys)
	return s
}

// strs returns a gjson array at path as a []string.
func strs(b []byte, path string) []string {
	res := gjson.GetBytes(b, path).Array()
	out := make([]string, 0, len(res))
	for _, r := range res {
		out = append(out, r.String())
	}
	return out
}

func joinArgs(args gjson.Result) string {
	var parts []string
	args.ForEach(func(_, v gjson.Result) bool {
		parts = append(parts, v.String())
		return true
	})
	return strings.Join(parts, " ")
}

// RepoPaths returns repos to inspect: every project recorded in ~/.claude.json,
// plus any sibling repo (living in those projects' parent dirs) that has a
// .claude capabilities dir — even if Claude was never run there.
//
// ponytail: shallow, one-level scan of the parent dirs you already use, not a
// full-disk crawl. A repo under a brand-new root you've never opened Claude in
// won't be found; open Claude there once (or it shares a parent with one you have).
func RepoPaths() []string {
	seen := map[string]bool{}
	var out []string
	add := func(p string) {
		if p != "" && !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}

	recorded := ProjectPaths()
	for _, p := range recorded {
		add(p)
	}

	// Roots = distinct parents of recorded projects, minus filesystem-shallow
	// ones (/, /Users, ~'s parent) that would scan far too much.
	tooShallow := map[string]bool{"/": true, filepath.Dir(Home()): true}
	roots := map[string]bool{}
	for _, p := range recorded {
		if r := filepath.Dir(p); !tooShallow[r] {
			roots[r] = true
		}
	}
	for _, r := range ExtraRoots {
		if r != "" {
			roots[r] = true
		}
	}
	for root := range roots {
		entries, _ := os.ReadDir(root)
		for _, e := range entries {
			if e.IsDir() {
				if repo := filepath.Join(root, e.Name()); hasCapabilities(repo) {
					add(repo)
				}
			}
		}
	}
	sort.Strings(out)
	return out
}

// hasCapabilities reports whether repo/.claude holds any agent/command/skill.
func hasCapabilities(repo string) bool {
	for _, sub := range []string{"agents", "commands", "skills"} {
		if entries, err := os.ReadDir(filepath.Join(repo, ".claude", sub)); err == nil && len(entries) > 0 {
			return true
		}
	}
	return false
}

// ProjectPaths returns the project/repo paths recorded in ~/.claude.json.
func ProjectPaths() []string {
	b, err := os.ReadFile(ClaudeJSONPath())
	if err != nil {
		return nil
	}
	var paths []string
	gjson.GetBytes(b, "projects").ForEach(func(k, _ gjson.Result) bool {
		paths = append(paths, k.String())
		return true
	})
	sort.Strings(paths)
	return paths
}
