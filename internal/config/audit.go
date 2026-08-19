package config

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// AuditLogPath is the append-only log of config changes Claudarium made.
func AuditLogPath() string { return filepath.Join(ClaudeDir(), ".claudarium-audit.jsonl") }

// AuditEntry is one logged change. Memory edits reference a .bak snapshot (and
// get a diff); toggles are logged with no backup.
type AuditEntry struct {
	ID      string `json:"id"`
	Unix    int64  `json:"unix"`
	Kind    string `json:"kind"` // memory | plugin | mcp
	Summary string `json:"summary"`
	Backup  string `json:"backup"` // .bak path, or "" for toggles

	When string     `json:"-"` // formatted
	Diff []DiffLine `json:"-"` // this snapshot -> its successor (memory only)
	Adds int        `json:"-"`
	Dels int        `json:"-"`
}

const auditDiffMaxBytes = 512 * 1024

// AppendAudit records one change. Best-effort: a logging failure never breaks
// the write that triggered it.
func AppendAudit(kind, summary, backup string) {
	e := AuditEntry{
		ID:      strconv.FormatInt(time.Now().UnixNano(), 10),
		Unix:    time.Now().Unix(),
		Kind:    kind,
		Summary: summary,
		Backup:  backup,
	}
	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	f, err := os.OpenFile(AuditLogPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	f.Write(append(b, '\n'))
}

// AuditCount returns the number of logged entries (cheap — no diffs).
func AuditCount() int { return len(readAudit()) }

func readAudit() []AuditEntry {
	b, err := os.ReadFile(AuditLogPath())
	if err != nil {
		return nil
	}
	var out []AuditEntry
	for _, line := range strings.Split(string(b), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var e AuditEntry
		if json.Unmarshal([]byte(line), &e) == nil {
			out = append(out, e)
		}
	}
	return out
}

// AuditEntries returns the log newest-first, with a diff computed for each
// memory snapshot (this backup -> the next memory backup, or the current file).
func AuditEntries() []AuditEntry {
	all := readAudit() // chronological (append order)
	var mem []int
	for i := range all {
		all[i].When = time.Unix(all[i].Unix, 0).Format("2006-01-02 15:04:05")
		if all[i].Kind == "memory" && all[i].Backup != "" {
			mem = append(mem, i)
		}
	}
	for n, i := range mem {
		before := readCapped(all[i].Backup)
		after := readCapped(MemoryPath())
		if n+1 < len(mem) {
			after = readCapped(all[mem[n+1]].Backup)
		}
		all[i].Diff = LineDiff(before, after)
		for _, l := range all[i].Diff {
			switch l.Op {
			case "+":
				all[i].Adds++
			case "-":
				all[i].Dels++
			}
		}
	}
	// Newest first. ID is UnixNano (unique, monotonic) so ties within the same
	// second still order correctly.
	sort.Slice(all, func(a, b int) bool {
		ai, _ := strconv.ParseInt(all[a].ID, 10, 64)
		bi, _ := strconv.ParseInt(all[b].ID, 10, 64)
		return ai > bi
	})
	return all
}

// DeleteAuditEntry drops one log entry (by id) and its backup file, if any.
func DeleteAuditEntry(id string) error {
	all := readAudit()
	var kept []AuditEntry
	var target *AuditEntry
	for i := range all {
		if all[i].ID == id {
			e := all[i]
			target = &e
			continue
		}
		kept = append(kept, all[i])
	}
	if target == nil {
		return errors.New("audit entry not found")
	}
	if target.Backup != "" {
		_ = DeleteBackup(target.Backup) // best-effort — may already be gone
	}
	return writeAudit(kept)
}

// ClearAudit removes the whole log and every backup it referenced; returns the
// count of backup files deleted.
func ClearAudit() (int, error) {
	n := 0
	for _, e := range readAudit() {
		if e.Backup != "" && DeleteBackup(e.Backup) == nil {
			n++
		}
	}
	if err := os.Remove(AuditLogPath()); err != nil && !os.IsNotExist(err) {
		return n, err
	}
	return n, nil
}

func writeAudit(entries []AuditEntry) error {
	var buf bytes.Buffer
	for _, e := range entries {
		b, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf.Write(b)
		buf.WriteByte('\n')
	}
	return WriteFile(AuditLogPath(), buf.Bytes())
}

// ---- backup file helpers (shared) ----

var bakRe = regexp.MustCompile(`\.bak\.\d+$`)

func readCapped(path string) string {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > auditDiffMaxBytes {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// DeleteBackup removes one backup file. Guarded: absolute, under home, and a
// real .bak.<unix> file — never an arbitrary path.
func DeleteBackup(path string) error {
	clean := filepath.Clean(path)
	if !filepath.IsAbs(clean) || !underHome(clean) {
		return errors.New("path is outside home directory")
	}
	if !bakRe.MatchString(filepath.Base(clean)) {
		return errors.New("not a backup file")
	}
	return os.Remove(clean)
}

func underHome(clean string) bool {
	return clean == Home() || strings.HasPrefix(clean, Home()+string(filepath.Separator))
}
