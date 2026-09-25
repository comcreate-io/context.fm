# context.fm

**Your session sets the soundtrack.**

Context.fm is a background music companion that quietly scores your work in Codex using music you already know and enjoy. It does not wait for a music command. The session itself is the input.

**Status: concept and implementation plan. No working application yet.** Spotify is the proposed first integration; Apple Music is a later investigation.

## The experience

Connect your music account and enable the companion once. Then continue using Codex normally: describe an idea, wrestle with a failing test, refine a design, or celebrate a fix. Context.fm uses a short rolling window of submitted prompts to estimate the shape and intensity of the work, then gradually adjusts upcoming music within your taste.

You never need to say “play music,” choose a mood, or manage a set of preset playlists. Context.fm stays in the background and lets the soundtrack emerge from the session. Ordinary operation has no music commands, mood questionnaires, chat announcements, or per-track confirmation prompts.

Ambient describes the behavior of the companion, not a required music genre. The soundtrack should sound like you.

For example:

- Open-ended brainstorming may lean toward familiar music that has accompanied energetic, exploratory sessions before.
- Sustained debugging may gradually favor less distracting tracks without reacting to every failed command.
- A resolved problem may lift the direction of the next track without interrupting the current one.
- Terse or frustrated-looking prompts are contextual clues, not proof of an emotion. Context.fm adapts conservatively and never claims to know how the listener feels.

## What it learns

The initial taste model comes from authorized listening evidence such as recent plays, top tracks and artists, and accessible library or playlist data. Over time, Context.fm can learn which parts of that taste fit different kinds of sessions.

Learning should be quiet and contextual:

- Listening through a companion-selected track is a weak positive signal.
- Replaying or saving it is a stronger positive signal.
- Skipping quickly is a negative-but-ambiguous signal, not a definitive dislike.
- Manually choosing different music tells Context.fm to yield immediately and may inform later sessions only when the surrounding context is comparable.
- Repeated outcomes matter more than a single action. Recent evidence should outweigh stale evidence.

The result is personal rather than universal: “intense debugging music” should mean whatever has actually worked for this listener. Context.fm must distinguish its own selections from independent listening so it does not train on its output as if it were new evidence of taste.

## Product principles

- Familiarity first: prioritize tracks you have played and artists you already enjoy, with recent listening weighted strongly.
- The session is the control surface: normal Codex prompts provide context; music commands are not required for the core experience.
- Context is a clue: use several submitted prompts to estimate the work context. Do not claim to know the listener's emotions.
- Continuity matters: let tracks finish and adjust upcoming selections gradually. A single message should not abruptly change the soundtrack.
- Learn conservatively: adapt from repeated, context-linked playback outcomes rather than treating every skip or listen as a verdict.
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
6. Observe bounded playback outcomes and update a local context-to-music preference memory without over-interpreting individual actions.
7. Provide a simple enable/pause control and an optional local explanation view for debugging.

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
