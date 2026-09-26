package spotify

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGateClosedByDefault(t *testing.T) {
	t.Setenv("CONTEXTFM_ENABLE_PLAYBACK", "")
	c := NewClient("tok", "http://example.invalid")
	ctl, err := NewController(c, "dev_1")
	if err != nil {
		t.Fatal(err)
	}
	if err := ctl.QueueTrack(context.Background(), "spotify:track:trk_1"); err == nil ||
		!strings.Contains(err.Error(), "gate closed") {
		t.Fatalf("expected closed-gate error, got %v", err)
	}
	if err := ctl.Pause(context.Background()); err == nil {
		t.Fatal("expected closed-gate error for pause")
	}
}

func TestEmptyDeviceRejected(t *testing.T) {
	c := NewClient("tok", "http://example.invalid")
	if _, err := NewController(c, ""); err == nil {
		t.Fatal("expected device error")
	}
}

func TestQueueRejectsNonTrackURI(t *testing.T) {
	t.Setenv("CONTEXTFM_ENABLE_PLAYBACK", "1")
	c := NewClient("tok", "http://example.invalid")
	ctl, _ := NewController(c, "dev_1")
	if err := ctl.QueueTrack(context.Background(), "spotify:album:abc"); err == nil {
		t.Fatal("expected URI error")
	}
}

func TestQueueAndPauseAgainstServer(t *testing.T) {
	t.Setenv("CONTEXTFM_ENABLE_PLAYBACK", "1")
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer tok" {
			t.Errorf("missing auth")
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	c := NewClient("tok", srv.URL)
	ctl, _ := NewController(c, "dev_1")
	ctx := context.Background()
	if err := ctl.QueueTrack(ctx, "spotify:track:trk_1"); err != nil {
		t.Fatal(err)
	}
	if err := ctl.Pause(ctx); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 2 || calls[0] != "POST /me/player/queue" || calls[1] != "PUT /me/player/pause" {
		t.Fatalf("calls=%v", calls)
	}
}

func TestCurrentParses(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"item":{"id":"trk_9","name":"Now","duration_ms":200000},"device":{"id":"dev_1","name":"Spk"},"is_playing":true,"progress_ms":190000}`))
	}))
	defer srv.Close()
	c := NewClient("tok", srv.URL)
	np, err := c.Current(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if np.TrackID != "trk_9" || !np.IsPlaying || np.ProgressMs != 190000 || np.DeviceID != "dev_1" {
		t.Fatalf("got %+v", np)
	}
}

func TestEnsureValidTokenLoadsStored(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONTEXTFM_CONFIG_DIR", dir)
	if err := SaveToken(Token{AccessToken: "live", ExpiresAt: time.Now().UTC().Add(time.Hour)}); err != nil {
		t.Fatal(err)
	}
	c, err := EnsureValidToken(context.Background(), "", "")
	if err != nil {
		t.Fatal(err)
	}
	if c.tok != "live" {
		t.Fatalf("got %q", c.tok)
	}
}

func TestEnsureValidTokenRefreshes(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONTEXTFM_CONFIG_DIR", dir)
	if err := SaveToken(Token{AccessToken: "stale", RefreshToken: "ref_1",
		ExpiresAt: time.Now().UTC().Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"fresh","expires_in":3600}`))
	}))
	defer srv.Close()
	c, err := EnsureValidToken(context.Background(), srv.URL, "cid")
	if err != nil {
		t.Fatal(err)
	}
	if c.tok != "fresh" {
		t.Fatalf("got %q", c.tok)
	}
	kept, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if kept.RefreshToken != "ref_1" {
		t.Fatalf("refresh token must be preserved, got %+v", kept)
	}
}

func TestEnsureValidTokenFailsWithoutToken(t *testing.T) {
	t.Setenv("CONTEXTFM_CONFIG_DIR", t.TempDir())
	if _, err := EnsureValidToken(context.Background(), "", ""); err == nil {
		t.Fatal("expected error with no stored token")
	}
}
