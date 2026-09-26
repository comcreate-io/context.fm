package queue

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func testDir(t *testing.T) string {
	t.Helper()
	d := t.TempDir()
	t.Setenv("CONTEXTFM_DATA_DIR", d)
	return d
}

func TestAppendAndRead(t *testing.T) {
	testDir(t)
	e := NewEvent("ses_1", "turn_1", "/repo", "hello")
	if err := Append(e); err != nil {
		t.Fatal(err)
	}
	events, err := ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].EventID != "turn_1" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestDedupByEventID(t *testing.T) {
	testDir(t)
	e := NewEvent("ses_1", "turn_dup", "/repo", "same prompt twice")
	if err := Append(e); err != nil {
		t.Fatal(err)
	}
	if err := Append(e); err != nil {
		t.Fatal(err)
	}
	events, err := ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected dedup to 1, got %d", len(events))
	}
}

func TestEmptyEventIDRejected(t *testing.T) {
	testDir(t)
	if err := Append(Event{}); err == nil {
		t.Fatal("expected error for empty event_id")
	}
}

func TestPruneCapsLengthAndTTL(t *testing.T) {
	testDir(t)
	for i := 0; i < MaxEvents+50; i++ {
		e := NewEvent("ses_1", fmt.Sprintf("turn_%d", i), "/repo", "x")
		if err := Append(e); err != nil {
			t.Fatal(err)
		}
	}
	events, err := ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != MaxEvents {
		t.Fatalf("expected cap %d, got %d", MaxEvents, len(events))
	}
	if events[0].EventID == "turn_0" {
		t.Fatal("expected oldest events to be pruned")
	}
}

func TestPruneDropsExpired(t *testing.T) {
	d := testDir(t)
	old := NewEvent("ses_1", "turn_old", "/repo", "stale")
	old.Timestamp = time.Now().UTC().Add(-2 * EventTTL)
	fresh := NewEvent("ses_1", "turn_fresh", "/repo", "live")
	rewrite(t, d, []Event{old, fresh})
	if err := Prune(); err != nil {
		t.Fatal(err)
	}
	kept, err := ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0].EventID != "turn_fresh" {
		t.Fatalf("expected only fresh event, got %+v", kept)
	}
}

func TestPromptTruncated(t *testing.T) {
	testDir(t)
	long := string(make([]byte, MaxPromptChars+100))
	for i := range []byte(long) {
		long = long[:i] + "a" + long[i+1:]
	}
	e := NewEvent("ses_1", "turn_long", "/repo", long)
	if len(e.Prompt) != MaxPromptChars {
		t.Fatalf("expected truncation to %d, got %d", MaxPromptChars, len(e.Prompt))
	}
	if e.PromptLen != MaxPromptChars+100 {
		t.Fatalf("expected full length recorded, got %d", e.PromptLen)
	}
}

func rewrite(t *testing.T, dir string, events []Event) {
	t.Helper()
	f, err := os.Create(filepath.Join(dir, "events.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	for _, e := range events {
		fmt.Fprintf(f, "{\"event_id\":%q,\"session_id\":%q,\"turn_id\":%q,\"cwd\":%q,\"prompt\":%q,\"prompt_len\":%d,\"ts\":%q}\n",
			e.EventID, e.SessionID, e.TurnID, e.CWD, e.Prompt, e.PromptLen, e.Timestamp.Format(time.RFC3339Nano))
	}
}
