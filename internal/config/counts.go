package config

import (
	"sync"
	"time"
)

// NavCounts are the per-tab totals shown as superscript badges in the nav.
type NavCounts struct {
	Capabilities int
	Plugins      int
	Marketplaces int
	MCP          int
	Health       int
	Logs         int
}

var (
	countsMu    sync.Mutex
	countsCache NavCounts
	countsAt    time.Time
	countsValid bool
)

// Counts returns nav badge totals, cached ~3s.
// ponytail: a TTL cache so we don't re-scan the filesystem on every request
// (incl. static assets); a few seconds stale is fine for a nav badge.
func Counts() NavCounts {
	countsMu.Lock()
	defer countsMu.Unlock()
	if countsValid && time.Since(countsAt) < 3*time.Second {
		return countsCache
	}
	countsCache = NavCounts{
		Capabilities: len(Capabilities()),
		Plugins:      len(Plugins()),
		Marketplaces: len(Marketplaces()),
		MCP:          len(MCPServers()),
		Health:       len(HealthChecks()),
		Logs:         AuditCount(),
	}
	countsAt = time.Now()
	countsValid = true
	return countsCache
}
