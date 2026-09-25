# Implementation plan

## Scope

Deliver a personal, local-first prototype that quietly adapts familiar music while the listener uses Codex. The core experience must work without music commands: ordinary submitted prompts provide session context, while playback outcomes gradually teach the system which familiar music fits comparable work. First prove prompt capture and playback behavior, then evaluate whether the selections feel right. This repository currently contains planning only; language, model, packaging, and hosting choices remain open.

## Components

```mermaid
flowchart LR
    C[Submitted Codex prompts] --> H[Prompt hook adapter]
    H --> W[Local background worker]
    W --> X[Rolling work context]
    S[Authorized Spotify listening data] --> T[Familiar candidate pool]
    X --> R[Selection policy]
    T --> R
    F[Context-to-music preference memory] --> R
    R --> P[Playback controller]
    P --> D[Selected Spotify device]
    D --> O[Playback observer]
    O --> F
```

The prompt adapter should enqueue bounded events and return promptly. Classification and network calls belong in the worker, independent of the coding turn. A music-service failure must not block a prompt.

Use event IDs for deduplication, a short context window, bounded storage, and a single playback owner. Keep raw prompts separate from derived context and preference memory so prompt text can expire without erasing the learned mapping. If multiple Codex tasks are active, start with one opted-in session rather than allowing competing controllers.

## Selection behavior

- Establish familiarity from actual listening evidence. Do not invent musical preferences.
- Rank for familiarity, work-context fit, and continuity; exact weights are experimental.
- Treat session context as a rolling estimate built from several submitted prompts. Do not require a music request or react strongly to one isolated message.
- Track-level mood or energy labels require a validated data source or explicit heuristic. Do not assume genre or artist identity reliably describes every song.
- Apply repetition limits and minimum time between direction changes. Keep the current direction when evidence is weak.
- Prefer natural track boundaries. Do not promise seamless crossfades, unrestricted queue editing, or precise timing until the client/API behavior is demonstrated.
- Treat skips as ambiguous feedback. Distinguish companion-selected plays from independent listening where possible so the system does not reinforce its own choices as new evidence of taste.
- Yield to manual pause, track selection, device changes, and playback controlled elsewhere. Define and test how manual intervention is detected.

## Feedback and adaptation

Preference memory should learn mappings between derived work context and music, not global rules about what a person likes. Store bounded records for companion-selected tracks with the derived context, selection confidence, and observable outcome. Do not retain raw prompt text in the long-term preference model.

Use conservative signal weights:

- A completed listen is a weak positive signal.
- A replay or save is a stronger positive signal when the provider exposes it reliably.
- A quick skip is negative but ambiguous; repeated skips in comparable contexts carry more weight than one skip.
- Manual playback, pause, album selection, playlist selection, or device changes override automation immediately. Manual music may become evidence only when ownership and surrounding context are clear.
- Volume changes, backgrounding, connectivity loss, and unavailable devices are not preference signals by themselves.

Recent repeated evidence should outweigh old evidence, and the model should cap the influence of any single session. Every learned association must be inspectable, resettable, and attributable to non-sensitive derived context plus playback observations. The selection policy must keep exploration limited so a weak experiment cannot take over the session.

## Milestones

### 1. Integration feasibility

- Verify `UserPromptSubmit` from the target Codex desktop build, including ordinary typed and submitted voice input if supported.
- Demonstrate that capture returns promptly and worker failures cannot stall Codex.
- Authenticate a Spotify development app and read back its authorized account and selected playback device.
- Verify access to top tracks, recent plays, and any proposed library/playlist endpoints.
- Demonstrate controlled playback with explicit test activation; record queue, boundary timing, and manual-control limitations.
- Review current provider terms for the proposed personalization and model usage before processing Spotify-derived data with a model.

**Exit evidence:** a concise record of observed behavior, API access, and unresolved limitations. If hooks are unavailable, investigate supported app-server events; transcript-file watching is only a fallback with format/version risks documented.

### 2. Observe-only prototype

- Generate candidate selections from real authorized preference data without changing playback.
- Use synthetic prompts for development; do not commit private conversations or listening histories.
- Compare sustained context changes with trivial edits and ambiguous prompts.
- Inspect proposed selections for familiarity, repetition, continuity, and unsupported assumptions.
- Replay synthetic playback outcomes to verify that repeated context-linked signals change ranking gradually while isolated skips do not.
- Show a local explanation containing the derived work context, relevant preference evidence, and why the current track would be preserved or the next candidate selected.

**Exit evidence:** reviewed examples show familiar selections, stable behavior across minor prompt changes, and gradual ranking changes from repeated synthetic outcomes. Mood-label quality and candidate coverage are documented rather than assumed.

### 3. Automatic personal pilot

- Enable autonomous selection after one-time setup and explicit pilot activation.
- Make gradual changes around track boundaries without per-track prompts or chat announcements.
- Verify pause/manual-selection precedence, duplicate events, multiple sessions, restarts, expired authorization, rate limits, unavailable devices, and offline recovery.
- Avoid replaying stale decisions when reconnecting.
- Record which tracks were selected by Context.fm and verify that independent listening is not fed back as if it were a companion decision.
- Demonstrate that repeated listens, replays, and skips in comparable contexts influence later sessions without causing abrupt changes during the current track.

**Exit evidence:** observed listening sessions demonstrate automatic adaptation without music commands, preserved manual control, context-linked learning across sessions, and safe recovery. The listener judges musical fit; successful API calls alone do not establish quality.

### 4. Packaging and expansion

Select the runtime and operating-system integration after the pilot. For NixOS, use declarative configuration for persistent services. Investigate Apple Music, broader distribution, provider access requirements, and licensing separately.

## Data boundaries

- Observe submitted prompts only, from opted-in sessions. No keylogging or broad desktop monitoring.
- Keep raw prompt content local by default and short-lived. Any remote classifier requires an explicit data-flow decision during setup.
- Send only playback identifiers and necessary control data to the music provider.
- Keep OAuth credentials in an appropriate local credential store; never in source control or debug output.
- Separate integration data from public sample fixtures. Avoid logging raw prompts, listening history, and tokens.
- Include a simple off switch. On uncertainty or errors, preserve existing playback.

## Decisions still needed

- Runtime and service packaging.
- Supported Codex desktop versions and hook behavior.
- Local classifier versus opt-in remote classification.
- Source and quality of musical attributes used for contextual matching.
- Derived work-context representation and the minimum evidence required before learning an association.
- Playback observations each provider exposes reliably enough to use as feedback.
- Familiar-only default versus an optional amount of discovery.
- Track-boundary scheduling and intervention detection supported by Spotify.
- Multi-session ownership and automatic resumption after a manual override.
- Distribution and license choice. Public visibility alone is not an open-source license.

## First collaboration pass

Review the experience in the README, challenge assumptions in milestone 1, and agree on a narrow integration spike. Record observations and decisions through issues or pull requests before committing to the implementation stack.
