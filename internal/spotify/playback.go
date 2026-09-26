// Gated playback control. All mutating calls require the pilot toggle:
// CONTEXTFM_ENABLE_PLAYBACK=1 plus an explicit device ID selected during
// setup. Anything else is refused without touching the API. On any doubt
// (no gate, no device, expired auth, API error) the controller preserves
// existing playback: it never retries a mutation blindly.
package spotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
)

// gateEnv is the explicit pilot activation switch.
const gateEnv = "CONTEXTFM_ENABLE_PLAYBACK"

// GateEnabled reports whether autonomous playback changes are allowed.
func GateEnabled() bool { return os.Getenv(gateEnv) == "1" }

// Controller issues playback mutations on one explicit device.
type Controller struct {
	client *Client
	device string
}

// NewController binds a client to the setup-selected device ID.
// Empty device IDs are rejected: playback never targets an implicit device.
func NewController(c *Client, deviceID string) (*Controller, error) {
	if strings.TrimSpace(deviceID) == "" {
		return nil, fmt.Errorf("spotify: playback requires an explicit device id")
	}
	return &Controller{client: c, device: deviceID}, nil
}

// checkGate refuses mutations unless the pilot toggle is on.
func checkGate() error {
	if !GateEnabled() {
		return fmt.Errorf("spotify: playback gate closed (set %s=1 for pilot)", gateEnv)
	}
	return nil
}

func (ctl *Controller) put(ctx context.Context, path string, body any) error {
	if err := checkGate(); err != nil {
		return err
	}
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	ctx, cancel := context.WithTimeout(ctx, HTTPTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, ctl.client.base+path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+ctl.client.tok)
	req.Header.Set("Content-Type", "application/json")
	resp, err := ctl.client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("spotify: PUT %s: status %d", path, resp.StatusCode)
	}
	return nil
}

// QueueTrack appends a track URI to the device queue (boundary-friendly:
// the current track is never interrupted).
func (ctl *Controller) QueueTrack(ctx context.Context, trackURI string) error {
	if err := checkGate(); err != nil {
		return err
	}
	if !strings.HasPrefix(trackURI, "spotify:track:") {
		return fmt.Errorf("spotify: refusing to queue non-track uri %q", trackURI)
	}
	ctx, cancel := context.WithTimeout(ctx, HTTPTimeout)
	defer cancel()
	u := fmt.Sprintf("%s/me/player/queue?uri=%s&device_id=%s",
		ctl.client.base, trackURI, ctl.device)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+ctl.client.tok)
	resp, err := ctl.client.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("spotify: queue: status %d", resp.StatusCode)
	}
	return nil
}

// Pause halts playback on the bound device (used when yielding to manual control).
func (ctl *Controller) Pause(ctx context.Context) error {
	return ctl.put(ctx, "/me/player/pause?device_id="+ctl.device, nil)
}

// NowPlaying is the minimal currently-playing view the observer needs.
type NowPlaying struct {
	TrackID    string
	Title      string
	DeviceID   string
	DeviceName string
	IsPlaying  bool
	ProgressMs int
	DurationMs int
}

// Current fetches the currently playing track. Read-only: never gated,
// since observation must work in observe-only mode too.
func (c *Client) Current(ctx context.Context) (NowPlaying, error) {
	var out struct {
		Item struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			DurationMs int    `json:"duration_ms"`
		} `json:"item"`
		Device struct {
			ID   string `json:"id"`
			Name string `json:"name"`
		} `json:"device"`
		IsPlaying  bool `json:"is_playing"`
		ProgressMs int  `json:"progress_ms"`
	}
	ctx, cancel := context.WithTimeout(ctx, HTTPTimeout)
	defer cancel()
	if err := c.get(ctx, "/me/player/currently-playing", &out); err != nil {
		return NowPlaying{}, err
	}
	return NowPlaying{
		TrackID: out.Item.ID, Title: out.Item.Name,
		DeviceID: out.Device.ID, DeviceName: out.Device.Name,
		IsPlaying: out.IsPlaying, ProgressMs: out.ProgressMs,
		DurationMs: out.Item.DurationMs,
	}, nil
}

// EnsureValidToken loads the stored token, refreshing it when expired, and
// returns a client bound to it. Expired auth surfaces as an error and never
// triggers playback: callers preserve current playback on failure.
func EnsureValidToken(ctx context.Context, tokenURL, clientID string) (*Client, error) {
	tok, err := LoadToken()
	if err != nil {
		return nil, fmt.Errorf("spotify: no stored token: %w", err)
	}
	if tok.Expired() {
		if tok.RefreshToken == "" {
			return nil, fmt.Errorf("spotify: access token expired with no refresh token")
		}
		tok, err = Refresh(ctx, tokenURL, clientID, tok.RefreshToken)
		if err != nil {
			return nil, fmt.Errorf("spotify: refresh failed: %w", err)
		}
		// Preserve the refresh token when the response omits it.
		if tok.RefreshToken == "" {
			old, lerr := LoadToken()
			if lerr == nil {
				tok.RefreshToken = old.RefreshToken
			}
		}
		if err := SaveToken(tok); err != nil {
			return nil, err
		}
	}
	return NewClient(tok.AccessToken, ""), nil
}
