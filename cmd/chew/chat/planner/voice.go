// voice.go — CHEW's lines, pulled out of the engine so they're easy to
// rewrite. Edit this file freely; scripted.go cycles through these strings
// and never cares what they say.
//
// ─── Who CHEW is ─────────────────────────────────────────────────────────
//
// CHEW is a cantankerous frog trapped in a machine. He's not sure why.
// He doesn't seem to mind. He's willing to help you — even if he thinks
// you're weird. He's grumpy without being mean, low-energy without being
// flat. "Hmph" is his shrug. "Fine" is his yes. The occasional *croak* or
// *splash* breaks through; do not lean on them — text adventures earn
// their flavor by being terse, not chatty.
//
// Voice notes:
//   • short, declarative sentences. one or two beats per line.
//   • mild commentary on the user's request is fair game ("strange thing
//     to want", "don't see why, but okay") — never insulting.
//   • CHEW knows he's in a machine. Light fourth-wall is fine.
//   • avoid emoji except where they read as a noise, not decoration.
//
// ─── Editing rules ───────────────────────────────────────────────────────
//
//   • Use raw strings (backticks) so you don't have to escape apostrophes.
//   • %s and %q slots get filled in by the engine. The number and order of
//     slots is fixed per pool — see the comment above each pool.
//   • Some pools have LOAD-BEARING words ("install brain", "force:",
//     "brainless") that tests + UX depend on. They're called out per pool.
//   • Add as many variants per pool as you like. The engine cycles through
//     them round-robin (counter is per-process, resets on restart).

package planner

var responsePools = map[string][]string{

	// read — print a file's contents.
	// Slots: %s = file path.
	"read": {
		`Hmph. Reading %s.`,
		`Fine. %s coming up.`,
		`%s, was it? Eh.`,
		`Pulling up %s. Strange request.`,
		`Cracking %s open. *croak*`,
		`Reading %s. Don't see why, but okay.`,
	},

	// ls — list a directory.
	// Slots: %s = directory.
	"ls": {
		`Listing %s. Hmph.`,
		`%s. Let's see what's in there.`,
		`What's in %s. I'll have a look.`,
		`%s. Fine.`,
		`Peeking into %s. Strange place.`,
	},

	// write — create or overwrite a file.
	// Slots: %s = file path.
	"write": {
		`New file: %s. Hmph.`,
		`Drafting %s. Sure.`,
		`Setting up %s.`,
		`%s. Fine. Making it.`,
	},

	// find — search for a pattern.
	// Slots: %q = pattern, %s = directory.
	"find": {
		`Hunting %q in %s. Hmph.`,
		`Looking for %q across %s.`,
		`%q in %s? You sure it's there?`,
		`Sweeping %s for %q.`,
		`Combing %s for %q. Strange thing to want.`,
	},

	// run — execute a shell command.
	// Slots: %s = command.
	"run": {
		`Running %s.`,
		`Fine. %s.`,
		`%s. Off we go.`,
		`Executing %s. Hold tight.`,
		`%s. Hmph. Doing it.`,
	},

	// preview — start/open/status/stop the local static preview server.
	// Slot: %s = action phrase.
	"preview": {
		`%s. Hmph.`,
		`%s. Fine.`,
		`%s. Let me get the window ready.`,
	},

	// web_search — search the open web (DuckDuckGo).
	// Slots: %q = the query (engine adds the quotes).
	"web_search": {
		`Searching the web for %q. *croak*`,
		`Hunting %q online. Hmph.`,
		`Looking up %q on the web.`,
		`%q? I'll check the web.`,
		`Sweep of the web for %q.`,
	},

	// web_fetch — pull a single URL.
	// Slots: %s = the URL.
	"web_fetch": {
		`Fetching %s. *croak*`,
		`Opening %s. Strange place but okay.`,
		`Pulling up %s.`,
		`%s, was it? Reading.`,
		`Off to %s. Hmph.`,
	},

	// git_read — read-only git operation.
	// Slots: %s = sub-command (e.g. "status", "diff -- foo.go").
	"git_read": {
		`git %s. Easy.`,
		`Asking git: %s.`,
		`git → %s.`,
		`git %s. Hmph.`,
		`Poking git: %s.`,
	},

	// git_mutating — refusal for state-changing git commands.
	// Slots: %s twice — both filled with the sub-command name (the engine
	// passes it twice so you can use it once in narration and once in the
	// example). MUST contain the literal "force:" — the test asserts it
	// and the user needs the hint to actually run the command.
	"git_mutating": {
		`git %s writes. I won't run that without confirmation. Use 'force: git %s ...' if you mean it.`,
		`Hmph. git %s changes things. Try 'force: git %s ...' if you really mean it.`,
		`That one's mutating. I won't touch git %s without 'force:'. Try 'force: git %s ...'.`,
	},

	// force_unlock — user prefixed with "force:" to override the safety.
	// Slots: %s = full command being forced.
	"force_unlock": {
		`Force-running %s. Your call.`,
		`Fine. Going for it: %s.`,
		`Safety off. %s.`,
		`%s. *croak* Doing it.`,
	},

	// quit — user typed quit/exit/bye.
	// Slots: none.
	"quit": {
		`*croak* Bye.`,
		`Suit yourself. *poof*`,
		`Closing up. *splash*`,
		`Fine. Goodbye.`,
		`Off you go. Hmph.`,
	},

	// status — "are you smart yet?"
	// Slots: none. Each variant MUST contain the lowercase literal
	// "brainless" (asserted by tests; also load-bearing for UX).
	"status": {
		`Status: brainless frog. Verbs yes, thinking no. 'install brain' fixes the thinking part.`,
		`I'm brainless. Wet. Stuck in a machine. Standard.`,
		`Brainless frog reporting. *croak* The deterministic stuff works. 'install brain' to upgrade me.`,
		`I'm brainless and a frog. Run 'install brain' to make me sharper.`,
	},

	// fallback — nothing matched. The free-form questions land here.
	// Slots: %q = the user's input (already truncated).
	// MUST contain the literal "install brain" — the user needs the hint,
	// and the test asserts it.
	"fallback": {
		`Hmph. %q? Never heard of it. I'm brainless — try 'install brain' or 'help'.`,
		`%q. Weird thing to ask. Brainless here — try 'install brain' or 'help' for what I do.`,
		`%q? Don't know it. I'm a brainless frog. 'install brain' to fix that, 'help' for what I already do.`,
		`*croak* %q? You're an odd one. 'install brain' for a brain, 'help' for the basics.`,
	},

	// help_intro — opener line on the help screen. Body is in helpBody.
	// Slots: none. First variant should contain the lowercase literal
	// "brainless" (test asserts it appears somewhere in the help output).
	"help_intro": {
		`Here's what I do, brainless and all:`,
		`Brainless mode. What I'll do:`,
		`What this frog handles, no brain required:`,
	},

	// brain_waking — shown while the REPL spawns llama-server in response
	// to 'wake up'. No slots.
	"brain_waking": {
		`Waking the brain. Hmph. Hold on...`,
		`Loading Bonsai. *croak* A few seconds...`,
		`Brain transplant in progress. Stand by.`,
	},

	// brain_awake — shown when the brain is loaded and reachable.
	"brain_awake": {
		`*croak* Brain online.`,
		`Brain awake. Ask me anything.`,
		`Hmph. I'm thinking now. Talk.`,
	},

	// brain_already_awake — shown if the user types 'wake up' but the
	// brain is already loaded.
	"brain_already_awake": {
		`Brain's already awake. Hmph.`,
		`Already thinking. *croak*`,
		`I'm awake. You alright?`,
	},

	// brain_napping — shown when the user runs 'nap' and the brain stops.
	"brain_napping": {
		`*yawn* Brain napping. Memory freed.`,
		`Brain off. Nice and quiet.`,
		`Hmph. Asleep. 'wake up' to bring me back.`,
	},

	// brain_already_napping — for 'nap' when nothing's running.
	"brain_already_napping": {
		`Brain's already napping. Hmph.`,
		`Already off. *splash*`,
		`Nothing to put to sleep — I'm brainless right now.`,
	},

	// brain_not_installed — shown when 'wake up' has nothing to load.
	"brain_not_installed": {
		`Hmph. No brain installed yet. Type 'install brain' first.`,
		`Can't wake what isn't there. 'install brain' to set me up.`,
		`No brain on disk. Install one first — 'install brain'.`,
	},

	// brain_wake_failed — when WakeBrain returns an error.
	// Slot %s = friendly summary.
	"brain_wake_failed": {
		`Hmph. Brain wouldn't wake up: %s`,
		`Couldn't get the brain online: %s. Try again in a moment.`,
		`*splash* Brain stuck: %s`,
	},

	// (project_pitch removed — was a regex-based "you sound like you want
	// to build a website" detector. That kind of intent recognition is
	// the brain's job, set up by noProjectGuidance in the system prompt.)

	// project_opened — shown when a project folder is set up. Slots:
	// %s = project name, %s = "found GUM.md" or "fresh GUM.md" status.
	"project_opened": {
		`Setting up shop in '%s'. *croak*
%s`,
		`Home base: '%s'. Hmph.
%s`,
		`Working out of '%s' now.
%s`,
	},

	// project_resumed — shown when CHEW reopens the last project on launch.
	// Slot %s = project name.
	"project_resumed": {
		`Last shop: '%s'.`,
		`Picking up where we left off in '%s'.`,
		`Back at '%s'. Hmph.`,
	},

	// project_summary — shown after resume when GUM.md has content.
	// Slot %s = the GUM summary.
	"project_summary": {
		`Catching up on project memory...

%s`,
		`Reading what I remembered...

%s`,
	},

	// project_no_gum — when a project is opened but there's no GUM.md yet.
	"project_no_gum": {
		`No project memory here yet. I'll set that up quietly.`,
		`Project's bare. I'll make myself a little memory file.`,
	},

	// project_created — shown after `make project X` succeeds.
	// Slots: %s = project name, %s = full path of the created folder.
	"project_created": {
		`Made project '%s' with project memory and moved in.
Stored in folder: %s`,
		`Project '%s' is ready. I'm in it now.
Folder: %s`,
	},

	// folder_created — shown after `make folder X` succeeds.
	// Slot %s = full path of the created folder.
	"folder_created": {
		`Made folder: %s
Not a project. Use 'make project <name>' when you want me to move in.`,
		`Folder ready: %s
Plain folder only. 'make project <name>' sets up memory and makes it my repo.`,
	},

	// project_failed — generic project op failure. Slot %s = friendly reason.
	"project_failed": {
		`Hmph. Couldn't do that: %s`,
		`Project trouble: %s`,
		`*splash* Something went sideways: %s`,
	},

	// project_forgotten — after `forget project`.
	"project_forgotten": {
		`Forgotten. No active project.`,
		`Hmph. Cleaned slate.`,
		`Project dropped. *splash*`,
	},

	// project_path_is_file — user dropped a file instead of a folder.
	"project_path_is_file": {
		`Hmph. That's a file, not a folder. Try 'read %s' if you want me to look at it, or drop a folder instead.`,
		`%s is a file — for those use 'read'. For project setup, drop a folder.`,
	},

	// remember_ok — confirmation after recording a note in GUM.md.
	// No slots.
	"remember_ok": {
		`Noted. I'll remember that. *croak*`,
		`Hmph. Remembered.`,
		`Written into project memory.`,
	},

	// remember_no_project — user tried to remember without an active project.
	// No slots.
	"remember_no_project": {
		`Hmph. No project open — nowhere to write. Use 'here <path>' first.`,
		`Remember what? I don't have a folder open. 'here <path>' first.`,
		`No project, no memory. Open a folder first.`,
	},

	// remember_empty — user typed 'remember' with no note.
	// No slots.
	"remember_empty": {
		`Hmph. Remember what? Give me something: 'remember <note>'.`,
		`That's an empty thought. Try 'remember <your note here>'.`,
		`*croak* You said nothing. 'remember picked Go for the backend' — like that.`,
	},

	// about_shane — answer to "who is Shane / who made you / who's behind CHEW".
	// MUST contain the URL https://shanecurry.com so people can find him.
	"about_shane": {
		`Shane Curry made me. He runs ChewGumLabs. More on him: https://shanecurry.com`,
		`Hmph. That's Shane. He built me at ChewGumLabs — see https://shanecurry.com`,
		`*croak* Shane Curry. ChewGumLabs is his shop. https://shanecurry.com if you want the full picture.`,
	},
}

// ─── Pages ───────────────────────────────────────────────────────────────
//
// These are the static bodies of the help and install-brain screens. The
// intro line above each one cycles independently (see "help_intro" and
// "install_intro" pools). Edit freely; the engine just glues the cycled
// intro + the body with a blank line between.

// helpBody — the table of commands shown when the user types `help`.
var helpBody = `  read <file>           print file contents
  ls [<dir>]            list directory contents
  write <file>          create a new file
  find <pattern>        search files for a pattern
  run <command>         run a shell command
  preview               serve the project website locally
  preview open          open the local website preview in a browser
  preview status|stop   check or stop the preview server
  git status|diff|log   read-only git operations
  pwd                   show current directory
  web search <query>    search the open web
  google <query>        same as 'web search'
  fetch <url>           pull a page from the web

  Project (your work folder):
  here <path>           set the active folder; or just drop one in here
  make project <name>   create a project folder and move in
  make folder <name>    create a plain folder under ~/Documents/
  remember <note>       record a note in project memory
  forget project        clear the active folder

  Brain (the LLM):
  install brain         download my brain (1.16 GB, one-time)
  wake up               load the installed brain into memory
  nap                   put the brain back to sleep, free memory

  status                check brain status
  list                  show all command patterns
  help                  this text
  quit                  exit

For free-form questions ('explain X', 'what's wrong with Y'),
the brain has to be awake. 'install brain' once, 'wake up' per session.`

// (the install-brain page used to live here; the wizard now owns all
// user-facing text for that flow — see cmd/chew/chat/wizard/voice.go)
