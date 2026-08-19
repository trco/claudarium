package config

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// bakRe matches "<original>.bak.<unixseconds>".
var bakRe = regexp.MustCompile(`^(.*)\.bak\.(\d+)$`)

// BackupEntry is one config backup file plus the change it captured.
type BackupEntry struct {
	Path     string     // full path to the .bak file
	Original string     // the file it backs up
	Name     string     // basename of the original
	When     string     // formatted local time
	Unix     int64      // backup timestamp (from the filename)
	Size     int64      // backup size in bytes
	Diff     []DiffLine // this snapshot -> the version that replaced it
	Adds     int        // added lines in Diff
	Dels     int        // removed lines in Diff
}

const backupDiffMaxBytes = 512 * 1024 // skip diffing files larger than this

// Backups lists every config backup written under the home dir, newest first.
// Each entry carries the diff from that snapshot to the version that replaced
// it (the next-newer backup of the same file, or the current file if newest).
func Backups() []BackupEntry {
	var all []BackupEntry
	seen := map[string]bool{}
	add := func(dir string, e os.DirEntry) {
		if e.IsDir() {
			return
		}
		m := bakRe.FindStringSubmatch(e.Name())
		if m == nil {
			return
		}
		unix, err := strconv.ParseInt(m[2], 10, 64)
		if err != nil {
			return
		}
		path := filepath.Join(dir, e.Name())
		if seen[path] {
			return
		}
		seen[path] = true
		info, _ := e.Info()
		var size int64
		if info != nil {
			size = info.Size()
		}
		all = append(all, BackupEntry{
			Path:     path,
			Original: filepath.Join(dir, m[1]),
			Name:     m[1],
			Unix:     unix,
			When:     time.Unix(unix, 0).Format("2006-01-02 15:04:05"),
			Size:     size,
		})
	}

	// Backups sit next to their originals: ~/.claude/ (Memory, settings) and
	// the home root (~/.claude.json). Top-level only — that's where they land.
	for _, dir := range []string{ClaudeDir(), Home()} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			add(dir, e)
		}
	}

	computeBackupDiffs(all)
	sort.Slice(all, func(i, j int) bool { return all[i].Unix > all[j].Unix })
	return all
}

// computeBackupDiffs fills each entry's Diff: snapshot -> its successor version.
func computeBackupDiffs(all []BackupEntry) {
	byOrig := map[string][]int{}
	for i := range all {
		byOrig[all[i].Original] = append(byOrig[all[i].Original], i)
	}
	for orig, idxs := range byOrig {
		sort.Slice(idxs, func(a, b int) bool { return all[idxs[a]].Unix < all[idxs[b]].Unix })
		for n, i := range idxs {
			before := readCapped(all[i].Path)
			after := readCapped(orig) // current live file (default for newest)
			if n+1 < len(idxs) {
				after = readCapped(all[idxs[n+1]].Path)
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
	}
}

func readCapped(path string) string {
	fi, err := os.Stat(path)
	if err != nil || fi.Size() > backupDiffMaxBytes {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(b)
}

// DeleteBackup removes one backup file. Guarded: absolute, under home, and a
// real .bak.<unix> file — never an arbitrary path from the HTTP form.
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

// DeleteAllBackups removes every discovered backup and returns the count.
func DeleteAllBackups() (int, error) {
	n := 0
	for _, e := range Backups() {
		if err := os.Remove(e.Path); err == nil {
			n++
		}
	}
	return n, nil
}

func underHome(clean string) bool {
	return clean == Home() || strings.HasPrefix(clean, Home()+string(filepath.Separator))
}
