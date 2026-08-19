package config

import (
	"fmt"
	"os"
	"time"
)

// Backup copies path to path.bak.<unix> and returns the backup path.
// If path doesn't exist yet (first write), it returns "" with no error.
func Backup(path string) (string, error) {
	src, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}
	bak := fmt.Sprintf("%s.bak.%d", path, time.Now().Unix())
	return bak, os.WriteFile(bak, src, 0o600)
}

// SaveFile backs up path, then writes content preserving the existing mode
// (default 0600). A failed backup aborts the write — we never clobber
// a hand-edited file without a copy on disk. Used for Memory edits.
func SaveFile(path string, content []byte) (backup string, err error) {
	backup, err = Backup(path)
	if err != nil {
		return "", fmt.Errorf("backup failed, not writing: %w", err)
	}
	return backup, WriteFile(path, content)
}

// WriteFile writes content to path preserving the existing mode (default 0600),
// without a backup. Used for surgical, reversible toggles.
func WriteFile(path string, content []byte) error {
	mode := os.FileMode(0o600)
	if fi, statErr := os.Stat(path); statErr == nil {
		mode = fi.Mode().Perm()
	}
	return os.WriteFile(path, content, mode)
}
