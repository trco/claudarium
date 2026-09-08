package config

import (
	"sort"
	"time"
)

// UsageRow is tokens/cost/sessions for one bucket (a window, a day, a project, a model).
type UsageRow struct {
	Key       string
	Sessions  int
	Tokens    int64
	Cost      float64
	TokensFmt string
	CostFmt   string
}

// UsageSummary aggregates every session's billed tokens and estimated cost.
type UsageSummary struct {
	Windows                   []UsageRow // Today, Last 7 days, Last 30 days
	ByDay, ByProject, ByModel []UsageRow
}

// Usage rolls every session up; UsageOf does the same for a list you already
// have (a page that lists sessions shouldn't index them twice).
func Usage() UsageSummary { return UsageOf(Sessions()) }

// UsageOf buckets usage by window and day from each session's per-day usage
// (so a session resumed across days lands on the days it was actually used),
// and by project and model from session totals.
func UsageOf(sessions []Session) UsageSummary {
	now := time.Now()
	day := func(t time.Time) string { return t.Local().Format(dayLayout) }
	monthAgo := day(now.AddDate(0, 0, -30))
	windows := []struct {
		row  UsageRow
		from string // inclusive first day, ISO — compares lexically
	}{
		{UsageRow{Key: "Today"}, day(now)},
		{UsageRow{Key: "Last 7 days"}, day(now.AddDate(0, 0, -7))},
		{UsageRow{Key: "Last 30 days"}, monthAgo},
	}

	var u UsageSummary
	byDay, byProj, byModel := map[string]*UsageRow{}, map[string]*UsageRow{}, map[string]*UsageRow{}
	row := func(m map[string]*UsageRow, k string) *UsageRow {
		if m[k] == nil {
			m[k] = &UsageRow{Key: k}
		}
		return m[k]
	}
	for _, s := range sessions {
		inWindow := make([]bool, len(windows))
		for d, du := range s.Daily {
			for i := range windows {
				if d >= windows[i].from {
					windows[i].row.Tokens += du.Tokens
					windows[i].row.Cost += du.Cost
					inWindow[i] = true
				}
			}
			if d >= monthAgo {
				r := row(byDay, d)
				r.Sessions++
				r.Tokens += du.Tokens
				r.Cost += du.Cost
			}
		}
		for i := range windows {
			if inWindow[i] {
				windows[i].row.Sessions++
			}
		}
		for _, r := range []*UsageRow{row(byProj, s.Repo), row(byModel, firstNonEmpty(s.Model, "(unknown)"))} {
			r.Sessions++
			r.Tokens += s.Tokens
			r.Cost += s.Cost
		}
	}
	for _, w := range windows {
		w.row.TokensFmt, w.row.CostFmt = fmtTokens(w.row.Tokens), fmtCost(w.row.Cost)
		u.Windows = append(u.Windows, w.row)
	}
	byCost := func(a, b UsageRow) bool { return a.Cost > b.Cost || (a.Cost == b.Cost && a.Tokens > b.Tokens) }
	u.ByDay = usageRows(byDay, func(a, b UsageRow) bool { return a.Key > b.Key })
	u.ByProject = usageRows(byProj, byCost)
	u.ByModel = usageRows(byModel, byCost)
	return u
}

func usageRows(m map[string]*UsageRow, less func(a, b UsageRow) bool) []UsageRow {
	out := make([]UsageRow, 0, len(m))
	for _, r := range m {
		r.TokensFmt, r.CostFmt = fmtTokens(r.Tokens), fmtCost(r.Cost)
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return less(out[i], out[j]) })
	return out
}
