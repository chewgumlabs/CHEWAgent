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

	// install_intro — opener line on the install-brain page. Body is in
	// installBody. Slots: none.
	"install_intro": {
		`Install-brain plan. Hmph.`,
		`Brain transplant time. Here's how:`,
		`Putting a brain in this frog. The plan:`,
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
  find <pattern>        search for a pattern in files
  run <command>         run a shell command
  git status|diff|log   read-only git operations
  pwd                   show current directory
  status                check brain status
  install brain         upgrade me to use an LLM
  list                  show all command patterns
  help                  this text
  quit                  exit

For free-form questions ('explain X', 'what's wrong with Y'),
I need a brain. Type 'install brain' to install one locally.`

// installBody — the static plan shown when the user types `install brain`.
var installBody = `To put a brain in this frog, I need a local LLM. We'll use Bonsai —
an 8B-class model packed into 1.16 GB. Apache 2.0 licensed, runs in
llama.cpp, lives entirely on your machine. No accounts, no telemetry.

The plan when you say 'yes':
  [1/3] Check that llama-server is installed (brew install llama.cpp on Mac).
  [2/3] Download Bonsai (1.16 GB) into ~/.chew/models/.
  [3/3] Write a config so I know where to find it.

Total disk: about 1.2 GB. No GPU required — Bonsai runs on plain CPU.

Reply 'yes' to start, 'tell me more' for the why, or 'cancel' to skip.`
