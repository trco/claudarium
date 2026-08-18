package config

import "strings"

// DiffLine is one line of a unified diff. Op is " ", "+", or "-".
type DiffLine struct {
	Op   string
	Text string
}

// LineDiff produces a line-level unified diff of old vs new via LCS.
// Good enough to eyeball a config change before applying it.
func LineDiff(old, new string) []DiffLine {
	a := strings.Split(old, "\n")
	b := strings.Split(new, "\n")

	// LCS length table.
	m, n := len(a), len(b)
	lcs := make([][]int, m+1)
	for i := range lcs {
		lcs[i] = make([]int, n+1)
	}
	for i := m - 1; i >= 0; i-- {
		for j := n - 1; j >= 0; j-- {
			if a[i] == b[j] {
				lcs[i][j] = lcs[i+1][j+1] + 1
			} else if lcs[i+1][j] >= lcs[i][j+1] {
				lcs[i][j] = lcs[i+1][j]
			} else {
				lcs[i][j] = lcs[i][j+1]
			}
		}
	}

	var out []DiffLine
	i, j := 0, 0
	for i < m && j < n {
		switch {
		case a[i] == b[j]:
			out = append(out, DiffLine{" ", a[i]})
			i, j = i+1, j+1
		case lcs[i+1][j] >= lcs[i][j+1]:
			out = append(out, DiffLine{"-", a[i]})
			i++
		default:
			out = append(out, DiffLine{"+", b[j]})
			j++
		}
	}
	for ; i < m; i++ {
		out = append(out, DiffLine{"-", a[i]})
	}
	for ; j < n; j++ {
		out = append(out, DiffLine{"+", b[j]})
	}
	return out
}

// HasChanges reports whether a diff contains any add/remove lines.
func HasChanges(d []DiffLine) bool {
	for _, l := range d {
		if l.Op != " " {
			return true
		}
	}
	return false
}
