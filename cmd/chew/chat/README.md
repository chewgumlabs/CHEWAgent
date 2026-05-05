# `chew chat` — the conversational shell for Chew

Date: 2026-05-04
Status: design sketch + mascot in place; implementation deferred

## What this is

A casual chat CLI that lets you talk to Chew the way you'd talk to a colleague:

```
$ chew chat
chew> here's https://github.com/foo/bar — can we pull this down and study it?
[chew picks: specimen-seed → fingerprint → engineering-orient → harvester]
chew> Started. Decomposition first (~30 min on JH), harvester after.
> what phase are we at?
[reads state-ledger + recent verb summaries]
chew> Engineering atlas, file 12 of 188. ~18 min left for this sub-phase.
> show me candidate behaviors so far
[reads harvester-round-up output if present]
chew> Round-up hasn't started yet — kicks off after decomposition. Want me to ping when it lands?
> yeah
chew> 👻
```

The pitch: **the chat CLI is a wrapper over the verb registry**. The model planning each turn knows about every Chew verb (the static header carries them) and decides how to translate "here's a repo, can we study it?" into a chain of verb calls. The verbs ARE the skills; the chat is just the casual entry point.

## CHEW the mascot

[`assets/CHEW_NES.png`](assets/CHEW_NES.png) — 8 frames, 16×16 each, authentic NES palette.

Frame mapping (per `assets/CHEW_NES.json`, column-major in the sheet):

| Frames | Animation | Mascot state | System state |
|---|---|---|---|
| 0, 1, 2 | Idle (3-frame cycle) | At rest, blinking | Waiting for user input; no verbs in flight |
| 3, 4, 5 | Walk (3-frame cycle) | Working, moving | A verb is executing; deep-read in progress; harvester pass running |
| 6, 7 | Ghost (2-frame cycle) | Spectral / dead | Transport error; model unreachable; verb returned with `model_error`; ChewJackHammer down |

The mascot is in the corner of the chat UI. Its animation is **bound to live system state**, not faked:
- Idle when the most recent verb summary is older than N seconds and no verb is running.
- Walk when an active verb `request.json` exists with no `summary.json` written yet (verb in flight).
- Ghost when the most recent verb's `summary.json` has `ok: false` with transport-class error_type, OR a liveness probe of the bound model fails.

This makes the mascot a **live status indicator** — not decoration. If you see ghost-mode CHEW, the model is genuinely unreachable.

## Architecture: stateless reader of disk + verb dispatcher

The chat shell is intentionally **a thin process that holds no state of its own**. All ground truth lives on disk in the existing telemetry artifacts (`request.json` / `summary.json` per verb call, `state-ledger.json` per specimen, harvester outputs in `evals/`).

```
                         ┌──────────────────────┐
                         │   chew chat (TUI)    │
                         │  ┌──────────┐        │
                         │  │ 🟡 CHEW  │  idle  │   ← polls disk every 500ms
                         │  └──────────┘        │   ← reads recent summary.json
                         │                      │       to choose mascot state
                         │  > what phase ...?   │
                         │                      │
                         │  [chat history]      │
                         └──────┬───────────────┘
                                │
            ┌───────────────────┼───────────────────┐
            ▼                   ▼                   ▼
    ChewJackHammer        Verb registry        Disk (artifacts)
    (planner LLM)         (existing 31 verbs)  truth/runs/...
                          + new status verbs   evals/...
                                               workspace/research/...
```

The chat REPL never owns work in progress. Long-running tasks (deep-read, full harvester) get **launched as detached processes** and write their telemetry to disk. The chat reads that disk to answer questions and update the mascot. This means:

- You can spin up `chew chat`, kill it, restart it — the harvester running underneath doesn't notice.
- Two chat sessions can run side-by-side, both seeing the same true state.
- The chat doesn't crash when the model is busy — it just shows ghost-CHEW.

## Three new verbs needed

To wire the chat to disk-state, add these verbs to the existing registry:

1. **`read_session_status`** — given a session name OR auto-discover the most recent run, return: current phase, last successful verb, count of verbs by status, ETA estimate, any transport errors. Pure read of `truth/runs/<run>/orchestration/...` artifacts.

2. **`read_specimen_progress`** — given a specimen ID, walk `workspace/research/_<Specimen>/` to report which orientation/decomposition/harvest stages are complete. Reads `state-ledger.json` and the `*_orientation`, `*_surfaces`, `*_harvest` directories.

3. **`liveness_probe`** — given a model alias, hit `/v1/models` on its endpoint and return up/down + latency. Drives ghost-mode detection.

Each is a deterministic disk/HTTP read with no LLM call, so the mascot polling cost stays trivial.

## TUI layout sketch

```
╔════════════════════════════════════════════════════════════════════╗
║                                                       ┌──────┐     ║
║  CHEW @ chewgumlabs                                   │ 🟡   │     ║
║                                                       │ idle │     ║
║                                                       └──────┘     ║
║  ──────────────────────────────────────────────────────────────    ║
║                                                                    ║
║  > here's https://github.com/foo/bar — can we study it?            ║
║                                                                    ║
║  Started. Engineering decomposition first (~30 min on              ║
║  ChewJackHammer), harvester runs after.                            ║
║                                                                    ║
║  [verb chain: specimen-seed → fingerprint → engineering-orient]   ║
║                                                                    ║
║  > what phase are we at?                                           ║
║                                                                    ║
║  Engineering atlas pass, file 12 of 188 (~18 min remaining)        ║
║                                                                    ║
║  ──────────────────────────────────────────────────────────────    ║
║  > _                                                               ║
╚════════════════════════════════════════════════════════════════════╝
```

The mascot animation runs in its own goroutine; chat input/output runs in the main loop. Mascot polls disk on a 500ms tick — cheap.

## Implementation surface (when we build)

- **Go subcommand**: extend `cmd/chew` with a `chat` subcommand. Existing CLI dispatch lives in `cmd/chew/main.go`.
- **TUI library**: `bubbletea` (charmbracelet) — actively maintained, mature, supports the Elm-style state model that maps cleanly to "render mascot from disk-state every tick."
- **Sprite rendering in terminal**: three options to evaluate at build time:
  1. **Unicode half-blocks** (▀▄): renders 16×16 sprite as 16×8 character cells. Works in any terminal. Lossy on color count but NES palette is small.
  2. **Sixel** (broad terminal support, Apple Terminal etc.): per-pixel color, true 16×16 rendering.
  3. **Kitty graphics protocol** (kitty + a few others): best fidelity, narrowest support.
  - Pick by what most users actually run. Half-blocks is the safest default.
- **Aseprite JSON parser**: trivial — the `assets/CHEW_NES.json` already has frame coordinates, animation state mapping is hand-defined here.
- **Planner**: `ChewJackHammer` running with the existing static header + a tiny "casual conversation" addendum. Same model that runs the verbs is the model that picks them — no separate brain.

## What this DOESN'T do

- **No mid-thought interruption**: you can't pause an in-flight verb to redirect it. Verbs run to completion (or transport-error). The chat lets you queue, observe, and ask, but not preempt. This is by design — interrupting a partially-emitted blueprint is more dangerous than just letting it land and starting fresh.
- **No persistent agent process holding RAM state**: everything is on disk. The chat shell can die at any time without losing work.
- **No web UI for v0**: terminal first. The mascot already maps to a web sprite-sheet shape (`assets/CHEW_NES.png` is exactly the layout a web canvas animation would consume), so a future `chew web` would render the same asset.

## Status

- ✅ Design + mascot assets landed (this file + `assets/`)
- ⏳ Implementation deferred until: (a) the harvester pipeline validates end-to-end on a specimen (in-flight today), (b) we have a few more verbs on the registry that benefit from casual invocation
- ⏳ The 3 status verbs (`read_session_status`, `read_specimen_progress`, `liveness_probe`) — easy adds; can ship before the chat REPL itself

## Why CHEW looks like CHEW

He's authentically NES-palette. 4 colors, 16×16, no anti-aliasing, no gradients. He blinks when waiting, walks when thinking, ghosts when the line goes dead. The aesthetic isn't decoration — it's a tonal commitment that this is software that respects its own constraints (small, local, finite, knowable). Same ethos that puts the agent on local models instead of remote APIs.
