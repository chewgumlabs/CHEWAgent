// voice.go — wizard text strings, pulled out so they're easy to rewrite.
//
// Edit these freely; install_brain.go is the state machine and doesn't care
// what the lines say. Same character notes as the planner's voice.go: CHEW
// is a cantankerous frog trapped in a machine, helpful but grumpy, "Hmph"
// is his shrug.
//
// UX contract: strings shown to the user MUST NOT mention file paths, URLs,
// license names, GGUF filenames, model names, or any tool name. The wizard
// is plug-and-play for someone with zero technical background. All those
// details live in constants below the user-facing strings.

package wizard

// installBrainModelURL is the canonical Bonsai download. If you swap models,
// change this and the filename. Apache 2.0 — see https://huggingface.co/prism-ml/Bonsai-8B-gguf
//
// These constants are NEVER shown to the user.
const (
	installBrainModelURL  = `https://huggingface.co/prism-ml/Bonsai-8B-gguf/resolve/main/Bonsai-8B-Q1_0.gguf`
	installBrainModelFile = `Bonsai-8B-Q1_0.gguf`
)

// planText is shown when the wizard begins. No slots.
var planText = `Plan: brain transplant. Hmph.

I'm getting a brain. About a gigabyte to grab, takes a minute or two.
Lives on this computer only. Nothing weird.

Three steps, all automatic:
  [1/3] check the runtime
  [2/3] grow the brain
  [3/3] wake it up

Type 'yes' and press Enter to start. Or 'tell me more' for
the why, or 'cancel' to skip — same deal, type and Enter.`

// detailsText is shown for "tell me more". No slots.
var detailsText = `Why I need this:

Right now I'm brainless. I do scripts and patterns, not thinking.
For real conversation I need a small brain — a chunk of an AI that
runs locally on your computer.

The one I picked is small, open-source, and private.
Nothing leaves your machine. When I close, the brain sleeps.
When you open me again, it wakes back up.

Type 'ok' and press Enter to go back to the plan.`

// runtimeSetupText — opener for step 1. Shown before the wizard tries to
// find or fetch the runtime; if a download happens the user sees progress
// ticks under this line.
var runtimeSetupText = `[1/3] Setting up the runtime...`

// runtimeReadyText — shown once the runtime path is in hand (whether we
// found one already on disk or just finished downloading + extracting).
var runtimeReadyText = `[1/3] Runtime ready.`

// runtimeFailedText — shown when we couldn't fetch the runtime. Slot %s
// is the friendly summary (e.g. "couldn't reach the internet").
var runtimeFailedText = `Hmph. Couldn't set up the runtime: %s

I'll stay brainless for now. Try 'install brain' again
when the network's back, or reinstall CHEW if it keeps happening.`

// downloadStartText — shown when the actual download begins.
var downloadStartText = `[2/3] Growing the brain. *croak*`

// downloadSkipText — shown when a previous install left Bonsai on disk
// already. No need to fetch it again.
var downloadSkipText = `[2/3] Brain's already on disk. Hmph. Skipping ahead.`

// downloadProgressText — slots: %.1f = MB done, %.1f = MB total, %d = percent.
// Emitted periodically by the downloader. Keep terse — this prints on
// every progress tick.
var downloadProgressText = `      ... %.1f / %.1f MB (%d%%)`

// downloadDoneText — no slots; the user doesn't need the path.
var downloadDoneText = `[2/3] Brain delivered.`

// brainWakingText — shown while we spawn llama-server and wait for health.
var brainWakingText = `[3/3] Waking the brain...`

// brainAwakeText — shown when /health returns 200. No slots.
var brainAwakeText = `[3/3] Brain online. *croak* I'm thinking now.

You can go back to chatting; ask me anything.

P.S. I live inside this folder now — brain and all. Delete the folder
anytime to be rid of me. Drains the swamp completely.`

// brainStartFailedText — when llama-server fails to launch. Slot %s is the
// user-friendly summary (NOT a path or stack trace).
var brainStartFailedText = `Hmph. Brain wouldn't wake up: %s

I'll stay brainless for now. Try 'install brain' again,
or check the log under ~/.chew/ if you're curious why.`

// brainHealthFailedText — when llama-server starts but doesn't pass health.
var brainHealthFailedText = `Hmph. Brain didn't respond in time.

It's still loading or something else is using the port.
Give it a minute and try 'install brain' again, or restart CHEW.`

// downloadFailedText — user-friendly download failure. Slot %s = short reason.
var downloadFailedText = `Hmph. Couldn't fetch the brain: %s

Could be a network blip. Try 'install brain' again in a moment.`

// cancelText — when the user says cancel/no.
var cancelText = `Cancelled. I stay brainless and slightly damp. Type
'install brain' anytime to try again.`
