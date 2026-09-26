// Package context derives a short-lived work-context estimate from a
// rolling window of submitted prompts.
//
// Heuristic only: keyword evidence aggregated with recency weighting.
// Labels describe the shape of the work (focus/explore/debug/refine/stuck),
// never the listener's emotions. A single message cannot flip an established
// estimate because it is one vote among up to WindowSize. Raw prompt text
// never leaves the caller; only the derived Snapshot is stored long-term.
package context

import (
	"strings"
	"time"
)

// Label is the derived work-context category.
type Label string

const (
	// Focus is the default when evidence is weak or mixed.
	Focus Label = "focus"
	// Explore covers open-ended brainstorming and design search.
	Explore Label = "explore"
	// Debug covers sustained failure investigation.
	Debug Label = "debug"
	// Refine covers polish, review, and cleanup passes.
	Refine Label = "refine"
	// Stuck marks repeated terse/blocked-looking signals. Contextual clue
	// only, not proof of frustration; policy must react conservatively.
	Stuck Label = "stuck"
)

// WindowSize bounds how many recent prompts form the estimate.
const WindowSize = 10

// windowDecay gives the oldest vote in the window this weight; the newest
// always weighs 1.0. Linear interpolation between the two.
const windowDecay = 0.4

// MinEvidence is the minimum aggregated weight before a non-default label
// can win. Below it the estimate stays Focus with low confidence.
const MinEvidence = 1.5

// Snapshot is the derived estimate over a window.
type Snapshot struct {
	Label         Label     `json:"label"`
	Confidence    float64   `json:"confidence"`
	EvidenceCount int       `json:"evidence_count"`
	WindowStart   time.Time `json:"window_start"`
	WindowEnd     time.Time `json:"window_end"`
}

// Input is one prompt vote. Only derived features are needed; callers pass
// the raw text and this package extracts bounded keyword evidence.
type Input struct {
	Text      string
	Timestamp time.Time
}

// Classify aggregates inputs (oldest first, capped at WindowSize) into a Snapshot.
func Classify(inputs []Input) Snapshot {
	if len(inputs) > WindowSize {
		inputs = inputs[len(inputs)-WindowSize:]
	}
	now := time.Now().UTC()
	snap := Snapshot{Label: Focus, WindowEnd: now}
	if len(inputs) == 0 {
		return snap
	}
	snap.WindowStart = inputs[0].Timestamp
	if snap.WindowStart.IsZero() {
		snap.WindowStart = now
	}
	snap.EvidenceCount = len(inputs)

	scores := map[Label]float64{}
	for i, in := range inputs {
		w := windowDecay + (1-windowDecay)*float64(i+1)/float64(len(inputs))
		// Expire stale votes: prompts older than 15 minutes fade linearly.
		if !in.Timestamp.IsZero() {
			if age := now.Sub(in.Timestamp); age > 15*time.Minute {
				over := age - 15*time.Minute
				fade := 1 - float64(over)/float64(15*time.Minute)
				if fade <= 0 {
					continue
				}
				w *= fade
			}
		}
		for _, l := range vote(in.Text) {
			scores[l] += w
		}
	}
	total := 0.0
	for _, s := range scores {
		total += s
	}
	if total < MinEvidence {
		snap.Confidence = 0
		return snap
	}
	best, bestScore := Focus, 0.0
	for l, s := range scores {
		if s > bestScore {
			best, bestScore = l, s
		}
	}
	snap.Label = best
	// Share alone overstates thin evidence (two votes can agree 1.0), so
	// discount by evidence amount: confidence grows with repeated votes.
	snap.Confidence = (bestScore / total) * (total / (total + 2))
	return snap
}

// vote maps one prompt to zero or more label votes via bounded keyword sets.
// Terse inputs (<=2 words) vote Stuck weakly: a contextual clue, not a verdict.
func vote(text string) []Label {
	t := strings.ToLower(text)
	var out []Label
	contains := func(words ...string) bool {
		for _, w := range words {
			if strings.Contains(t, w) {
				return true
			}
		}
		return false
	}
	if contains("brainstorm", "idea", "ideas", "approach", "approaches", "option", "explore", "design", "what if", "propose") {
		out = append(out, Explore)
	}
	if contains("error", "fail", "flak", "stack", "trace", "panic", "debug", "broken", "crash", "assertion", "failing", "bug") {
		out = append(out, Debug)
	}
	if contains("refactor", "polish", "cleanup", "clean up", "review", "improve", "tidy", "simplif") {
		out = append(out, Refine)
	}
	if contains("wtf", "why", "still", "again", "blocked", "stuck", "gah", "ugh", "argh") {
		out = append(out, Stuck)
	}
	if len(strings.Fields(t)) <= 2 && len(out) == 0 {
		out = append(out, Stuck)
	}
	return out
}
