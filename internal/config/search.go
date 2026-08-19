package config

import "strings"

// SearchResult is one cross-tab match.
type SearchResult struct {
	Kind   string // agent | skill | command | plugin | marketplace | mcp
	Name   string
	Detail string // description / target / location
	Source string // where it lives
	Path   string // for Reveal / raw when available
}

// Search returns matches for q across every inspectable config surface
// (case-insensitive substring over name + detail + source). Empty q → nil.
func Search(q string) []SearchResult {
	q = strings.ToLower(strings.TrimSpace(q))
	if q == "" {
		return nil
	}
	match := func(fields ...string) bool {
		for _, f := range fields {
			if strings.Contains(strings.ToLower(f), q) {
				return true
			}
		}
		return false
	}

	var out []SearchResult
	for _, c := range Capabilities() {
		if match(c.Name, c.Description, c.Source) {
			out = append(out, SearchResult{c.Kind, c.Name, c.Description, c.Source, c.Path})
		}
	}
	for _, p := range Plugins() {
		if match(p.Base, p.Description, p.Marketplace, p.Author) {
			out = append(out, SearchResult{"plugin", p.Base, p.Description, "marketplace:" + p.Marketplace, p.Path})
		}
	}
	for _, m := range Marketplaces() {
		if match(m.Name, m.Description, m.Location, m.Owner) {
			out = append(out, SearchResult{"marketplace", m.Name, firstNonEmpty(m.Description, m.Location), m.SourceType, ""})
		}
	}
	for _, s := range MCPServers() {
		if match(s.Name, s.Target, s.Scope) {
			out = append(out, SearchResult{"mcp", s.Name, s.Target, s.Scope, ""})
		}
	}
	return out
}
