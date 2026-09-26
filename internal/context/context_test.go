package context

import (
	"strings"
	"testing"
	"time"
)

func mk(text string, age time.Duration) Input {
	return Input{Text: text, Timestamp: time.Now().UTC().Add(-age)}
}

func TestEmptyStaysFocusNoConfidence(t *testing.T) {
	s := Classify(nil)
	if s.Label != Focus || s.Confidence != 0 {
		t.Fatalf("got %+v", s)
	}
}

func TestSustainedDebuggingWins(t *testing.T) {
	var ins []Input
	for i := 0; i < 6; i++ {
		ins = append(ins, mk("this test keeps failing with a stack trace, help me debug the assertion", time.Duration(i)*time.Minute))
	}
	s := Classify(ins)
	if s.Label != Debug {
		t.Fatalf("got %+v", s)
	}
	if s.Confidence < 0.5 {
		t.Fatalf("weak confidence: %+v", s)
	}
}

func TestBrainstormingWins(t *testing.T) {
	var ins []Input
	for i := 0; i < 5; i++ {
		ins = append(ins, mk("brainstorm three approaches for the onboarding flow design", time.Duration(i)*time.Minute))
	}
	s := Classify(ins)
	if s.Label != Explore {
		t.Fatalf("got %+v", s)
	}
}

func TestSingleTrivialMessageCannotFlipDebug(t *testing.T) {
	var ins []Input
	for i := 0; i < 9; i++ {
		ins = append(ins, mk("debug this failing assertion and stack trace", time.Duration(9-i)*time.Minute))
	}
	before := Classify(ins)
	ins = append(ins, mk("typo", 0))
	after := Classify(ins)
	if before.Label != Debug || after.Label != Debug {
		t.Fatalf("flip: before=%+v after=%+v", before, after)
	}
}

func TestSingleDebugCannotFlipExplore(t *testing.T) {
	var ins []Input
	for i := 0; i < 8; i++ {
		ins = append(ins, mk("explore design options and propose approaches", time.Duration(8-i)*time.Minute))
	}
	ins = append(ins, mk("this error trace shows a failing test", 0))
	s := Classify(ins)
	if s.Label != Explore {
		t.Fatalf("single message flipped: %+v", s)
	}
}

func TestAmbiguousPromptsStayFocus(t *testing.T) {
	ins := []Input{mk("ok", 0), mk("next", 0)}
	s := Classify(ins)
	// Two terse votes total ~1.9 weight but split... assert conservative outcome:
	// either Focus or low-confidence Stuck. Confidence must stay low.
	if s.Label == Focus && s.Confidence != 0 {
		t.Fatalf("focus must carry no confidence: %+v", s)
	}
	if s.Confidence > 0.6 {
		t.Fatalf("ambiguous input overconfident: %+v", s)
	}
}

func TestStaleVotesExpire(t *testing.T) {
	ins := []Input{mk("debug stack trace failing assertion", 40*time.Minute)}
	s := Classify(ins)
	if s.Label != Focus {
		t.Fatalf("stale vote should expire: %+v", s)
	}
}

func TestWindowCapsAtSize(t *testing.T) {
	var ins []Input
	for i := 0; i < WindowSize+5; i++ {
		ins = append(ins, mk("refactor and polish this review cleanup", 0))
	}
	s := Classify(ins)
	if s.Label != Refine || s.EvidenceCount != WindowSize {
		t.Fatalf("got %+v", s)
	}
}

func TestNoEmotionClaimsInLabels(t *testing.T) {
	// Labels must stay work-descriptive; guard against emotive relabeling.
	for _, l := range []Label{Focus, Explore, Debug, Refine, Stuck} {
		s := string(l)
		for _, banned := range []string{"happy", "sad", "angry", "emotion", "mood"} {
			if strings.Contains(s, banned) {
				t.Fatalf("label %q leaks emotion language", s)
			}
		}
	}
}
