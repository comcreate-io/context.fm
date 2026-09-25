# Vibe Companion

A background music companion that follows your work in Codex, drawing from music you already know and enjoy.

**Status: concept and implementation plan. No working application yet.** Spotify is the proposed first integration; Apple Music is a later investigation. The name is provisional.

## The experience

Connect your music account and enable the companion once. Continue using Codex normally. As your conversation moves between exploration, creative work, and focused problem solving, upcoming music gradually shifts with it. No music commands, mood questionnaires, or confirmation prompts during normal listening.

Ambient describes the behavior of the companion, not a required music genre. The soundtrack should sound like you.

## Product principles

- Familiarity first: prioritize tracks you have played and artists you already enjoy, with recent listening weighted strongly.
- Context is a clue: use several submitted prompts to estimate the work context. Do not claim to know the listener's emotions.
- Continuity matters: let tracks finish and adjust upcoming selections gradually. A single message should not abruptly change the soundtrack.
- Stay quiet: ordinary operation needs no notifications or approval clicks after initial setup.
- Respect direct control: a manual pause stays paused; choosing an album or playlist makes the companion yield.
- Preserve momentum: uncertain context or an unavailable service should leave current playback alone.

Example: familiar upbeat songs accompany brainstorming. As the conversation settles into sustained debugging, upcoming selections gradually become less busy while remaining within the listener's taste.

## Proposed first version

1. Capture submitted user prompts from an opted-in local Codex session.
2. Build a candidate pool from authorized Spotify listening data and accessible library/playlists, verifying each data source against the app's API access.
3. Maintain a small, short-lived estimate of the current work context.
4. Rank familiar candidates for contextual fit and continuity, with repetition limits and conservative switching.
5. Schedule playback changes around track boundaries on one explicitly selected device.
6. Provide a simple enable/pause control and an optional local explanation view for debugging.

There is no requirement to pick five preset playlists or tell the companion when to shift. Manual music commands through MCP may be useful later, but the background experience must work without them.

## Architecture and next steps

See [the implementation plan](docs/PLAN.md) for components, experiments, open decisions, and acceptance criteria. See [contributing](CONTRIBUTING.md) for collaboration conventions.

## API evidence and limits

Documentation reviewed September 25, 2026; actual account and desktop behavior still require a prototype.

- [Codex hooks](https://developers.openai.com/codex/hooks) document `UserPromptSubmit` with the submitted prompt. Verify support and timing in the target desktop build before selecting this integration.
- Spotify exposes authorized [top tracks/artists](https://developer.spotify.com/documentation/web-api/reference/get-users-top-artists-and-tracks) and [recent listening](https://developer.spotify.com/documentation/web-api/reference/get-recently-played). These are preference signals, not access to Spotify's internal recommendation model or complete listening history.
- Spotify supports [playback control](https://developer.spotify.com/documentation/web-api/reference/start-a-users-playback) for Premium users. Its [development-mode limits](https://developer.spotify.com/documentation/web-api/tutorials/february-2026-migration-guide) must be checked before expanding beyond a personal prototype.
- [Apple MusicKit](https://developer.apple.com/musickit/) supports music access and playback integrations. Equivalent control of an existing player has not been validated for this project.

Do not assume access to Spotify audio features, audio analysis, or recommendation endpoints. Music classification and provider policy compatibility are explicit discovery work, not solved capabilities.
