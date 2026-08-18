package config

import (
	"bytes"
	"os"

	"github.com/yuin/goldmark"
)

func ReadMemory() (string, error) {
	b, err := os.ReadFile(MemoryPath())
	if os.IsNotExist(err) {
		return "", nil
	}
	return string(b), err
}

// RenderMarkdown renders CLAUDE.md to HTML for the preview pane.
func RenderMarkdown(src string) (string, error) {
	var buf bytes.Buffer
	if err := goldmark.Convert([]byte(src), &buf); err != nil {
		return "", err
	}
	return buf.String(), nil
}
