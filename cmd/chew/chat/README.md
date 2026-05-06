# `cmd/chew/chat`

The chat shell, mascot, and supporting packages for CHEWAgent. This is
the package tour — for install + usage docs see the [top-level
README](../../../README.md).

## What runs

`chat/repl/` is the binary entrypoint. `go run ./cmd/chew/chat/repl`
(or `chew` after `./install.sh`) starts the loop:

1. Start the fixed-header terminal UI when stdin/stdout are interactive;
   use plain text mode for pipes or `CHEW_PLAIN=1`.
2. Check brain state (installed? napping? not installed?).
3. Resume the last project from `<repo>/brain/last-project.txt` if any.
4. Prompt. On each input:
   - `planner.Plan(input)` — match against the regex vocabulary.
   - Print the response, dispatch any verbs through `tool.Registry`.
   - Handle system actions (install brain / wake up / nap / open project / etc.).
   - Update mascot state when needed.
5. On exit (quit, Ctrl+C, terminal close): stop the brain cleanly.

## Packages

```
encode/    NES CHR-ROM encoder. Aseprite JSON + PNG → bit-plane Go data.
           Used at build time only (the encoded data is committed in
           sprite/chr_data.go and sprite/gum_chr.go).

sprite/    NES PPU bit-plane decode + terminal rendering. Decodes the
           encoded CHR data back to per-pixel grids and renders frames
           via full-cell ANSI background colors (no glyphs, no half-
           blocks — the half-block approach showed line-gap stripes
           through colors so we switched to 2-spaces-per-pixel).

planner/   The "brainless" layer. Regex vocabulary table that converts
           user input into a Plan{Response, Verbs, LaunchWizard, ...}.
           voice.go holds every CHEW string — round-robin pools so each
           command response varies. Edit voice.go to rewrite anything
           CHEW says.

tool/      Verb implementations. Tool interface + Registry. Standard
           tools registered by NewDefault(): web_search, web_fetch,
           read_file, list_dir, search, write_file, run_command. The
           planner emits verb names; the registry runs them.

wizard/    Multi-step interactive flows. Currently: install_brain
           (download Bonsai, spawn llama-server, write config). brain.go
           is the subprocess/endpoint manager (Start/Attach/WaitHealthy/
           Stop). profile.go owns the hidden model profile contract.
           Includes orphan cleanup via brain.pid + KillStaleBrain.
           voice.go in this package holds the wizard-specific text.

project/   "A project is a folder." Open(path), Create(path),
           starter GUM.md template, last-project memory, and the
           `.gum/status.json` / `.gum/status.jsonl` checkpoint trail.
           Silent git init on Create (save points without saying "git").

gum/       The Truth Steward — deterministic stage detection. Detect()
           observes a project (GUM.md content, source files, commits)
           and returns one of: NoProject / EmptyProject / IntentKnown
           / Started / Mature. Each stage has a paragraph of
           instructions in instructions.go that gets folded into the
           brain's system prompt, telling it what to do at this stage.

repl/      The chat shell binary. Wires planner + tools + wizards +
           project + gum together. brain_chat.go: HTTP client for the
           OpenAI-compat endpoint llama-server exposes; system prompt
           built from gum.Detect/Instructions + project's GUM.md.
           main.go: REPL loop, mascot rendering, signal handling.
           tui.go keeps brain-backed free-form asks asynchronous so the
           header keeps animating and status questions can be answered
           from Gum facts while the model is busy.

testbed/   Sprite playground. `go run ./cmd/chew/chat/testbed`. Type
           0..7 to step through CHEW frames, gum 0..5 for GUM, all to
           see them all.

assets/    Source art. CHEW_NES.png + CHEW_NES.json (Aseprite export),
           same for nesGUM. CC-BY-NC.
```

## Mascot

`assets/CHEW_NES.png` — 8 frames, 16×16 each, NES palette. Bound to
**actual system state**, not faked:

| frames | state | when CHEW shows it |
|---|---|---|
| 0–2 | idle | waiting for input, no verb in flight |
| 3–5 | walk | deterministic verb/tool work |
| 6–7 | ghost | brainless, unreachable, error, or a long brain call in flight |

`assets/nesGUM.png` is also encoded in Go. The chat still talks as CHEW, but
the TUI can briefly swap the header sprite to Gum for deterministic records
and tool work: project setup, GUM.md updates, file verbs, shell verbs, and web
fetch/search.

`mascotState.set(state)` updates the current animation state. Interactive
sessions render CHEW in a fixed 16-row TUI header with a scrollable dialog
viewport below it and an input row at the bottom. The TUI owns the viewport
instead of using terminal scroll regions, so resizing the window shows more
or less dialog without covering text. Piped output and `CHEW_PLAIN=1`
stay plain text.

The full-cell renderer is two ANSI background-colored spaces per source
pixel, so a 16×16 sprite is 32×16 character cells — readable in any
modern terminal, no glyphs needed.

## Architecture: chassis vs steward vs brain

CHEW is intentionally three layers, each in its own role:

- **Chassis** (planner + tool registry + REPL): deterministic. Handles
  every command that has a clear shape — file ops, web verbs, git read-
  only, project setup, brain lifecycle.
- **GUM, the steward** (`gum/`): also deterministic. Watches the
  project's shape and decides which stage we're at. Hands the brain a
  stage-appropriate playbook on every chat session rebuild.
- **The brain** (Bonsai via llama-server, when awake): the
  conversational layer. Free-form questions, brainstorming, explaining
  code. The system prompt it gets is composed from CHEW's character +
  the project's GUM.md + the current GUM stage instructions, so it
  always has situation awareness without having to track state itself.

Small models (Bonsai is 8B-class compressed to 1.16 GB) are great at
language and bad at tracking state across turns. GUM does the tracking
deterministically and tells the brain what matters this turn.

The TUI treats work status as a Gum fact, not a second model call. When
CHEW sends a long free-form request to the brain, it writes a compact
checkpoint to `.gum/status.json` and appends an event to
`.gum/status.jsonl`. While the brain is occupied, CHEW renders in ghost
state and questions like "what are you doing?" or "where are we at?" get a
foreground answer from that checkpoint without interrupting the model.

Public CHEWAgent keeps Bonsai as the automatic default: normal users see
`install brain`, `wake up`, and `nap`, not a model chooser. The hidden
`wizard.ProfileConfig` contract (`<repo>/brain/config.json`,
`chew-profile-config.v0`) lets internal/private builds point CHEW at a
different llama-server or OpenAI-compatible endpoint without changing the
chat shell. Profile-switching verbs exist for development, but are not
advertised in the top-level README, first-run text, or help output. Once
the v0 profile schema is written, older pre-profile CHEWAgent builds will
not understand that config file.

## Where to edit

- **CHEW's voice (responses, fallbacks, project messages)** —
  [`planner/voice.go`](planner/voice.go). Round-robin pools, raw
  string literals, heavily commented. Edit freely.
- **Wizard text (install brain plan/details/done)** —
  [`wizard/voice.go`](wizard/voice.go). Same shape.
- **Brain system prompt + stage playbooks** —
  [`repl/brain_chat.go`](repl/brain_chat.go) (character + commands)
  and [`gum/instructions.go`](gum/instructions.go) (per-stage rules).
- **The character** — [`planner/voice.go`](planner/voice.go) opens with
  a "Who CHEW is" comment block that captures the cantankerous-frog
  tone in a few lines. Read that before rewriting voice strings so the
  rhythm stays consistent.

## Why CHEW looks like CHEW

He's authentically NES-palette. 4 colors, 16×16, no anti-aliasing, no
gradients. He blinks when waiting, walks when thinking, ghosts when the
line goes dead. The aesthetic isn't decoration — it's a tonal commitment
that this is software that respects its own constraints (small, local,
finite, knowable). Same ethos that puts the agent on a local model
instead of a cloud API.
