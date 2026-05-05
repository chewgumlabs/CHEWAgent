# `cmd/chew/chat`

The chat shell, mascot, and supporting packages for CHEWAgent. This is
the package tour — for install + usage docs see the [top-level
README](../../../README.md).

## What runs

`chat/repl/` is the binary entrypoint. `go run ./cmd/chew/chat/repl`
(or `chew` after `./install.sh`) starts the loop:

1. Render the CHEW sprite (you see his face immediately).
2. Check brain state (installed? napping? not installed?).
3. Resume the last project from `<repo>/brain/last-project.txt` if any.
4. Prompt. On each input:
   - `planner.Plan(input)` — match against the regex vocabulary.
   - Print the response, dispatch any verbs through `tool.Registry`.
   - Handle system actions (install brain / wake up / nap / open project / etc.).
   - Re-render the mascot.
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
           is the subprocess manager (Start/WaitHealthy/Stop). Includes
           orphan cleanup via brain.pid + KillStaleBrain. voice.go in
           this package holds the wizard-specific text.

project/   "A project is a folder." Open(path), Create(path),
           starter GUM.md template, last-project memory. Silent
           git init on Create (save points without saying "git").

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
| 3–5 | walk | verb running / brain thinking |
| 6–7 | ghost | error / brain unreachable / no brain installed |

`mascotState.set(state)` updates the current animation; `renderMascot()`
draws one frame inline. The full-cell renderer is two ANSI background-
colored spaces per source pixel, so a 16×16 sprite is 32×16 character
cells — readable in any modern terminal, no glyphs needed.

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
