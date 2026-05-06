# CHEWAgent

A frog in the machine. A cantankerous coding agent.

CHEW is a plug-and-play local AI agent: a chat shell with a frog mascot,
a deterministic vocabulary for common file/shell/web tasks, and a small
local LLM (**Bonsai**, ~1.16 GB) you can install with one command. No
accounts, no telemetry, no cloud — your data stays on your machine.

> *Hmph. I'm CHEW. I'm a frog, somehow trapped in this machine. I don't
> know why. I don't seem to mind. I'll help you, even if I think you're
> weird.*

---

## Install

```sh
git clone https://github.com/chewgumlabs/CHEWAgent.git
cd CHEWAgent
./install.sh
```

`install.sh` downloads the right binary for your platform from the
[latest GitHub Release](https://github.com/chewgumlabs/CHEWAgent/releases/latest)
and drops it as `~/.local/bin/chew` (or wherever's writable on your PATH —
no sudo). Open any terminal, type `chew`, you're in.

When working from this checkout, use `./install.sh --local` to rebuild
and reinstall the `chew` command from the current source tree.

**Supported platforms:** macOS (Apple Silicon + Intel), Linux (x64 + ARM64),
Windows (x64). No Go required at install time. `install.sh` falls back to
local `go build` only if your platform isn't pre-built.

---

## First time

```
$ chew
[CHEW opens a terminal chat with CHEW fixed at the top]

No brain installed yet. Type 'install brain' to set one up.
┌──────────────────────────────────────────────────────┐
│ CHEW chat                                            │
│ Commands: read, ls, write, find, run, git, web,      │
│ fetch, preview, remember, install brain, wake up,    │
│ nap, help, quit.                                     │
└──────────────────────────────────────────────────────┘

> install brain
[wizard walks you through downloading Bonsai — ~1 minute, ~1.16 GB]

> wake up
*croak* Brain online.

> what's a good way to start a Python tracker app?
[brain answers in CHEW's voice, with project-aware advice]
```

---

## What CHEW can do

### Files & shell

| command | what it does |
|---|---|
| `read <file>` | print a file's contents |
| `ls [<dir>]` | list a directory |
| `write <file>` | create a new file (refuses to overwrite) |
| `find <pattern>` | regex-search the tree (skips `.git`, `node_modules`, binary files) |
| `run <command>` | shell command, 30s timeout, 8 KB output cap |
| `preview` | start or reuse a local static website preview |
| `preview open` | start or reuse the preview and open it in a browser |
| `preview status \| stop` | inspect or stop the preview server |
| `pwd` | current directory |

### Web (no key, no account, no JS)

| command | what it does |
|---|---|
| `web search <query>` (or `google <query>`) | DuckDuckGo HTML, top 5 hits with URLs and snippets |
| `fetch <url>` | HTTPS GET, strip `<script>`/`<style>`, capped at 1 MB |

### Git (read-only by default)

| command | what it does |
|---|---|
| `git status \| diff \| log \| branch \| show \| blame` | runs the read-only verb |
| `git push \| commit \| merge \| reset \| ...` | refused without explicit consent |
| `force: <command>` | escape hatch to run a mutating command you typed yourself |

### Project

CHEW works out of a project folder you give him. No git terminology
required.

| command | what it does |
|---|---|
| `here <path>` | set the active project folder |
| (drag a folder onto the chat) | same as `here <path>` |
| `make project <name>` | create a project folder under `~/Documents/`, set up memory/git, and move in |
| `make folder <name>` | create a plain folder under `~/Documents/` without changing projects |
| `remember <note>` | record a note in project memory |
| `forget project` | clear the active folder |

Each project carries CHEW's project memory. He reads it on arrival to
catch up on what's true, and updates it as significant decisions land.
You talk to CHEW; he keeps the notes.

CHEW also keeps a tiny Gum status trail while he works. The current
checkpoint lives at `.gum/status.json`; the append-only event trail lives
at `.gum/status.jsonl`. In the TUI, questions like "what are you doing?"
or "where are we at?" are answered from those Gum facts, so CHEW can give
a live human-language update without interrupting a brain call.

Advanced/private builds can load a **Gum Key** with `CHEW_GUM_KEY=/path/to/key.json`.
A Gum Key is a small `chew-gum-key.v0` profile that adds pack-specific
instructions and, optionally, a foreground status provider. The public command
loads built-in Public Gum automatically when no key is set. This repo also ships
a concrete public profile at [`gum-keys/public.gum-key.json`](gum-keys/public.gum-key.json)
for posts, demos, debugging, and anyone who wants to point at the explicit file:

```sh
CHEW_GUM_KEY="$PWD/gum-keys/public.gum-key.json" chew
```

Public Gum keeps CHEW conversational while making status reports traceable with
simple labels like `Checkpoint`, `Next`, and `Blocked`. Private/internal
launchers can use the same Gum Key shape to unlock richer workflow knowledge
without forking the CHEW shell.

Entrypoints stay split by responsibility: Gum Keys carry workflow guidance;
runtime profiles carry model/state/endpoint details. See [`docs/entrypoints.md`](docs/entrypoints.md)
for the public/internal contract.

Preview runtime files live under **`.chew/runtime/`** and are ignored by
`.chew/.gitignore`. That keeps local server PIDs and logs out of portable
project memory.

`preview` runs `make build` when the project has a `build` target, then
serves `site/`, `dist/`, `public/`, or a root `index.html` on localhost.

### Brain (the LLM)

| command | what it does |
|---|---|
| `install brain` | one-time download of Bonsai (~1.16 GB) into `<repo>/brain/` |
| `wake up` | load Bonsai into memory (~3 seconds, ~1.5 GB RAM) |
| `nap` | stop Bonsai, free memory |

The brain dies cleanly with the REPL — Ctrl+C, terminal close, normal
quit, all handled. A hard kill (e.g. `kill -9`) leaves a metadata file;
the next CHEW launch reads it, skips any brain owned by another active
CHEW session, and stops only genuine orphans.

---

## How CHEW thinks (and where you can edit him)

Two layers, working in parallel:

1. **The chat shell** runs CHEW's voice and dispatches commands. The
   regex vocabulary + frog phrases live in [`cmd/chew/chat/planner/`](cmd/chew/chat/planner/).
   Edit `planner/voice.go` to rewrite anything CHEW says.
2. **GUM, the steward** ([`cmd/chew/chat/gum/`](cmd/chew/chat/gum/)) is
   a deterministic layer that observes the project's shape — what's in
   `GUM.md`, `.gum/status.json`, what files exist, what's been committed —
   and hands the brain a stage-appropriate playbook on every turn. Optional
   Gum Keys can add pack-specific instructions and a status/orientation
   provider without changing the TUI. Edit `gum/instructions.go` to rewrite how
   CHEW behaves at each stage of project life (no project / empty / intent known
   / started / mature).

The brain (Bonsai) is the conversational layer. GUM is the situation
awareness. They're separate on purpose: small models are great at
language, bad at tracking state, so GUM does the tracking deterministically
and tells the brain what to focus on each turn. CHEW stays the one you talk
to; Gum can briefly pop into the header during records/tool work. When the
brain is busy, the TUI keeps animating and status questions are answered
from the last emitted Gum checkpoint instead of sending a second prompt to
the model.

Naming-wise, this repo is the public **`chew.agent`** chassis: the local
CLI, deterministic verbs, GUM project memory, mascot shell, and small-model
hooks. Public, reusable machinery belongs here. Private orchestration can
layer on top as **`chew.internal`**, sharing the same core behavior without
shipping private workspace assumptions.

---

## What's in the repo

```
.github/workflows/release.yml   tag v* → build all platforms → release
build-binaries.sh               cross-compile chew for 5 platforms
install.sh                      install the chew command system-wide
docs/entrypoints.md             public/internal launcher contract
cmd/chew/chat/
  encode/                         NES CHR-ROM encoder (asset pipeline)
  planner/                        regex vocabulary + frog voice pools
  repl/                           the chat shell (binary entrypoint)
  sprite/                         NES PPU bit-plane decode + render
  testbed/                        sprite playground (`go run ./testbed`)
  wizard/                         install-brain wizard, brain process mgmt
  project/                        folder + GUM.md as project memory
  gum/                            stage detection + brain playbooks
  tool/                           web_search/web_fetch + file & shell verbs
  assets/                         CHEW + GUM sprite source data
gum-keys/                       public Gum Key templates
bin/<platform>/                 bundled llama-server runtimes
brain/                          where Bonsai lives once installed (gitignored)
```

---

## Releases

CHEW ships via [GitHub Releases](https://github.com/chewgumlabs/CHEWAgent/releases).
Each tag matching `v*` triggers a CI build that cross-compiles all five
platform binaries and attaches them to the release page. `install.sh`
pulls from `releases/latest/download/chew-<platform>` so a fresh clone
gets the newest version automatically.

To cut a release as a maintainer:

```sh
git tag v0.2 -m "what changed"
git push origin v0.2
```

---

## See the mascot

```sh
go run ./cmd/chew/chat/testbed
```

A small playground for the CHEW + GUM NES sprites. Type `0..7` to step
through the CHEW frames, `gum 0..5` for GUM, `all` to see them all.

---

## License

Apache 2.0 for code. See [LICENSE](LICENSE).

The CHEW + GUM mascot art (under [`cmd/chew/chat/assets/`](cmd/chew/chat/assets/))
is licensed separately as **CC-BY-NC** — feel free to learn from it,
but don't use the frog to sell something. The art and the CHEW name
belong to [ChewGumLabs](https://shanecurry.com).

---

## Origin

Extracted from [ChewGumLabs/swarmlab](https://github.com/ChewGumLabs/swarmlab)
on 2026-05-05 as the public chassis half of a public/private split.
Built by [Shane Curry](https://shanecurry.com).
