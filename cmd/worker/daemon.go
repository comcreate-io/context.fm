package main

import (
	"context"
	"fmt"
	"os"
	"time"

	workctx "github.com/comcreate-io/context.fm/internal/context"
	"github.com/comcreate-io/context.fm/internal/memory"
	"github.com/comcreate-io/context.fm/internal/observe"
	"github.com/comcreate-io/context.fm/internal/policy"
	"github.com/comcreate-io/context.fm/internal/queue"
	"github.com/comcreate-io/context.fm/internal/spotify"
)

// daemon is the autonomous pilot loop: classify → rank → gated queue →
// observe → learn. Every step degrades to preservation on error.
func runDaemon(deviceID string, interval time.Duration) error {
	lock, err := observe.AcquireOwner()
	if err != nil {
		return err
	}
	defer lock.Release()

	store, err := memory.Open()
	if err != nil {
		return err
	}
	defer store.Close()

	obs := observe.New(deviceID, string(workctx.Focus), "daemon")
	var lastQueued string
	lastShift := time.Now().UTC().Add(-time.Hour)

	for {
		if err := tick(deviceID, store, obs, &lastQueued, &lastShift); err != nil {
			fmt.Fprintln(os.Stderr, "contextfm-worker: tick:", err)
		}
		time.Sleep(interval)
	}
}

func tick(deviceID string, store *memory.Store, obs *observe.Observer, lastQueued *string, lastShift *time.Time) error {
	now := time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	events, err := queue.ReadAll()
	if err != nil {
		return err
	}
	inputs := make([]workctx.Input, 0, len(events))
	for _, e := range events {
		ts := e.Timestamp
		if ts.IsZero() {
			ts = now
		}
		inputs = append(inputs, workctx.Input{Text: e.Prompt, Timestamp: ts})
	}
	snap := workctx.Classify(inputs)
	sess := "daemon"
	if len(events) > 0 {
		sess = events[len(events)-1].SessionID
	}
	obs.SetLabel(string(snap.Label), sess)

	// Observe-only unless the gate, device, and auth all check out.
	if !spotify.GateEnabled() || deviceID == "" {
		return observeOnce(ctx, deviceID, store, obs, now)
	}
	client, err := spotify.EnsureValidToken(ctx, "", spotify.ClientID())
	if err != nil {
		return observeOnce(ctx, deviceID, store, obs, now)
	}
	cands, err := familiarPool(ctx, client)
	if err != nil {
		return observeOnce(ctx, deviceID, store, obs, now)
	}
	affin := map[string]float64{}
	for _, c := range cands {
		if a, err := store.LearnedAffinity(c.TrackID, string(snap.Label), now); err == nil && a != 0 {
			affin[c.TrackID] = a
		}
	}
	ranked := policy.Rank(cands, affin, now)
	if shift, _ := policy.ShouldShift(snap, ranked, *lastQueued, now.Sub(*lastShift)); shift {
		top := ranked[0]
		ctl, err := spotify.NewController(client, deviceID)
		if err != nil {
			return err
		}
		uri := "spotify:track:" + top.Candidate.TrackID
		if err := ctl.QueueTrack(ctx, uri); err != nil {
			return err
		}
		obs.NoteSelected(top.Candidate.TrackID)
		*lastQueued, *lastShift = top.Candidate.TrackID, now
		fmt.Printf("queued %s [%s] context=%s conf=%.2f\n",
			top.Candidate.Title, top.Candidate.TrackID, snap.Label, snap.Confidence)
	}
	return observeOnce(ctx, deviceID, store, obs, now)
}

// observeOnce samples playback and stores any boundary/takeover record.
// Observation errors (expired auth, missing device, offline) are advisory:
// the daemon logs and preserves rather than retrying.
func observeOnce(ctx context.Context, deviceID string, store *memory.Store, obs *observe.Observer, now time.Time) error {
	if deviceID == "" {
		return nil
	}
	client, err := spotify.EnsureValidToken(ctx, "", spotify.ClientID())
	if err != nil {
		return nil // no auth yet: stay quiet
	}
	cur, err := client.Current(ctx)
	if err != nil {
		return nil // device gone / offline / rate limited: preserve
	}
	if rec := obs.Observe(cur, now); rec != nil {
		if err := store.Save(*rec); err != nil {
			return err
		}
		fmt.Printf("observed %s outcome=%s by=%s\n", rec.TrackID, rec.Outcome, rec.ChosenBy)
	}
	return nil
}

// familiarPool builds candidates from live top/recent listening evidence.
// Rank order sets familiarity; nothing is invented.
func familiarPool(ctx context.Context, client *spotify.Client) ([]policy.Candidate, error) {
	top, err := client.TopTracks(ctx, 20)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var cands []policy.Candidate
	for i, t := range top {
		seen[t.ID] = true
		artist := ""
		if len(t.Artists) > 0 {
			artist = t.Artists[0].Name
		}
		cands = append(cands, policy.Candidate{
			TrackID: t.ID, Title: t.Name, Artist: artist,
			Familiarity: 1 - float64(i)/float64(len(top))*0.5,
		})
	}
	recent, err := client.RecentlyPlayed(ctx, 20)
	if err != nil {
		return nil, err
	}
	for _, r := range recent {
		if seen[r.Track.ID] {
			continue
		}
		seen[r.Track.ID] = true
		artist := ""
		if len(r.Track.Artists) > 0 {
			artist = r.Track.Artists[0].Name
		}
		cands = append(cands, policy.Candidate{
			TrackID: r.Track.ID, Title: r.Track.Name, Artist: artist,
			Familiarity: 0.4, LastPlayed: r.PlayedAt,
		})
	}
	return cands, nil
}
