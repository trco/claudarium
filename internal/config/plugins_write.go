package config

import (
	"os"

	"github.com/tidwall/sjson"
)

// SetPluginEnabled flips enabledPlugins[name] in settings.json to enabled.
// It edits surgically (formatting preserved); no backup — the change is a
// single reversible key flip.
func SetPluginEnabled(name string, enabled bool) error {
	b, err := ReadSettingsRaw()
	if err != nil {
		if os.IsNotExist(err) {
			b = []byte("{}")
		} else {
			return err
		}
	}
	// Escape sjson path metacharacters so the full plugin id (which contains
	// '@', and could contain '.') is treated as one literal key.
	key := "enabledPlugins." + sjsonEscape(name)
	updated, err := sjson.SetBytes(b, key, enabled)
	if err != nil {
		return err
	}
	return WriteFile(SettingsPath(), updated)
}
