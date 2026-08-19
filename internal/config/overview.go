package config

import "github.com/tidwall/gjson"

// Hook is one configured hook command (flattened from settings.json).
type Hook struct {
	Event   string // PreToolUse | PostToolUse | Stop | ...
	Matcher string // tool/name matcher (may be empty)
	Command string
}

// SettingsInfo is a read-only overview of the parts of settings.json that
// otherwise have no home in the UI (hooks, statusLine, permissions summary).
type SettingsInfo struct {
	Path           string
	Model          string
	StatusLine     string
	Hooks          []Hook
	Allow          int
	Deny           int
	Ask            int
	AdditionalDirs []string
}

// Settings parses settings.json into a display overview (best-effort).
func Settings() SettingsInfo {
	b, _ := ReadSettingsRaw()
	root := gjson.ParseBytes(b)
	info := SettingsInfo{
		Path:       SettingsPath(),
		Model:      root.Get("model").String(),
		StatusLine: root.Get("statusLine.command").String(),
		Allow:      len(root.Get("permissions.allow").Array()),
		Deny:       len(root.Get("permissions.deny").Array()),
		Ask:        len(root.Get("permissions.ask").Array()),
	}
	for _, d := range root.Get("permissions.additionalDirectories").Array() {
		info.AdditionalDirs = append(info.AdditionalDirs, d.String())
	}
	root.Get("hooks").ForEach(func(event, groups gjson.Result) bool {
		groups.ForEach(func(_, g gjson.Result) bool {
			matcher := g.Get("matcher").String()
			g.Get("hooks").ForEach(func(_, h gjson.Result) bool {
				info.Hooks = append(info.Hooks, Hook{event.String(), matcher, h.Get("command").String()})
				return true
			})
			return true
		})
		return true
	})
	return info
}
