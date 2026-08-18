package config

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/tidwall/gjson"
)

// Issue is a single config-health finding.
type Issue struct {
	Level   string // error | warn | info
	Area    string // permissions | plugins | mcp
	Message string
}

// HealthChecks scans the config for common problems. Read-only, best-effort.
func HealthChecks() []Issue {
	var issues []Issue

	settings, _ := ReadSettingsRaw()

	// Permissions: duplicate rules within a list.
	for _, list := range []string{"allow", "deny", "ask"} {
		seen := map[string]bool{}
		for _, r := range strs(settings, "permissions."+list) {
			if seen[r] {
				issues = append(issues, Issue{"warn", "permissions", "duplicate rule in " + list + ": " + r})
			}
			seen[r] = true
		}
	}
	// additionalDirectories that no longer exist.
	for _, d := range strs(settings, "permissions.additionalDirectories") {
		if !dirExists(d) {
			issues = append(issues, Issue{"info", "permissions", "additionalDirectories path is missing: " + d})
		}
	}

	// Plugins: enabled-but-not-installed and installed-but-not-enabled.
	enabled := map[string]bool{}
	gjson.GetBytes(settings, "enabledPlugins").ForEach(func(k, v gjson.Result) bool {
		if v.Bool() {
			enabled[k.String()] = true
		}
		return true
	})
	installed := map[string]bool{}
	if b, err := os.ReadFile(InstalledPluginsPath()); err == nil {
		gjson.GetBytes(b, "plugins").ForEach(func(k, _ gjson.Result) bool {
			installed[k.String()] = true
			return true
		})
	}
	for name := range enabled {
		if !installed[name] {
			issues = append(issues, Issue{"error", "plugins", "enabled but not installed: " + name})
		}
	}
	for name := range installed {
		if !enabled[name] {
			issues = append(issues, Issue{"info", "plugins", "installed but not enabled: " + name})
		}
	}

	// MCP: stale repo references and stdio commands not found on PATH.
	for _, s := range MCPServers() {
		if s.Stale {
			issues = append(issues, Issue{"warn", "mcp", s.Name + " references a missing repo: " + strings.TrimPrefix(s.Scope, "repo:")})
		}
		if s.Kind == "stdio" {
			if fields := strings.Fields(s.Target); len(fields) > 0 {
				if !commandExists(fields[0]) {
					issues = append(issues, Issue{"warn", "mcp", s.Name + " command not found on PATH: " + fields[0]})
				}
			}
		}
	}

	return issues
}

// commandExists reports whether cmd resolves on PATH or is an existing file.
func commandExists(cmd string) bool {
	if strings.ContainsRune(cmd, filepath.Separator) {
		return dirExists(filepath.Dir(cmd)) && fileExists(cmd)
	}
	_, err := exec.LookPath(cmd)
	return err == nil
}

func fileExists(p string) bool {
	fi, err := os.Stat(p)
	return err == nil && !fi.IsDir()
}
