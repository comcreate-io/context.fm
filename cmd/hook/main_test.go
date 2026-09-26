package main

import (
	"strings"
	"testing"

	"github.com/comcreate-io/context.fm/internal/queue"
)

func TestRunEnqueuesPrompt(t *testing.T) {
	t.Setenv("CONTEXTFM_DATA_DIR", t.TempDir())
	in := `{"session_id":"ses_1","cwd":"/repo","hook_event_name":"UserPromptSubmit","turn_id":"turn_1","prompt":"help me debug this flaky test"}`
	if err := run(strings.NewReader(in)); err != nil {
		t.Fatal(err)
	}
	events, err := queue.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Prompt != "help me debug this flaky test" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestRunSkipsEmptyPrompt(t *testing.T) {
	t.Setenv("CONTEXTFM_DATA_DIR", t.TempDir())
	if err := run(strings.NewReader(`{"session_id":"ses_1","prompt":""}`)); err != nil {
		t.Fatal(err)
	}
	events, err := queue.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %+v", events)
	}
}

func TestRunSkipsEmptyStdin(t *testing.T) {
	t.Setenv("CONTEXTFM_DATA_DIR", t.TempDir())
	if err := run(strings.NewReader("  \n")); err != nil {
		t.Fatal(err)
	}
}

func TestRunBadJSONReturnsError(t *testing.T) {
	t.Setenv("CONTEXTFM_DATA_DIR", t.TempDir())
	if err := run(strings.NewReader("{not json")); err == nil {
		t.Fatal("expected error for malformed JSON (main converts it to exit 0)")
	}
}

func TestRunDedupsRetriedDelivery(t *testing.T) {
	t.Setenv("CONTEXTFM_DATA_DIR", t.TempDir())
	in := `{"session_id":"ses_1","turn_id":"turn_retry","prompt":"same prompt delivered twice"}`
	if err := run(strings.NewReader(in)); err != nil {
		t.Fatal(err)
	}
	if err := run(strings.NewReader(in)); err != nil {
		t.Fatal(err)
	}
	events, err := queue.ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 {
		t.Fatalf("expected dedup to 1, got %d", len(events))
	}
}
