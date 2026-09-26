package observe

import (
	"testing"
	"time"

	"github.com/comcreate-io/context.fm/internal/memory"
	"github.com/comcreate-io/context.fm/internal/spotify"
)

func sample(track, device string, playing bool, progress, duration int) spotify.NowPlaying {
	return spotify.NowPlaying{TrackID: track, Title: track, DeviceID: device,
		IsPlaying: playing, ProgressMs: progress, DurationMs: duration}
}

func TestCompletedBoundary(t *testing.T) {
	o := New("dev_1", "debug", "s1")
	at := time.Now().UTC()
	o.NoteSelected("trk_a")
	if r := o.Observe(sample("trk_a", "dev_1", true, 1000, 200000), at); r != nil {
		t.Fatalf("first sight must be silent: %+v", r)
	}
	r := o.Observe(sample("trk_b", "dev_1", true, 0, 200000), at.Add(3*time.Minute))
	if r == nil || r.Outcome != memory.Completed || r.ChosenBy != memory.Companion {
		t.Fatalf("got %+v", r)
	}
}

func TestQuickSkipBoundary(t *testing.T) {
	o := New("dev_1", "debug", "s1")
	at := time.Now().UTC()
	o.NoteSelected("trk_a")
	_ = o.Observe(sample("trk_a", "dev_1", true, 1000, 200000), at)
	r := o.Observe(sample("trk_b", "dev_1", true, 0, 200000), at.Add(10*time.Second))
	if r == nil || r.Outcome != memory.QuickSkip {
		t.Fatalf("got %+v", r)
	}
}

func TestIndependentListeningNotLearned(t *testing.T) {
	o := New("dev_1", "debug", "s1")
	at := time.Now().UTC()
	_ = o.Observe(sample("trk_x", "dev_1", true, 1000, 200000), at)
	r := o.Observe(sample("trk_y", "dev_1", true, 0, 200000), at.Add(3*time.Minute))
	if r == nil || r.ChosenBy != memory.Independent {
		t.Fatalf("got %+v", r)
	}
}

func TestPauseYieldsManualOverride(t *testing.T) {
	o := New("dev_1", "debug", "s1")
	at := time.Now().UTC()
	o.NoteSelected("trk_a")
	_ = o.Observe(sample("trk_a", "dev_1", true, 60000, 200000), at)
	r := o.Observe(sample("trk_a", "dev_1", false, 60000, 200000), at.Add(time.Minute))
	if r == nil || r.Outcome != memory.ManualOverride {
		t.Fatalf("got %+v", r)
	}
}

func TestDeviceMoveYields(t *testing.T) {
	o := New("dev_1", "debug", "s1")
	at := time.Now().UTC()
	_ = o.Observe(sample("trk_a", "dev_1", true, 60000, 200000), at)
	r := o.Observe(sample("trk_a", "dev_other", true, 61000, 200000), at.Add(time.Minute))
	if r == nil || r.Outcome != memory.ManualOverride {
		t.Fatalf("got %+v", r)
	}
}

func TestSteadyStateSilent(t *testing.T) {
	o := New("dev_1", "debug", "s1")
	at := time.Now().UTC()
	_ = o.Observe(sample("trk_a", "dev_1", true, 1000, 200000), at)
	if r := o.Observe(sample("trk_a", "dev_1", true, 60000, 200000), at.Add(time.Minute)); r != nil {
		t.Fatalf("steady playback must be silent: %+v", r)
	}
}

func TestOwnerLockExclusive(t *testing.T) {
	t.Setenv("CONTEXTFM_DATA_DIR", t.TempDir())
	a, err := AcquireOwner()
	if err != nil {
		t.Fatal(err)
	}
	defer a.Release()
	if _, err := AcquireOwner(); err == nil {
		t.Fatal("second owner must fail")
	}
}
