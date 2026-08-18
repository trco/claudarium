package config

import (
	"errors"
	"os/exec"
	"path/filepath"
	"strings"
)

// RevealFile opens the given path in Finder (macOS `open -R`). It only allows
// absolute, existing paths under the user's home dir — this is a trust boundary:
// the path comes from an HTTP form, so we never exec on arbitrary input.
func RevealFile(path string) error {
	if path == "" || !filepath.IsAbs(path) {
		return errors.New("path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != Home() && !strings.HasPrefix(clean, Home()+string(filepath.Separator)) {
		return errors.New("path is outside home directory")
	}
	if !fileExists(clean) && !dirExists(clean) {
		return errors.New("path does not exist")
	}
	return exec.Command("open", "-R", clean).Start()
}
