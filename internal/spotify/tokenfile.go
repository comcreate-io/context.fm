package spotify

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// tokenFileName is the 0600 fallback when the OS keyring is unavailable.
// Keyring integration lands before the pilot slice; until then this file
// is the only store and must never be committed (see .gitignore patterns).
const tokenFileName = "spotify-token.json"

func getenv(k string) string { return os.Getenv(k) }

// TokenFilePath resolves the fallback token file.
func TokenFilePath() string {
	if d := strings.TrimSpace(os.Getenv("CONTEXTFM_CONFIG_DIR")); d != "" {
		return filepath.Join(d, tokenFileName)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "context.fm", tokenFileName)
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_CONFIG_HOME")); xdg != "" {
		return filepath.Join(xdg, "context.fm", tokenFileName)
	}
	return filepath.Join(home, ".config", "context.fm", tokenFileName)
}

// SaveToken writes the token with 0600 permissions.
func SaveToken(t Token) error {
	p := TokenFilePath()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return err
	}
	raw, err := json.Marshal(t)
	if err != nil {
		return err
	}
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, p)
}

// LoadToken reads the fallback token file.
func LoadToken() (Token, error) {
	var t Token
	raw, err := os.ReadFile(TokenFilePath())
	if err != nil {
		return t, err
	}
	if err := json.Unmarshal(raw, &t); err != nil {
		return t, fmt.Errorf("spotify: corrupt token file: %w", err)
	}
	if t.AccessToken == "" {
		return t, fmt.Errorf("spotify: empty access token in file")
	}
	return t, nil
}

// ClearToken removes the fallback token file.
func ClearToken() error {
	err := os.Remove(TokenFilePath())
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
