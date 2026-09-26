// Package memory learns context-to-music mappings from bounded playback
// outcomes. This file holds the pure scoring model; SQLite persistence
// lands in the pilot slice (store.go).
//
// Only companion-selected plays teach the model. Independent listening and
// manual overrides are recorded for audit but carry zero learning weight,
// so the system never trains on its own output as new taste evidence.
// Raw prompt text never enters a Record: only the derived context label.
package memory

import (
	"math"
	"time"
)

// Outcome is the observable result of a companion-selected play.
type Outcome string

const (
	// Completed is a full listen: weak positive.
	Completed Outcome = "completed"
	// Replay is a replay: stronger positive.
	Replay Outcome = "replay"
	// Save is a library save: stronger positive.
	Save Outcome = "save"
	// QuickSkip is a fast skip: negative but ambiguous.
	QuickSkip Outcome = "quick_skip"
	// ManualOverride means the listener took control: immediate yield,
	// zero learning weight.
	ManualOverride Outcome = "manual_override"
)

// SelectedBy marks who chose the track.
type SelectedBy string

const (
	// Companion marks context.fm selections: the only learning source.
	Companion SelectedBy = "companion"
	// Manual marks listener-chosen music: never learning evidence.
	Manual SelectedBy = "manual"
	// Independent marks listening outside the companion: never evidence.
	Independent SelectedBy = "independent"
)

// Record is one observed play.
type Record struct {
	TrackID      string
	ContextLabel string
	Outcome      Outcome
	ChosenBy     SelectedBy
	SessionID    string
	Timestamp    time.Time
}

// outcomeWeight maps outcomes to learning weights. Single actions are
// deliberately small; repeated context-linked evidence must accumulate
// before ranking moves.
func outcomeWeight(o Outcome) float64 {
	switch o {
	case Completed:
		return 0.2
	case Replay:
		return 0.6
	case Save:
		return 0.8
	case QuickSkip:
		return -0.3
	default:
		return 0
	}
}

// recencyHalfLife scales old evidence down: recent repeated outcomes
// outweigh stale ones.
const recencyHalfLife = 30 * 24 * time.Hour

// maxSessionWeight caps one session's total influence so a single long
// session cannot dominate the mapping.
const maxSessionWeight = 1.0

// Affinity returns the learned score in [-1, 1] for trackID in the given
// derived-context label. Empty when no companion evidence exists.
func Affinity(trackID, label string, records []Record, now time.Time) float64 {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	perSession := map[string]float64{}
	for _, r := range records {
		if r.TrackID != trackID || r.ContextLabel != label {
			continue
		}
		if r.ChosenBy != Companion {
			continue
		}
		w := outcomeWeight(r.Outcome)
		if w == 0 {
			continue
		}
		if !r.Timestamp.IsZero() {
			age := now.Sub(r.Timestamp)
			if age < 0 {
				age = 0
			}
			w *= math.Pow(2, -float64(age)/float64(recencyHalfLife))
		}
		perSession[r.SessionID] += w
	}
	total := 0.0
	for _, w := range perSession {
		if w > maxSessionWeight {
			w = maxSessionWeight
		} else if w < -maxSessionWeight {
			w = -maxSessionWeight
		}
		total += w
	}
	if total > 1 {
		total = 1
	} else if total < -1 {
		total = -1
	}
	return total
}
