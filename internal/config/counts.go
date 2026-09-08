package config

import (
	"sync"
	"time"
)

// NavCounts are the per-tab totals shown as badges in the sidebar.
type NavCounts struct {
	Capabilities int
	Plugins      int
	Marketplaces int
	MCP          int
	Health       int
	Problems     int // Doctor findings that are warn/error (drives the amber badge)
	Logs         int
	Sessions     int
	Worktrees    int
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
	issues := HealthChecks()
	countsCache = NavCounts{
		Capabilities: len(Capabilities()),
		Plugins:      len(Plugins()),
		Marketplaces: len(Marketplaces()),
		MCP:          len(MCPServers()),
		Health:       len(issues),
		Problems:     countProblems(issues),
		Logs:         AuditCount(),
		Sessions:     SessionCount(),
		Worktrees:    len(Worktrees()),
	}
	countsAt = time.Now()
	countsValid = true
	return countsCache
}

func countProblems(issues []Issue) int {
	n := 0
	for _, i := range issues {
		if i.Level != "info" {
			n++
		}
	}
	return n
}
