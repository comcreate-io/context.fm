# context.fm — agent guide

Planning-phase repo. Product truth: `README.md`, `docs/PLAN.md`. Build truth: `docs/BUILD_PLAN.md`. Read all three before changing anything.

## Stack (locked)

- Go 1.25. SQLite via pure-Go driver, single file `~/.local/share/context.fm/state.db`. Tokens in OS keyring, `0600` file fallback. TUI is Bubble Tea, setup + status only.
- Dev shell is Nix: flake `go` template + `.envrc` (`use flake`) + `direnv allow`. Never `nix-env`, global `pip`, or `npm -g`. System changes belong in `~/dotfiles/`, not here.

## Commands (after scaffold exists)

- `direnv allow` once, then `go build ./...`, `go vet ./...`, `go test ./...`.
- Status/explain without TUI: `go run ./cmd/worker --help`, `contextfm status`, `contextfm explain` (names per BUILD_PLAN; check flags before running).
- Review every implementation PR: `ocr review --from main --to <branch> --format json --output /tmp/opencode/ocr-<branch>.json`. See `docs/BUILD_PLAN.md:7`.

## Hard rules

- No implementation code until BUILD_PLAN sign-off checklist passes. Docs-only until then.
- Hook fast path: `cmd/hook/` parses stdin, appends bounded event, exits 0. No network, no LLM, no blocking. p99 <200ms.
- Playback is gated: no `play` calls outside the gated controller + explicit pilot toggle + tests. Uncertainty or error → preserve current playback. Manual pause/selection/device change yields immediately.
- Secrets never in repo: no tokens, no listening history, no real transcripts, no PII fixtures. Synthetic `fixtures/` only. SQLite/config are `0600` and untracked.
- Memory stores derived context + track outcomes only. Raw prompts live in a short-TTL local spool, never in long-term tables.
- Don’t assume Spotify audio-features/analysis/recommendations. Verify live and record limits.

## Conventions

- Small PRs, descriptive branches: `feat/prompt-capture`, `fix/…`, `chore/…`, `docs/…`. Never `codex/`, `claude/`, `ai/`, `bot/` prefixes.
- Commits authored by repo `user.name`/`user.email`. No `Co-Authored-By`, no `Generated with…`, no assistant names in commits, branches, or PR titles/bodies.
- Each PR body: what was observed, what remains assumed, failure modes tested, `ocr` result.
- Go: handle every error, bound every queue/table/buffer, timeout every external call, dedup by event ID, single playback owner.

## Layout (proposed, not all created)

`cmd/hook/ cmd/worker/ cmd/tui/ internal/context/ internal/spotify/ internal/policy/ internal/memory/ internal/observe/ internal/queue/ fixtures/ .codex/hooks.json`
