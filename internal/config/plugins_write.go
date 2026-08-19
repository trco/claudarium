package config

import (
	"os"
	"strings"

	"github.com/tidwall/sjson"
)

// SetPluginEnabled flips enabledPlugins[name] in settings.json to enabled.
// It edits surgically (formatting preserved) and backs the file up first via
// SaveFile — a failed backup aborts the write. Returns the backup path.
func SetPluginEnabled(name string, enabled bool) (string, error) {
	b, err := ReadSettingsRaw()
	if err != nil {
		if os.IsNotExist(err) {
			b = []byte("{}")
		} else {
			return "", err
		}
	}
	// Escape sjson path metacharacters so the full plugin id (which contains
	// '@', and could contain '.') is treated as one literal key.
	esc := strings.NewReplacer(".", `\.`, "@", `\@`, "*", `\*`, "?", `\?`)
	key := "enabledPlugins." + esc.Replace(name)
	updated, err := sjson.SetBytes(b, key, enabled)
	if err != nil {
		return "", err
	}
	return SaveFile(SettingsPath(), updated)
}
