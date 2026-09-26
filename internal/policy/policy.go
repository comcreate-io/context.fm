// Package policy ranks familiar candidates for contextual fit and
// continuity. Exact weights are experimental; the structure is stable:
// familiarity first, learned context affinity second, continuity guards
// last. Track-level energy labels are not assumed: without a validated
// attribute source, fit comes from learned context affinity only.
package policy

import (
	"fmt"
	"sort"
	"strings"
	"time"

	workctx "github.com/comcreate-io/context.fm/internal/context"
)

// Candidate is one familiar track eligible for selection.
type Candidate struct {
	TrackID     string
	Title       string
	Artist      string
	Familiarity float64 // 0-1 from listening evidence (top/recent weight)
	LastPlayed  time.Time
	PlayCount7d int
}

// Scored is a ranked candidate with human-readable reasons for explain.
type Scored struct {
	Candidate Candidate
	Score     float64
	Reasons   []string
}

// Tuning knobs. Experimental per PLAN; defaults are conservative.
var (
	// FamiliarityWeight dominates: the soundtrack must sound like the listener.
	FamiliarityWeight = 0.5
	// AffinityWeight bounds learned context fit below familiarity.
	AffinityWeight = 0.25
	// RepeatPenalty caps the cost of recent overplay.
	RepeatPenalty = 0.4
	// Cooldown is how long after a play a track is sheltered from reselection.
	Cooldown = 30 * time.Minute
	// MinShiftInterval is the minimum time between direction changes.
	MinShiftInterval = 10 * time.Minute
	// MinConfidence keeps the current direction when evidence is weak.
	MinConfidence = 0.4
	// MinMargin requires the challenger to beat the current pick clearly.
	MinMargin = 0.05
)

// Rank orders candidates best-first. affin maps trackID to learned affinity
// in [-1,1] for the current context label; missing entries score 0.
func Rank(cands []Candidate, affin map[string]float64, now time.Time) []Scored {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	out := make([]Scored, 0, len(cands))
	for _, c := range cands {
		s := Scored{Candidate: c}
		fam := clamp01(c.Familiarity)
		s.Score += FamiliarityWeight * fam
		s.Reasons = append(s.Reasons, fmt.Sprintf("familiarity %.2f", fam))

		if a, ok := affin[c.TrackID]; ok && a != 0 {
			s.Score += AffinityWeight * a
			s.Reasons = append(s.Reasons, fmt.Sprintf("context affinity %+.2f", a))
		}
		if c.PlayCount7d > 3 {
			pen := RepeatPenalty * float64(c.PlayCount7d-3) / float64(c.PlayCount7d)
			s.Score -= pen
			s.Reasons = append(s.Reasons, fmt.Sprintf("repeat penalty -%.2f (%dx/7d)", pen, c.PlayCount7d))
		}
		if !c.LastPlayed.IsZero() {
			if since := now.Sub(c.LastPlayed); since < Cooldown && since >= 0 {
				pen := RepeatPenalty * (1 - float64(since)/float64(Cooldown))
				s.Score -= pen
				s.Reasons = append(s.Reasons, fmt.Sprintf("cooldown -%.2f (played %s ago)", pen, since.Round(time.Minute)))
			}
		}
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return out[i].Candidate.TrackID < out[j].Candidate.TrackID
		}
		return out[i].Score > out[j].Score
	})
	return out
}

// ShouldShift decides whether the next track should change direction.
// Conservative by design: weak context confidence, a recent shift, or a
// thin margin all keep the current track.
func ShouldShift(snap workctx.Snapshot, ranked []Scored, currentID string, sinceShift time.Duration) (bool, string) {
	if len(ranked) == 0 {
		return false, "no candidates: preserve playback"
	}
	top := ranked[0]
	if top.Candidate.TrackID == currentID {
		return false, "top candidate is current track: preserve playback"
	}
	if snap.Confidence < MinConfidence {
		return false, fmt.Sprintf("context confidence %.2f below %.2f: preserve playback", snap.Confidence, MinConfidence)
	}
	if sinceShift < MinShiftInterval {
		return false, fmt.Sprintf("last shift %s ago (< %s): preserve playback", sinceShift.Round(time.Minute), MinShiftInterval)
	}
	if currentID != "" {
		for _, s := range ranked {
			if s.Candidate.TrackID == currentID {
				if top.Score-s.Score < MinMargin {
					return false, fmt.Sprintf("margin %.3f below %.2f: preserve playback", top.Score-s.Score, MinMargin)
				}
				break
			}
		}
	}
	return true, fmt.Sprintf("shift to %s (%s): %s", top.Candidate.Title, top.Candidate.TrackID, strings.Join(top.Reasons, "; "))
}

func clamp01(f float64) float64 {
	if f < 0 {
		return 0
	}
	if f > 1 {
		return 1
	}
	return f
}
