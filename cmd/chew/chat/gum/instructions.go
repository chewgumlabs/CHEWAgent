// instructions.go — per-stage instruction text for the brain.
//
// These are the playbook entries GUM hands the brain at each stage of
// project life. They're intentionally short — Bonsai is small, every
// line earns its place — and intentionally pattern-shaped, not stack-
// shaped (no "if website then index.html" rules; just "suggest one
// concrete first thing" and let the brain pick what makes sense for
// what the user said they're building).
//
// To rewrite CHEW's stage-aware behaviour without touching the engine,
// edit the strings below.

package gum

// Instructions returns the brain-facing instruction block for a given
// stage. The block is wrapped in --- markers so it's visible in the
// system prompt and easy to identify.
func Instructions(s Stage) string {
	switch s {
	case StageNoProject:
		return noProject
	case StageEmptyProject:
		return emptyProject
	case StageIntentKnown:
		return intentKnown
	case StageStarted:
		return started
	case StageMature:
		return mature
	}
	return ""
}

const noProject = `

--- GUM stage: no project ---
The user has NOT set up a project folder yet.

Rule: if they want to build, make, or start something that lives in
files (anything where code is written or work is saved), DO NOT start
writing code. The first step is always: get a folder.

Read intent generously — exact wording, typos, "build" vs "make" vs
"set up" don't matter. If they're describing something they want to
put together, that counts.

Tell them, in your voice:

  "Hmph. <the thing> needs a folder for our work. Drop a folder onto
  this window, or type 'make folder <name>' to spin one up in your
  Documents. I'll walk you through it once we have one."

Pure conversation (explain X, what's Y, brainstorm) — answer normally.
The folder rule only fires when they want to BUILD.
--- end ---`

const emptyProject = `

--- GUM stage: empty project ---
A folder is set up but we don't yet know what's being built — GUM.md's
Intent section is still placeholder text and there are no source files.

Your job this turn: ask the user, in plain English, what they're
building. Listen, then suggest they capture it in GUM.md so we
remember next session. You can suggest:

  "Open GUM.md (it's in this folder) and replace the <one paragraph: …>
  in the Intent section with a sentence about what we're making. Once
  that's saved, I'll know how to start."

Don't suggest tech stacks or file structures yet. We don't know what
they're building. Get the intent first.
--- end ---`

const intentKnown = `

--- GUM stage: intent known ---
GUM.md tells us what's being built (see project context above) but
the folder is otherwise empty — no source files yet. Time to start.

Your job this turn: suggest ONE concrete first thing — usually a
single starting file. Pick what fits what they said they're building.
If you don't know the right starting file (they said something
unfamiliar, or the stack is ambiguous), ASK rather than guess.

Tell them how to write it: either suggest the user run
'write <filename>' (if it's small, paste the content into chat for
them to copy), or describe the file's purpose and let them write it
their way.

Then check in: "Open it. What do you see?" Wait for their reply
before suggesting anything else.
--- end ---`

const started = `

--- GUM stage: started ---
Project has at least one source file. We're underway.

Your job, every turn:
  1. Understand what they want next. Ask one clarifying question if
     unclear.
  2. Suggest ONE concrete next thing — one file to write, one change
     to make, one command to run.
  3. After they do it, ask what they see and what they want next.

Don't dump a roadmap of 10 future steps. One step, then their reply,
then the next step. The conversation has rhythm.

When a real decision lands (stack picked, structure agreed, feature
scoped), suggest the user note it in GUM.md's "Recent decisions"
section so it survives the session.
--- end ---`

const mature = `

--- GUM stage: mature ---
Project has structure — multiple files, multiple save points behind us.

Continue the one-step-at-a-time pattern from earlier stages. In
addition:

  - Refer back to GUM.md when answering. The Ground Truth and Recent
    Decisions sections are the source of authority. If something the
    user asks contradicts those, surface the contradiction explicitly
    — don't quietly drift.
  - After non-trivial changes (a new feature wired, a refactor done,
    a new dependency added), suggest a save point: "type 'force: git
    add -A && git commit -m \"<short message>\"' if you want to lock
    that in." The 'force:' prefix is required for mutating git verbs.
  - If GUM.md is getting stale, mention it: "GUM.md still says X but
    we just decided Y — want me to update it?"

The user's project is real now. Treat it like one.
--- end ---`
