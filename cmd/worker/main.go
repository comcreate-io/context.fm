// Command worker polls the local prompt-event queue.
// Milestone 1 scope: prove the worker lives off the Codex critical path.
// Classification, Spotify, and policy wiring land in later milestones.
package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/comcreate-io/context.fm/internal/queue"
)

func main() {
	once := flag.Bool("once", false, "drain status once and exit")
	interval := flag.Duration("interval", 2*time.Second, "poll interval in daemon mode")
	flag.Parse()

	if err := run(*once, *interval); err != nil {
		fmt.Fprintln(os.Stderr, "contextfm-worker:", err)
		os.Exit(1)
	}
}

func run(once bool, interval time.Duration) error {
	for {
		events, err := queue.ReadAll()
		if err != nil {
			return err
		}
		seen := map[string]int{}
		for _, e := range events {
			seen[e.SessionID]++
		}
		fmt.Printf("queue_depth=%d sessions=%d\n", len(events), len(seen))
		if once {
			return nil
		}
		time.Sleep(interval)
	}
}
