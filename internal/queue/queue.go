// Package queue is the bounded local event queue between the
// Codex UserPromptSubmit hook adapter and the background worker.
//
// Storage is JSONL at $CONTEXTFM_DATA_DIR/events.jsonl
// (default ~/.local/share/context.fm). The file is capped at MaxEvents
// lines and TTL; Append dedups by EventID so retried hook deliveries
// cannot double-count a prompt.
package queue

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// MaxPromptChars bounds how much prompt text is kept per event.
	MaxPromptChars = 4000
	// MaxEvents bounds the queue file length.
	MaxEvents = 200
	// EventTTL bounds how long an unprocessed event is kept.
	EventTTL = 24 * time.Hour
	// MaxStdinBytes bounds hook stdin reads.
	MaxStdinBytes = 1 << 20
)

// Event is one submitted-prompt delivery.
type Event struct {
	EventID   string    `json:"event_id"`
	SessionID string    `json:"session_id"`
	TurnID    string    `json:"turn_id,omitempty"`
	CWD       string    `json:"cwd,omitempty"`
	Prompt    string    `json:"prompt,omitempty"`
	PromptLen int       `json:"prompt_len"`
	Timestamp time.Time `json:"ts"`
}

// NewEvent builds an Event, truncating prompt text to MaxPromptChars.
// TurnID doubles as the dedup key when present; otherwise a random ID is made.
func NewEvent(sessionID, turnID, cwd, prompt string) Event {
	fullLen := len(prompt)
	if len(prompt) > MaxPromptChars {
		prompt = prompt[:MaxPromptChars]
	}
	id := strings.TrimSpace(turnID)
	if id == "" {
		id = "evt_" + randHex(8)
	}
	return Event{
		EventID:   id,
		SessionID: sessionID,
		TurnID:    turnID,
		CWD:       cwd,
		Prompt:    prompt,
		PromptLen: fullLen,
		Timestamp: time.Now().UTC(),
	}
}

// DataDir resolves the local state dir, honoring CONTEXTFM_DATA_DIR for tests.
func DataDir() string {
	if d := strings.TrimSpace(os.Getenv("CONTEXTFM_DATA_DIR")); d != "" {
		return d
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return filepath.Join(os.TempDir(), "context.fm")
	}
	if xdg := strings.TrimSpace(os.Getenv("XDG_DATA_HOME")); xdg != "" {
		return filepath.Join(xdg, "context.fm")
	}
	return filepath.Join(home, ".local", "share", "context.fm")
}

// QueuePath is the JSONL queue file.
func QueuePath() string { return filepath.Join(DataDir(), "events.jsonl") }

// Append adds e unless its EventID is already queued. It prunes after write.
func Append(e Event) error {
	if strings.TrimSpace(e.EventID) == "" {
		return fmt.Errorf("queue: empty event_id")
	}
	if err := os.MkdirAll(DataDir(), 0o700); err != nil {
		return err
	}
	if dup, err := contains(QueuePath(), e.EventID); err == nil && dup {
		return nil
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}
	f, err := os.OpenFile(QueuePath(), os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if err := json.NewEncoder(f).Encode(e); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return Prune()
}

// ReadAll returns queued events oldest-first. Missing file yields empty slice.
func ReadAll() ([]Event, error) {
	f, err := os.Open(QueuePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()
	var out []Event
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64*1024), MaxStdinBytes)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue // skip corrupt lines, keep worker alive
		}
		out = append(out, e)
	}
	return out, sc.Err()
}

// Prune drops expired events and caps length at MaxEvents (keeps newest).
func Prune() error {
	events, err := ReadAll()
	if err != nil {
		return err
	}
	cutoff := time.Now().UTC().Add(-EventTTL)
	kept := events[:0]
	for _, e := range events {
		if e.Timestamp.IsZero() || e.Timestamp.After(cutoff) {
			kept = append(kept, e)
		}
	}
	if len(kept) > MaxEvents {
		kept = kept[len(kept)-MaxEvents:]
	}
	if len(kept) == len(events) {
		return nil
	}
	tmp := QueuePath() + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	enc := json.NewEncoder(f)
	for _, e := range kept {
		if err := enc.Encode(e); err != nil {
			_ = f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, QueuePath())
}

func contains(path, eventID string) (bool, error) {
	events, err := ReadAll()
	if err != nil {
		return false, err
	}
	for _, e := range events {
		if e.EventID == eventID {
			return true, nil
		}
	}
	return false, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%d", time.Now().UTC().UnixNano())
	}
	return hex.EncodeToString(b)
}
