# context.fm — Build plan (draft for sign-off, no implementation yet)

Status: planning only. This doc locks stack + scope before any `feat/` branch.
Source of truth for product: `README.md`, `docs/PLAN.md`.

## 0. Decisions locked

- Runtime: Go 1.25 (single static binaries for hook + worker + TUI, fits NixOS).
- Dev shell: `nix flake init -t ~/dotfiles#go` + `.envrc` (`use flake`) + `direnv allow`. No `nix-env`, no global `pip`/`npm -g`.
- Memory store: SQLite via pure-Go driver (no cgo), single file `~/.local/share/context.fm/state.db`. Bounded tables, no raw prompt text in long-term tables.
- Secrets: OS keyring first (`zalando/go-keyring`), fallback `0600` file under `~/.config/context.fm/`. Never in repo, logs, or fixtures.
- Classifier v1: local heuristic only. No remote LLM on prompts or Spotify-derived data until provider terms review in Milestone 1.
- Spotify: Premium + dev app available. Scopes to request: `user-top-read user-read-recently-played user-read-playback-state user-modify-playback-state user-library-read playlist-read-private`. Verify dev-mode allowlist limits before pilot.
- TUI: Bubble Tea + Lip Gloss, `cmd/tui/`. Setup + status only. Worker stays headless and scriptable.
- Code review: `open-code-review` (`ocr`) for all implementation PRs, see §7.

## 1. Architecture

```
Submitted Codex prompts → hook (UserPromptSubmit) → file queue
  → worker → rolling context → policy ← candidate pool (Spotify) + memory (SQLite)
  → playback controller → device → observer → memory
  TUI/CLI reads worker state + SQLite for status/explain
```

- Hook (`cmd/hook/`): parse stdin JSON (`session_id`, `cwd`, `prompt`, `turn_id`, `permission_mode`), append bounded event `{event_id, session_id, ts, prompt_hash?, prompt_len, cwd}` with full prompt in a separate short-TTL spool, exit 0 immediately. `async:true`, timeout ≤5s. Must not block Codex; worker crash ≠ turn stall.
- Worker (`cmd/worker/`): polls queue, dedups by `event_id`, updates `internal/context`, ranks via `internal/policy`, controls via `internal/spotify`, observes via `internal/observe`, learns via `internal/memory`.
- Single-owner lock for multi-session: one opted-in `session_id` controls playback; others are ignored with a logged reason.
- Raw prompts: local spool only, short TTL (e.g. 24h / 200 events cap). Derived context + track outcomes only in SQLite.

Proposed layout (not created yet):

```
cmd/hook/ cmd/worker/ cmd/tui/
internal/context/ internal/spotify/ internal/policy/
internal/memory/ internal/observe/ internal/queue/
fixtures/prompts/ fixtures/spotify/ fixtures/playback/
.codex/hooks.json (repo-local example)
```

## 2. Data shapes (v1 sketches)

- `context_snapshot { window_start, window_end, label: focus|explore|debug|refine|stuck, confidence 0-1, evidence_count, prompt_count }` — rolling last 5–10 prompts, 15-min decay. One message cannot flip label; require sustained evidence.
- `candidate { spotify_track_id, artist, title, familiarity_score, last_played_ts, source: top|recent|library|playlist, selection_count_7d }`
- `play_record { id, track_id, context_label, confidence, selected_by: companion|manual|independent, outcome: completed|replay|save|quick_skip|manual_override, ts }` — companion rows only drive learning.
- Signal weights: completed = weak+, replay/save = stronger+ (if provider exposes reliably), quick-skip = ambiguous−, repeated same-context skips outweigh one. Manual pause/selection/device change = immediate yield, not a preference signal by itself. Cap per-session influence; recency > age. All associations inspectable + resettable from TUI.

## 3. Spotify integration (Milestone 1 must verify, not assume)

Verify live and record: `GET /me`, `/me/top/{tracks,artists}`, `/me/player/recently-played`, library/playlist endpoints used, `/me/player/devices`, `PUT /me/player/play` queue + boundary timing on explicit device. Record manual-takeover limits, 401/429/device-gone/offline behavior.

Do not assume per `README.md:76`: audio-features, audio-analysis, recommendations. Track-label source is explicit discovery work.

Playback gating: `CONTEXTFM_ENABLE_PLAYBACK=1` + explicit TUI pilot toggle. Default is observe-only (rank + explain, no `play` calls). On uncertainty/error: preserve current playback.

## 4. TUI scope (setup + debug, never required for loop)

- First run: OAuth device/loopback flow → account confirm → device picker → enable pilot.
- Status: worker up, context label + confidence, current/next track + preserve/switch reason, last outcome.
- Controls: enable/pause, resume, reset memory, explain tail. Manual pause stays paused.
- CLI parity underneath: `contextfm status`, `contextfm explain` so CI/tests don’t need the TUI.

## 5. Milestones + acceptance

0. Scaffold: `go build ./...`, `go test ./...` pass under nix shell. Branch `chore/go-scaffold`.
1. Hook isolation: synthetic `UserPromptSubmit` payloads, hook p99 <200ms, worker killed mid-run → Codex unaffected. Branch `feat/prompt-capture`.
2. Spotify read: auth round-trip, device list, top/recent fetch from real account, dev-mode limits noted. No fixtures with real data. Branch `feat/spotify-read`.
3. Observe-only rank + explain: `fixtures/` synthetic prompts show familiar picks, stability across trivial edits, gradual shift on sustained change, isolated skip ≠ rank collapse. `contextfm explain` shows context + scores + evidence. Branch `feat/observe-only`.
4. Gated playback + observer/memory: one explicit-device change on test activation, boundary respect, manual yield, duplicate/multi-session/restart/401/429/device-gone/offline recovery, own-vs-independent attribution correct. Cross-session learning gradual, no mid-track abrupt shift. Branch `feat/pilot-playback`.

Each PR: what observed, what assumed, failure modes tested, `ocr` review clean.

## 6. Testing

- `go test ./...` with `fixtures/` only. No private transcripts, tokens, or listening history in repo.
- Synthetic replay harness: prompts → context labels → rankings → playback outcomes → memory deltas. Asserts: stability, gradual learning, caps.
- Failure injection: expired auth, missing device, rate limit, offline, duplicate events, restart mid-decision.

## 7. Code review with open-code-review

Tool: https://github.com/alibaba/open-code-review — CLI `ocr`, Go, deterministic pipeline + LLM agent, line-level comments.

Setup (one-time per dev machine):

```sh
npm install -g @alibaba-group/open-code-review
ocr config provider   # pick provider, enter key, test connectivity
ocr config model      # pick model for reviews
```

Per PR (required before merge):

```sh
cd ~/projects/work/context.fm
ocr review --from main --to <feat-branch> --format json --output /tmp/opencode/ocr-<branch>.json
# address findings or record as assumed-risk in PR body
```

Rules to add at scaffold time (`.opencodereview/`): Go vet-level checks (error handling, context timeouts), no secrets/tokens/fixtures-with-PII, hook fast-path (no network in `cmd/hook`), playback gating (no `play` outside gated controller + tests), SQLite migrations bounded.

CI (later, after scaffold): GitHub Action running `ocr review` on PR diffs + `go vet`/`go test`. Not in v1 scope.

Docs: https://open-codereview.ai/docs (CLI ref, review rules, delegation mode if we want the coding agent to self-review without extra LLM key).

## 8. Security / data boundaries

- Opted-in sessions only, submitted prompts only. No keylogging.
- Tokens in keyring, config `0600`, SQLite `0600`. No tokens/prompts/history in git, issues, screenshots, or `ocr` output pasted verbatim.
- License/distribution still open per `docs/PLAN.md:112` — public repo ≠ licensed. Decide before packaging.

## 9. Sign-off checklist

- [ ] Layout §1 + data shapes §2 agreed
- [ ] Spotify scopes + dev-mode check plan agreed
- [ ] TUI scope §4 agreed (setup/status only)
- [ ] `ocr` flow §7 agreed (provider/model per dev, JSON artifact per PR)
- [ ] Approve → create `chore/go-scaffold` branch and implement Milestone 0 only
