// Command worker polls the local prompt-event queue.
// Milestone 1 scope: prove the worker lives off the Codex critical path.
// Classification, Spotify, and policy wiring land in later milestones.
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"time"

	workctx "github.com/comcreate-io/context.fm/internal/context"
	"github.com/comcreate-io/context.fm/internal/memory"
	"github.com/comcreate-io/context.fm/internal/policy"
	"github.com/comcreate-io/context.fm/internal/queue"
)

func main() {
	once := flag.Bool("once", false, "drain status once and exit")
	interval := flag.Duration("interval", 2*time.Second, "poll interval in daemon mode")
	explain := flag.Bool("explain", false, "print context + ranking explanation and exit")
	candsPath := flag.String("candidates", "fixtures/spotify/candidates.json", "candidate pool JSON for --explain")
	outcomesPath := flag.String("outcomes", "", "playback outcome JSON for --explain affinity (optional)")
	current := flag.String("current", "", "current track ID for --explain shift verdict")
	sinceShift := flag.Duration("since-shift", time.Hour, "time since last direction change for --explain")
	flag.Parse()

	var err error
	switch {
	case *explain:
		err = runExplain(*candsPath, *outcomesPath, *current, *sinceShift)
	case *once:
		err = runOnce()
	default:
		err = runLoop(*interval)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "contextfm-worker:", err)
		os.Exit(1)
	}
}

func runOnce() error {
	events, err := queue.ReadAll()
	if err != nil {
		return err
	}
	seen := map[string]int{}
	for _, e := range events {
		seen[e.SessionID]++
	}
	fmt.Printf("queue_depth=%d sessions=%d\n", len(events), len(seen))
	return nil
}

func runLoop(interval time.Duration) error {
	for {
		if err := runOnce(); err != nil {
			return err
		}
		time.Sleep(interval)
	}
}

// candidateFile mirrors the fixture JSON shape.
type candidateFile struct {
	TrackID     string  `json:"track_id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Familiarity float64 `json:"familiarity"`
	LastPlayed  string  `json:"last_played,omitempty"`
	PlayCount7d int     `json:"play_count_7d"`
}

// outcomeFile mirrors memory.Record JSON.
type outcomeFile struct {
	TrackID      string `json:"track_id"`
	ContextLabel string `json:"context_label"`
	Outcome      string `json:"outcome"`
	ChosenBy     string `json:"chosen_by"`
	SessionID    string `json:"session_id"`
}

// runExplain renders the local explanation view: derived context over the
// queued prompts, candidate ranking with reasons, and the preserve/shift
// verdict. Read-only; never touches playback.
func runExplain(candsPath, outcomesPath, currentID string, sinceShift time.Duration) error {
	events, err := queue.ReadAll()
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	inputs := make([]workctx.Input, 0, len(events))
	for _, e := range events {
		ts := e.Timestamp
		if ts.IsZero() {
			ts = now
		}
		inputs = append(inputs, workctx.Input{Text: e.Prompt, Timestamp: ts})
	}
	snap := workctx.Classify(inputs)
	fmt.Printf("context=%s confidence=%.2f evidence=%d\n", snap.Label, snap.Confidence, snap.EvidenceCount)

	raw, err := os.ReadFile(candsPath)
	if err != nil {
		return fmt.Errorf("read candidates: %w", err)
	}
	var files []candidateFile
	if err := json.Unmarshal(raw, &files); err != nil {
		return fmt.Errorf("parse candidates: %w", err)
	}
	cands := make([]policy.Candidate, 0, len(files))
	for _, f := range files {
		c := policy.Candidate{
			TrackID: f.TrackID, Title: f.Title, Artist: f.Artist,
			Familiarity: f.Familiarity, PlayCount7d: f.PlayCount7d,
		}
		if f.LastPlayed != "" {
			if t, err := time.Parse(time.RFC3339, f.LastPlayed); err == nil {
				c.LastPlayed = t
			}
		}
		cands = append(cands, c)
	}

	affin := map[string]float64{}
	if outcomesPath != "" {
		raw, err := os.ReadFile(outcomesPath)
		if err != nil {
			return fmt.Errorf("read outcomes: %w", err)
		}
		var outs []outcomeFile
		if err := json.Unmarshal(raw, &outs); err != nil {
			return fmt.Errorf("parse outcomes: %w", err)
		}
		var records []memory.Record
		for _, o := range outs {
			records = append(records, memory.Record{
				TrackID: o.TrackID, ContextLabel: o.ContextLabel,
				Outcome: memory.Outcome(o.Outcome), ChosenBy: memory.SelectedBy(o.ChosenBy),
				SessionID: o.SessionID, Timestamp: now,
			})
		}
		for _, c := range cands {
			if a := memory.Affinity(c.TrackID, string(snap.Label), records, now); a != 0 {
				affin[c.TrackID] = a
			}
		}
	}

	ranked := policy.Rank(cands, affin, now)
	for i, s := range ranked {
		fmt.Printf("%d. %s — %s [%s] score=%.3f (%s)\n",
			i+1, s.Candidate.Title, s.Candidate.Artist, s.Candidate.TrackID,
			s.Score, joinReasons(s.Reasons))
	}
	shift, reason := policy.ShouldShift(snap, ranked, currentID, sinceShift)
	fmt.Printf("verdict: shift=%v: %s\n", shift, reason)
	return nil
}

func joinReasons(rs []string) string {
	out := ""
	for i, r := range rs {
		if i > 0 {
			out += "; "
		}
		out += r
	}
	return out
}
