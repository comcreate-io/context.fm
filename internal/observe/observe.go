// Package observe turns raw playback samples into bounded learning records
// and detects manual takeovers. Direction of safety: anything ambiguous
// becomes a yield (manual override), never a preference signal.
package observe

import (
	"time"

	"github.com/comcreate-io/context.fm/internal/memory"
	"github.com/comcreate-io/context.fm/internal/spotify"
)

// Thresholds for outcome classification.
const (
	// CompleteRatio marks a completed listen when progress passed this
	// fraction of duration at track change.
	CompleteRatio = 0.9
	// SkipWindow marks a quick skip when the track changed within this
	// time of first sight with low progress.
	SkipWindow = 30 * time.Second
)

// Observer tracks one bound device. Not goroutine-safe; the daemon is the
// only caller.
type Observer struct {
	device   string
	label    string
	session  string
	last     spotify.NowPlaying
	first    time.Time
	selected map[string]bool
}

// New binds an observer to the setup-selected device and current derived
// context label. SetLabel updates the label as context evolves.
func New(deviceID, contextLabel, sessionID string) *Observer {
	return &Observer{device: deviceID, label: contextLabel, session: sessionID,
		selected: map[string]bool{}}
}

// SetLabel updates the derived context for subsequent records.
func (o *Observer) SetLabel(label, sessionID string) {
	o.label, o.session = label, sessionID
}

// NoteSelected registers a companion-selected track so later transitions
// are attributed to the companion instead of independent listening.
func (o *Observer) NoteSelected(trackID string) {
	if trackID != "" {
		o.selected[trackID] = true
	}
}

// Observe folds one sample in and returns a record when a track boundary
// or takeover is detected. Nil means no event.
func (o *Observer) Observe(cur spotify.NowPlaying, at time.Time) *memory.Record {
	defer func() { o.last = cur }()
	prev := o.last

	// Device moved or unknown device: listener took control elsewhere.
	if cur.DeviceID != "" && cur.DeviceID != o.device {
		return &memory.Record{TrackID: prev.TrackID, ContextLabel: o.label,
			Outcome: memory.ManualOverride, ChosenBy: memory.Companion,
			SessionID: o.session, Timestamp: at}
	}
	// Pause on the bound device that we did not issue: yield.
	if prev.IsPlaying && !cur.IsPlaying && prev.TrackID == cur.TrackID && cur.TrackID != "" {
		return &memory.Record{TrackID: cur.TrackID, ContextLabel: o.label,
			Outcome: memory.ManualOverride, ChosenBy: memory.Companion,
			SessionID: o.session, Timestamp: at}
	}
	// Track boundary: judge the finished track.
	if prev.TrackID != "" && cur.TrackID != prev.TrackID {
		chosen := memory.Independent
		if o.selected[prev.TrackID] {
			chosen = memory.Companion
		}
		outcome := memory.Completed
		if chosen == memory.Companion {
			switch {
			case prev.DurationMs > 0 && float64(prev.ProgressMs)/float64(prev.DurationMs) >= CompleteRatio:
				outcome = memory.Completed
			case !o.first.IsZero() && at.Sub(o.first) <= SkipWindow:
				outcome = memory.QuickSkip
			default:
				outcome = memory.Completed
			}
		} else {
			// Independent listening is recorded for audit, never learned from.
			return &memory.Record{TrackID: prev.TrackID, ContextLabel: o.label,
				Outcome: memory.Completed, ChosenBy: memory.Independent,
				SessionID: o.session, Timestamp: at}
		}
		o.first = at
		return &memory.Record{TrackID: prev.TrackID, ContextLabel: o.label,
			Outcome: outcome, ChosenBy: chosen, SessionID: o.session, Timestamp: at}
	}
	// First sight of a track starts the skip window.
	if cur.TrackID != "" && cur.TrackID != prev.TrackID {
		o.first = at
	}
	return nil
}
