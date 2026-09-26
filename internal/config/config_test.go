package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONTEXTFM_CONFIG_DIR", dir)
	if err := Save(Config{DeviceID: "dev_1", Enabled: true}); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("config perm = %o, want 600", info.Mode().Perm())
	}
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DeviceID != "dev_1" || !got.Enabled {
		t.Fatalf("got %+v", got)
	}
}

func TestMissingFileIsZero(t *testing.T) {
	t.Setenv("CONTEXTFM_CONFIG_DIR", t.TempDir())
	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got != (Config{}) {
		t.Fatalf("got %+v", got)
	}
}
