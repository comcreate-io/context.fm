package policy

import (
	"strings"
	"testing"
	"time"

	workctx "github.com/comcreate-io/context.fm/internal/context"
)

func TestFamiliarWins(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{
		{TrackID: "a", Familiarity: 0.9},
		{TrackID: "b", Familiarity: 0.2},
	}, nil, now)
	if ranked[0].Candidate.TrackID != "a" {
		t.Fatalf("got %+v", ranked)
	}
}

func TestAffinityLiftsFamiliar(t *testing.T) {
	now := time.Now().UTC()
	cands := []Candidate{
		{TrackID: "a", Familiarity: 0.7},
		{TrackID: "b", Familiarity: 0.6},
	}
	base := Rank(cands, nil, now)
	if base[0].Candidate.TrackID != "a" {
		t.Fatalf("baseline wrong: %+v", base)
	}
	with := Rank(cands, map[string]float64{"b": 1.0}, now)
	if with[0].Candidate.TrackID != "b" {
		t.Fatalf("affinity should lift b: %+v", with)
	}
}

func TestRepeatPenaltyDethronesOverplayed(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{
		{TrackID: "a", Familiarity: 0.8, PlayCount7d: 12},
		{TrackID: "b", Familiarity: 0.7},
	}, nil, now)
	if ranked[0].Candidate.TrackID != "b" {
		t.Fatalf("overplayed track should yield: %+v", ranked)
	}
}

func TestCooldownSheltersRecentPlay(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{
		{TrackID: "a", Familiarity: 0.9, LastPlayed: now.Add(-5 * time.Minute)},
		{TrackID: "b", Familiarity: 0.85},
	}, nil, now)
	if ranked[0].Candidate.TrackID != "b" {
		t.Fatalf("recent play should be sheltered: %+v", ranked)
	}
}

func TestWeakConfidencePreserves(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{{TrackID: "b", Familiarity: 0.9}}, nil, now)
	shift, reason := ShouldShift(workctx.Snapshot{Label: workctx.Focus, Confidence: 0.1}, ranked, "a", time.Hour)
	if shift {
		t.Fatalf("weak confidence must preserve: %s", reason)
	}
}

func TestRecentShiftPreserves(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{{TrackID: "b", Familiarity: 0.9}}, nil, now)
	snap := workctx.Snapshot{Label: workctx.Debug, Confidence: 0.9}
	shift, _ := ShouldShift(snap, ranked, "a", 2*time.Minute)
	if shift {
		t.Fatal("recent shift must preserve")
	}
}

func TestThinMarginPreserves(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{
		{TrackID: "b", Familiarity: 0.81},
		{TrackID: "a", Familiarity: 0.80},
	}, nil, now)
	snap := workctx.Snapshot{Label: workctx.Debug, Confidence: 0.9}
	shift, _ := ShouldShift(snap, ranked, "a", time.Hour)
	if shift {
		t.Fatal("thin margin must preserve")
	}
}

func TestClearShiftHasReason(t *testing.T) {
	now := time.Now().UTC()
	ranked := Rank([]Candidate{
		{TrackID: "b", Familiarity: 0.9},
		{TrackID: "a", Familiarity: 0.2},
	}, nil, now)
	snap := workctx.Snapshot{Label: workctx.Debug, Confidence: 0.9}
	shift, reason := ShouldShift(snap, ranked, "a", time.Hour)
	if !shift || !strings.Contains(reason, "b") {
		t.Fatalf("shift=%v reason=%q", shift, reason)
	}
}

func TestEmptyCandidatesPreserves(t *testing.T) {
	shift, _ := ShouldShift(workctx.Snapshot{Confidence: 1}, nil, "a", time.Hour)
	if shift {
		t.Fatal("no candidates must preserve")
	}
}
