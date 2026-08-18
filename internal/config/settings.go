package config

import "os"

// ReadSettingsRaw returns the raw settings.json bytes. Read-only: settings
// editing was intentionally removed; the Plugins view still reads enabledPlugins.
func ReadSettingsRaw() ([]byte, error) { return os.ReadFile(SettingsPath()) }
