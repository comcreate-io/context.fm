package memory

import (
	"testing"
	"time"
)

func rec(chosen SelectedBy, outcome Outcome, label, session string, age time.Duration) Record {
	return Record{
		TrackID: "trk_1", ContextLabel: label, Outcome: outcome,
		ChosenBy: chosen, SessionID: session,
		Timestamp: time.Now().UTC().Add(-age),
	}
}

func TestNoEvidenceIsZero(t *testing.T) {
	if a := Affinity("trk_1", "debug", nil, time.Now().UTC()); a != 0 {
		t.Fatalf("got %v", a)
	}
}

func TestSingleCompleteIsWeakPositive(t *testing.T) {
	a := Affinity("trk_1", "debug",
		[]Record{rec(Companion, Completed, "debug", "s1", time.Hour)}, time.Now().UTC())
	if a <= 0 || a > 0.25 {
		t.Fatalf("single complete should be small positive, got %v", a)
	}
}

func TestRepeatedCompletesAccumulateGradually(t *testing.T) {
	now := time.Now().UTC()
	one := Affinity("trk_1", "debug",
		[]Record{rec(Companion, Completed, "debug", "s1", time.Hour)}, now)
	many := Affinity("trk_1", "debug", []Record{
		rec(Companion, Completed, "debug", "s1", time.Hour),
		rec(Companion, Completed, "debug", "s2", 2*time.Hour),
		rec(Companion, Completed, "debug", "s3", 3*time.Hour),
	}, now)
	if many <= one {
		t.Fatalf("repeated evidence should outweigh one: one=%v many=%v", one, many)
	}
	if many > 0.7 {
		t.Fatalf("three completes should still be modest: %v", many)
	}
}

func TestReplayStrongerThanComplete(t *testing.T) {
	now := time.Now().UTC()
	c := Affinity("trk_1", "debug",
		[]Record{rec(Companion, Completed, "debug", "s1", time.Hour)}, now)
	r := Affinity("trk_1", "debug",
		[]Record{rec(Companion, Replay, "debug", "s1", time.Hour)}, now)
	if r <= c {
		t.Fatalf("replay %v should beat complete %v", r, c)
	}
}

func TestSingleSkipBarelyMoves(t *testing.T) {
	a := Affinity("trk_1", "debug",
		[]Record{rec(Companion, QuickSkip, "debug", "s1", time.Hour)}, time.Now().UTC())
	if a >= 0 || a < -0.35 {
		t.Fatalf("single skip should be small negative, got %v", a)
	}
}

func TestRepeatedSkipsOutweighOne(t *testing.T) {
	now := time.Now().UTC()
	one := Affinity("trk_1", "debug",
		[]Record{rec(Companion, QuickSkip, "debug", "s1", time.Hour)}, now)
	rep := Affinity("trk_1", "debug", []Record{
		rec(Companion, QuickSkip, "debug", "s1", time.Hour),
		rec(Companion, QuickSkip, "debug", "s2", 2*time.Hour),
		rec(Companion, QuickSkip, "debug", "s3", 3*time.Hour),
	}, now)
	if rep >= one {
		t.Fatalf("repeated skips should be more negative: one=%v rep=%v", one, rep)
	}
}

func TestManualAndIndependentIgnored(t *testing.T) {
	now := time.Now().UTC()
	a := Affinity("trk_1", "debug", []Record{
		rec(Manual, Completed, "debug", "s1", time.Hour),
		rec(Independent, Replay, "debug", "s1", time.Hour),
		rec(Companion, ManualOverride, "debug", "s1", time.Hour),
	}, now)
	if a != 0 {
		t.Fatalf("non-companion evidence must not teach, got %v", a)
	}
}

func TestLabelScoped(t *testing.T) {
	now := time.Now().UTC()
	a := Affinity("trk_1", "explore",
		[]Record{rec(Companion, Completed, "debug", "s1", time.Hour)}, now)
	if a != 0 {
		t.Fatalf("debug evidence must not leak into explore, got %v", a)
	}
}

func TestSessionCap(t *testing.T) {
	now := time.Now().UTC()
	var rs []Record
	for i := 0; i < 10; i++ {
		rs = append(rs, rec(Companion, Save, "debug", "s1", time.Hour))
	}
	a := Affinity("trk_1", "debug", rs, now)
	if a > 1.0 {
		t.Fatalf("session cap exceeded: %v", a)
	}
}

func TestRecencyOutweighsStale(t *testing.T) {
	now := time.Now().UTC()
	fresh := Affinity("trk_1", "debug",
		[]Record{rec(Companion, Completed, "debug", "s1", time.Hour)}, now)
	stale := Affinity("trk_1", "debug",
		[]Record{rec(Companion, Completed, "debug", "s1", 90*24*time.Hour)}, now)
	if fresh <= stale {
		t.Fatalf("fresh %v should beat stale %v", fresh, stale)
	}
}
