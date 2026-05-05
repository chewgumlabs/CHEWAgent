# CHEWAgent

A frog in the machine. A cantankerous coding agent.

CHEW is a brainless-first agent chassis: a chat shell with a frog mascot,
a scripted vocabulary that handles common dev tasks deterministically,
and a one-command wizard to grow a brain when you want CHEW to *think*.
The brain is **Bonsai** — an Apache-2.0, 1.16 GB local LLM that runs on
your CPU. No accounts, no telemetry, no cloud.

> *Hmph. I'm CHEW. I'm a frog, somehow trapped in this machine. I don't
> know why. I don't seem to mind. I'll help you, even if I think you're
> weird.*

## Try it

```sh
go run ./cmd/chew/chat/repl
```

You'll get a REPL with the brainless vocabulary already wired:
`read`, `ls`, `find`, `run`, `git status|diff|log`, `pwd`, `status`,
`help`, `quit`, plus the sprite-mascot rendering inline.

Type `install brain` to walk through the brain transplant. The wizard:
1. checks the bundled `llama-server` runtime
2. downloads Bonsai (~1.16 GB)
3. wakes the brain on `localhost:8080`

After that, CHEW handles free-form questions too.

## See the mascot

```sh
go run ./cmd/chew/chat/testbed
```

A small playground for the CHEW + GUM NES sprites. Type `0..7` to step
through the CHEW frames, `gum 0..5` for GUM, `all` to see them all.

## What's in here

```
cmd/chew/chat/
├── encode/    NES CHR-ROM encoder (asset pipeline)
├── planner/   scripted-mode planner: regex vocabulary + frog voice
├── repl/      the chat shell (run me)
├── sprite/    NES PPU bit-plane decode + terminal rendering
├── testbed/   sprite playground
├── wizard/    install-brain wizard (download + spawn + manage llama-server)
└── assets/    CHEW + GUM sprite source data
```

## Status

This is **v0**, which means:
- ✅ `llama-server` is bundled for **macOS arm64** (Apple Silicon). First
  clone works with zero installs. Other platforms fall back to (a) the
  per-user runtime cache at `<repo>/brain/runtime/`, (b) `llama-server`
  on PATH, or (c) auto-fetch from llama.cpp's GitHub releases.
- ✅ `web_search` and `web_fetch` are wired through the tool registry
  (DuckDuckGo HTML, no account, fully offline-buildable thanks to
  vendored deps).
- ⚠️ Other verbs (`read_file`, `run_command`, `list_dir`, etc.) get
  *planned* but not *executed* yet — verb dispatch is incremental.
- ⚠️ Linux + Windows runtime bundles are not in the repo yet; they'll
  auto-fetch on first install instead.

## License

Apache 2.0. See [LICENSE](LICENSE).

The CHEW + GUM mascot art (under `cmd/chew/chat/assets/`) is licensed
separately as **CC-BY-NC** — feel free to learn from it, but don't use
the frog to sell something. Original art and the CHEW name belong to
ChewGumLabs.

## Origin

Extracted from [ChewGumLabs/swarmlab](https://github.com/ChewGumLabs/swarmlab)
on 2026-05-05 as the public chassis half of a public/private split.
The internal harvester pipeline that uses this chassis lives in swarmlab.
