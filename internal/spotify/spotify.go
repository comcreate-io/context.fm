// Package spotify reads authorized listening data and drives gated playback.
//
// Milestone 2 slice: auth URL + PKCE, token exchange/refresh, and
// read-only getters (top tracks/artists, recent plays, devices) against
// the Web API base URL (overridable for tests). No playback calls yet;
// those land behind CONTEXTFM_ENABLE_PLAYBACK in the pilot slice.
// Secrets never enter the repo: client ID via CONTEXTFM_SPOTIFY_CLIENT_ID,
// tokens via keyring with 0600 file fallback (see tokenfile.go).
package spotify

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// AuthURL is the Spotify authorization endpoint.
	AuthURL = "https://accounts.spotify.com/authorize"
	// TokenURL is the Spotify token endpoint.
	TokenURL = "https://accounts.spotify.com/api/token"
	// APIBase is the Web API base.
	APIBase = "https://api.spotify.com/v1"
	// RedirectURI is the loopback callback for the local auth flow.
	RedirectURI = "http://127.0.0.1:8899/callback"
	// HTTPTimeout bounds every Spotify network call.
	HTTPTimeout = 10 * time.Second
)

// Scopes requests the minimum for taste + device + gated playback.
var Scopes = []string{
	"user-top-read",
	"user-read-recently-played",
	"user-read-playback-state",
	"user-modify-playback-state",
	"user-library-read",
	"playlist-read-private",
}

// Token is an OAuth token pair.
type Token struct {
	AccessToken  string    `json:"access_token"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	ExpiresAt    time.Time `json:"expires_at"`
}

// Expired reports whether the access token needs refresh (60s skew).
func (t Token) Expired() bool { return time.Now().UTC().Add(60 * time.Second).After(t.ExpiresAt) }

// ClientID reads the app client ID from the environment.
func ClientID() string { return strings.TrimSpace(getenv("CONTEXTFM_SPOTIFY_CLIENT_ID")) }

// AuthCodeURL builds the user-facing authorization URL.
func AuthCodeURL(clientID, state, challenge string) string {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("response_type", "code")
	v.Set("redirect_uri", RedirectURI)
	v.Set("scope", strings.Join(Scopes, " "))
	v.Set("state", state)
	v.Set("code_challenge_method", "S256")
	v.Set("code_challenge", challenge)
	return AuthURL + "?" + v.Encode()
}

// NewVerifier creates a PKCE code verifier (43-128 chars).
func NewVerifier() (string, error) {
	b := make([]byte, 48)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Challenge derives the S256 code challenge for a verifier.
func Challenge(verifier string) string {
	sum := sha256.Sum256([]byte(verifier))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

// NewState creates a CSRF state token.
func NewState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// Client calls the Web API with a bearer token.
type Client struct {
	http *http.Client
	base string
	tok  string
}

// NewClient builds a client; base defaults to APIBase (override in tests).
func NewClient(accessToken, base string) *Client {
	if base == "" {
		base = APIBase
	}
	return &Client{
		http: &http.Client{Timeout: HTTPTimeout},
		base: strings.TrimSuffix(base, "/"),
		tok:  accessToken,
	}
}

func (c *Client) get(ctx context.Context, path string, out any) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.tok)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify: GET %s: status %d", path, resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Track is the minimal track view used for familiarity ranking.
type Track struct {
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Artists    []Artist `json:"artists"`
	Popularity int      `json:"popularity"`
	URI        string   `json:"uri"`
}

// Artist is the minimal artist view.
type Artist struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PlayRecord is one recently-played entry.
type PlayRecord struct {
	Track    Track     `json:"track"`
	PlayedAt time.Time `json:"played_at"`
}

// Device is one Spotify Connect device.
type Device struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Type     string `json:"type"`
	IsActive bool   `json:"is_active"`
}

// TopTracks returns the authorized user's top tracks.
func (c *Client) TopTracks(ctx context.Context, limit int) ([]Track, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var out struct {
		Items []Track `json:"items"`
	}
	if err := c.get(ctx, fmt.Sprintf("/me/top/tracks?limit=%d", limit), &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// TopArtists returns the authorized user's top artists.
func (c *Client) TopArtists(ctx context.Context, limit int) ([]Artist, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var out struct {
		Items []Artist `json:"items"`
	}
	if err := c.get(ctx, fmt.Sprintf("/me/top/artists?limit=%d", limit), &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// RecentlyPlayed returns recently played tracks (newest first).
func (c *Client) RecentlyPlayed(ctx context.Context, limit int) ([]PlayRecord, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	var out struct {
		Items []PlayRecord `json:"items"`
	}
	if err := c.get(ctx, fmt.Sprintf("/me/player/recently-played?limit=%d", limit), &out); err != nil {
		return nil, err
	}
	return out.Items, nil
}

// Devices lists available Connect devices.
func (c *Client) Devices(ctx context.Context) ([]Device, error) {
	var out struct {
		Devices []Device `json:"devices"`
	}
	if err := c.get(ctx, "/me/player/devices", &out); err != nil {
		return nil, err
	}
	return out.Devices, nil
}

// Exchange swaps an auth code for tokens.
func Exchange(ctx context.Context, tokenURL, clientID, code, verifier string) (Token, error) {
	return tokenForm(ctx, tokenURL, url.Values{
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"redirect_uri":  {RedirectURI},
		"client_id":     {clientID},
		"code_verifier": {verifier},
	})
}

// Refresh swaps a refresh token for a new access token.
func Refresh(ctx context.Context, tokenURL, clientID, refreshToken string) (Token, error) {
	return tokenForm(ctx, tokenURL, url.Values{
		"grant_type":    {"refresh_token"},
		"refresh_token": {refreshToken},
		"client_id":     {clientID},
	})
}

func tokenForm(ctx context.Context, tokenURL string, form url.Values) (Token, error) {
	if tokenURL == "" {
		tokenURL = TokenURL
	}
	ctx, cancel := context.WithTimeout(ctx, HTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL,
		strings.NewReader(form.Encode()))
	if err != nil {
		return Token{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := (&http.Client{}).Do(req)
	if err != nil {
		return Token{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return Token{}, fmt.Errorf("spotify: token exchange: status %d", resp.StatusCode)
	}
	var raw struct {
		AccessToken  string `json:"access_token"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return Token{}, err
	}
	if raw.AccessToken == "" {
		return Token{}, fmt.Errorf("spotify: token exchange: empty access token")
	}
	return Token{
		AccessToken:  raw.AccessToken,
		RefreshToken: raw.RefreshToken,
		ExpiresAt:    time.Now().UTC().Add(time.Duration(raw.ExpiresIn) * time.Second),
	}, nil
}
