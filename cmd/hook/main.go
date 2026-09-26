// Command hook is the Codex UserPromptSubmit adapter.
//
// Contract: read one JSON object on stdin, append a bounded queue event,
// exit 0. stdout stays silent on success so Codex never waits on us;
// diagnostics go to stderr. Never exit nonzero: a hook failure must not
// block the coding turn.
package main

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/comcreate-io/context.fm/internal/queue"
)

// hookInput mirrors the documented UserPromptSubmit fields we care about.
// Unknown fields are ignored for forward compatibility.
type hookInput struct {
	SessionID   string `json:"session_id"`
	Transcript  string `json:"transcript_path"`
	CWD         string `json:"cwd"`
	HookEvent   string `json:"hook_event_name"`
	Model       string `json:"model"`
	TurnID      string `json:"turn_id"`
	Prompt      string `json:"prompt"`
	PermMode    string `json:"permission_mode"`
}

func main() {
	if err := run(os.Stdin); err != nil {
		// Never block the turn: report and exit 0.
		fmt.Fprintln(os.Stderr, "contextfm-hook:", err)
	}
}

func run(r io.Reader) error {
	raw, err := io.ReadAll(io.LimitReader(r, queue.MaxStdinBytes))
	if err != nil {
		return err
	}
	if len(strings.TrimSpace(string(raw))) == 0 {
		return nil
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		return err
	}
	if strings.TrimSpace(in.Prompt) == "" {
		return nil
	}
	e := queue.NewEvent(in.SessionID, in.TurnID, in.CWD, in.Prompt)
	return queue.Append(e)
}
