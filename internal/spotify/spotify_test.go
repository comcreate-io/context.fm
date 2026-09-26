package spotify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func fixtureServer(t *testing.T) *httptest.Server {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/me/top/tracks", func(w http.ResponseWriter, r *http.Request) {
		mustAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"trk_1","name":"Synthetic Song","artists":[{"id":"art_1","name":"Fixture Band"}],"popularity":60,"uri":"spotify:track:trk_1"}]}`))
	})
	mux.HandleFunc("/me/top/artists", func(w http.ResponseWriter, r *http.Request) {
		mustAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"id":"art_1","name":"Fixture Band"}]}`))
	})
	mux.HandleFunc("/me/player/recently-played", func(w http.ResponseWriter, r *http.Request) {
		mustAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"items":[{"track":{"id":"trk_1","name":"Synthetic Song","artists":[{"id":"art_1","name":"Fixture Band"}],"uri":"spotify:track:trk_1"},"played_at":"2026-09-26T10:00:00Z"}]}`))
	})
	mux.HandleFunc("/me/player/devices", func(w http.ResponseWriter, r *http.Request) {
		mustAuth(t, r)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"devices":[{"id":"dev_1","name":"Fixture Speaker","type":"Speaker","is_active":true}]}`))
	})
	mux.HandleFunc("/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"acc_1","refresh_token":"ref_1","expires_in":3600}`))
	})
	return httptest.NewServer(mux)
}

func mustAuth(t *testing.T, r *http.Request) {
	t.Helper()
	if r.Header.Get("Authorization") != "Bearer test-token" {
		t.Errorf("missing bearer auth: %q", r.Header.Get("Authorization"))
	}
}

func TestGettersAgainstFixtureServer(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	c := NewClient("test-token", srv.URL)
	ctx := context.Background()

	tracks, err := c.TopTracks(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].ID != "trk_1" || tracks[0].Artists[0].Name != "Fixture Band" {
		t.Fatalf("unexpected tracks: %+v", tracks)
	}

	artists, err := c.TopArtists(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(artists) != 1 || artists[0].ID != "art_1" {
		t.Fatalf("unexpected artists: %+v", artists)
	}

	recent, err := c.RecentlyPlayed(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(recent) != 1 || recent[0].Track.ID != "trk_1" || recent[0].PlayedAt.IsZero() {
		t.Fatalf("unexpected recent: %+v", recent)
	}

	devices, err := c.Devices(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(devices) != 1 || !devices[0].IsActive {
		t.Fatalf("unexpected devices: %+v", devices)
	}
}

func TestGetterSurfacesNon200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	defer srv.Close()
	c := NewClient("bad", srv.URL)
	if _, err := c.Devices(context.Background()); err == nil {
		t.Fatal("expected error on 401")
	}
}

func TestExchangeAndRefresh(t *testing.T) {
	srv := fixtureServer(t)
	defer srv.Close()
	ctx := context.Background()
	tok, err := Exchange(ctx, srv.URL+"/token", "cid", "code1", "verifier1")
	if err != nil {
		t.Fatal(err)
	}
	if tok.AccessToken != "acc_1" || tok.RefreshToken != "ref_1" || tok.Expired() {
		t.Fatalf("unexpected token: %+v", tok)
	}
	tok2, err := Refresh(ctx, srv.URL+"/token", "cid", "ref_1")
	if err != nil {
		t.Fatal(err)
	}
	if tok2.AccessToken == "" {
		t.Fatalf("empty refresh result: %+v", tok2)
	}
}

func TestPKCEAndAuthURL(t *testing.T) {
	v, err := NewVerifier()
	if err != nil || len(v) < 43 {
		t.Fatalf("bad verifier: %q %v", v, err)
	}
	ch := Challenge(v)
	if ch == "" || ch == v {
		t.Fatalf("bad challenge: %q", ch)
	}
	u := AuthCodeURL("cid123", "state1", ch)
	for _, want := range []string{"cid123", "state1", "S256", "user-top-read", "127.0.0.1"} {
		if !strings.Contains(u, want) {
			t.Fatalf("auth URL missing %q: %s", want, u)
		}
	}
}

func TestTokenFileRoundTrip0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONTEXTFM_CONFIG_DIR", dir)
	tok := Token{AccessToken: "a", RefreshToken: "r", ExpiresAt: time.Now().UTC().Add(time.Hour)}
	if err := SaveToken(tok); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(TokenFilePath())
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("token file perm = %o, want 600", info.Mode().Perm())
	}
	loaded, err := LoadToken()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.AccessToken != "a" {
		t.Fatalf("unexpected token: %+v", loaded)
	}
	if err := ClearToken(); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadToken(); err == nil {
		t.Fatal("expected error after clear")
	}
}

func TestTokenJSONShape(t *testing.T) {
	raw := []byte(`{"access_token":"x","refresh_token":"y","expires_at":"2026-09-26T10:00:00Z"}`)
	var tok Token
	if err := json.Unmarshal(raw, &tok); err != nil {
		t.Fatal(err)
	}
	if tok.Expired() != true {
		t.Fatal("past expiry should report expired")
	}
}
