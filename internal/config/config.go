// Package config holds the user-facing pilot settings written by the TUI:
// the setup-selected device and the enable switch. Secrets never live here.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Config is the pilot setup state.
type Config struct {
	DeviceID string `json:"device_id"`
	Enabled  bool   `json:"enabled"`
}

// Path resolves the config file.
func Path() string {
	if d := strings.TrimSpace(os.Getenv("CONTEXTFM_CONFIG_DIR")); d != "" {
		return filepath.Join(d, "config.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "context.fm", "config.json")
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "context.fm", "config.json")
	}
	return filepath.Join(home, ".config", "context.fm", "config.json")
}

// Load reads config; missing file yields a zero Config.
func Load() (Config, error) {
	var c Config
	raw, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return c, nil
		}
		return c, err
	}
	if err := json.Unmarshal(raw, &c); err != nil {
		return c, err
	}
	return c, nil
}

// Save writes config with 0600 permissions.
func Save(c Config) error {
	p := Path()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}
