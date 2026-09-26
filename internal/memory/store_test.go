package memory

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func openTest(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("CONTEXTFM_DATA_DIR", dir)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestSaveAndListRoundTrip(t *testing.T) {
	s := openTest(t)
	r := Record{TrackID: "trk_1", ContextLabel: "debug", Outcome: Completed,
		ChosenBy: Companion, SessionID: "s1", Timestamp: time.Now().UTC()}
	if err := s.Save(r); err != nil {
		t.Fatal(err)
	}
	got, err := s.List("trk_1", "debug", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Outcome != Completed || got[0].SessionID != "s1" {
		t.Fatalf("got %+v", got)
	}
}

func TestSaveRejectsEmptyTrack(t *testing.T) {
	s := openTest(t)
	if err := s.Save(Record{}); err == nil {
		t.Fatal("expected error")
	}
}

func TestLearnedAffinityEndToEnd(t *testing.T) {
	s := openTest(t)
	now := time.Now().UTC()
	for _, sess := range []string{"s1", "s2", "s3"} {
		if err := s.Save(Record{TrackID: "trk_1", ContextLabel: "debug",
			Outcome: Completed, ChosenBy: Companion, SessionID: sess, Timestamp: now}); err != nil {
			t.Fatal(err)
		}
	}
	// Manual evidence in the store must not move affinity.
	if err := s.Save(Record{TrackID: "trk_1", ContextLabel: "debug",
		Outcome: Replay, ChosenBy: Manual, SessionID: "s9", Timestamp: now}); err != nil {
		t.Fatal(err)
	}
	a, err := s.LearnedAffinity("trk_1", "debug", now)
	if err != nil {
		t.Fatal(err)
	}
	if a <= 0.3 || a > 0.7 {
		t.Fatalf("three stored completes + manual replay should score modest positive, got %v", a)
	}
	other, err := s.LearnedAffinity("trk_1", "explore", now)
	if err != nil {
		t.Fatal(err)
	}
	if other != 0 {
		t.Fatalf("cross-label leak: %v", other)
	}
}

func TestResetClears(t *testing.T) {
	s := openTest(t)
	if err := s.Save(Record{TrackID: "t", ContextLabel: "debug", Outcome: Completed,
		ChosenBy: Companion, SessionID: "s", Timestamp: time.Now().UTC()}); err != nil {
		t.Fatal(err)
	}
	if err := s.Reset(); err != nil {
		t.Fatal(err)
	}
	n, err := s.Count()
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatalf("expected 0 after reset, got %d", n)
	}
}

func TestDBFileIs0600(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CONTEXTFM_DATA_DIR", dir)
	s, err := Open()
	if err != nil {
		t.Fatal(err)
	}
	_ = s.Close()
	info, err := os.Stat(filepath.Join(dir, "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("state.db perm = %o, want 600", info.Mode().Perm())
	}
}
