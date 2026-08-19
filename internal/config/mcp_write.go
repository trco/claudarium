package config

import (
	"errors"
	"os"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// globalDisabledMCPKey stashes disabled global MCP server definitions so they
// survive a disable (Claude only reads "mcpServers"). Nothing is ever deleted.
const globalDisabledMCPKey = "_claudariumDisabledMcpServers"

// sjsonEscape escapes sjson/gjson path metacharacters so s is treated as one
// literal key (plugin ids contain '@'; repo paths could contain '.').
func sjsonEscape(s string) string {
	return strings.NewReplacer(".", `\.`, "@", `\@`, "*", `\*`, "?", `\?`).Replace(s)
}

// SetMCPEnabled enables/disables an MCP server, writing ~/.claude.json.
// Reversible and non-destructive: repo servers toggle the native
// disabledMcpServers list; global servers move between mcpServers and a
// private stash key. No backup — nothing is ever deleted.
func SetMCPEnabled(scope, name string, enabled bool) error {
	b, err := os.ReadFile(ClaudeJSONPath())
	if err != nil {
		return err
	}
	var updated []byte
	if p, ok := strings.CutPrefix(scope, "repo:"); ok {
		updated, err = setRepoMCPEnabled(b, p, name, enabled)
	} else {
		updated, err = setGlobalMCPEnabled(b, name, enabled)
	}
	if err != nil {
		return err
	}
	return WriteFile(ClaudeJSONPath(), updated)
}

// setRepoMCPEnabled adds/removes name in projects.<path>.disabledMcpServers.
// The server definition under mcpServers is left untouched either way.
func setRepoMCPEnabled(b []byte, projPath, name string, enabled bool) ([]byte, error) {
	base := "projects." + sjsonEscape(projPath)
	if !gjson.GetBytes(b, base).Exists() {
		return nil, errors.New("project not found in ~/.claude.json: " + projPath)
	}
	list := []string{}
	for _, v := range gjson.GetBytes(b, base+".disabledMcpServers").Array() {
		if v.String() != name {
			list = append(list, v.String())
		}
	}
	if !enabled {
		list = append(list, name)
	}
	return sjson.SetBytes(b, base+".disabledMcpServers", list)
}

// setGlobalMCPEnabled moves a global server between mcpServers and the stash.
func setGlobalMCPEnabled(b []byte, name string, enabled bool) ([]byte, error) {
	from, to := "mcpServers", globalDisabledMCPKey
	if enabled {
		from, to = globalDisabledMCPKey, "mcpServers"
	}
	key := sjsonEscape(name)
	src := gjson.GetBytes(b, from+"."+key)
	if !src.Exists() {
		return nil, errors.New("MCP server not found: " + name)
	}
	b, err := sjson.SetRawBytes(b, to+"."+key, []byte(src.Raw))
	if err != nil {
		return nil, err
	}
	return sjson.DeleteBytes(b, from+"."+key)
}
