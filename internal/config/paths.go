// Package config locates and safely reads/writes Claude Code config files.
package config

import (
	"os"
	"path/filepath"
)

// Home is the user's home dir; overridable in tests.
var Home = func() string {
	h, _ := os.UserHomeDir()
	return h
}

func ClaudeDir() string     { return filepath.Join(Home(), ".claude") }
func SettingsPath() string  { return filepath.Join(ClaudeDir(), "settings.json") }
func MemoryPath() string    { return filepath.Join(ClaudeDir(), "CLAUDE.md") }
func ClaudeJSONPath() string { return filepath.Join(Home(), ".claude.json") }

func InstalledPluginsPath() string {
	return filepath.Join(ClaudeDir(), "plugins", "installed_plugins.json")
}
func KnownMarketplacesPath() string {
	return filepath.Join(ClaudeDir(), "plugins", "known_marketplaces.json")
}
