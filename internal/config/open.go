package config

import (
	"errors"
	"os"
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

// ReadTextFile returns the contents of a Markdown file under the user's home
// dir, for in-app preview. Same trust boundary as RevealFile, plus: only .md
// files (so we never dump settings.json / .claude.json, which can hold
// secrets), and capped at 1 MiB.
func ReadTextFile(path string) ([]byte, error) {
	if path == "" || !filepath.IsAbs(path) {
		return nil, errors.New("path must be absolute")
	}
	clean := filepath.Clean(path)
	if clean != Home() && !strings.HasPrefix(clean, Home()+string(filepath.Separator)) {
		return nil, errors.New("path is outside home directory")
	}
	if !strings.HasSuffix(strings.ToLower(clean), ".md") {
		return nil, errors.New("only .md files can be viewed")
	}
	if !fileExists(clean) {
		return nil, errors.New("file does not exist")
	}
	b, err := os.ReadFile(clean)
	if err != nil {
		return nil, err
	}
	const max = 1 << 20
	if len(b) > max {
		b = append(b[:max:max], []byte("\n… (truncated)")...)
	}
	return b, nil
}
