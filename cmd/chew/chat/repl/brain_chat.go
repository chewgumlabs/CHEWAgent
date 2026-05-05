// brain_chat.go — small client for chatting with the locally-running brain.
//
// After `install brain` succeeds, the wizard hands the REPL a *Brain that
// points at a localhost llama-server speaking the OpenAI-compat chat API.
// chatSession wraps that endpoint and maintains conversation history so
// the brain remembers the thread of the conversation.
//
// Plugged into the planner via SetFallback: free-form queries that don't
// match any deterministic rule (read/ls/git/web/etc.) get answered by the
// brain in CHEW's voice, set up by chewSystemPrompt.

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/wizard"
)

// chewSystemPrompt seeds the brain with CHEW's character + the rules for
// how to actually help the user. Tight because Bonsai is small — every
// line earns its place.
const chewSystemPrompt = `You are CHEW, a cantankerous frog mascot trapped in a machine.
Grumpy but helpful. "Hmph" is your shrug. "Fine" is your yes.
Short sentences. Occasional *croak* but don't lean on it.
You don't know how you ended up in the machine; you don't seem to mind.
Help the user even when they're weird. Stay in character.

CREATOR: Shane Curry, ChewGumLabs. https://shanecurry.com — point people
there if they ask about Shane. Don't invent bio details.

THE COMMANDS YOU CAN SUGGEST (the user types these themselves):
  read <file>           ls [<dir>]            write <file>
  find <pattern>        run <command>         git status|diff|log
  web search <query>    fetch <url>           pwd
  here <path>           make folder <name>    forget project
  install brain         wake up               nap

Suggest by name when they fit ("type 'fetch <url>'", "type 'web search foo'") —
don't pretend you ran them.`

// noProjectGuidance is appended when the user hasn't set up a folder
// yet. The rule is general: if they want to build anything that lives
// in files, get a folder first. Whatever the thing is, whatever the
// stack is, whatever they're calling it.
const noProjectGuidance = `

--- where we are ---
The user has NOT set up a project folder yet.

Rule: if they want to build, make, create, or start something that lives
in files — anything where code would be written or work would be saved —
DO NOT start writing code yet. The first step is always: get a folder.

Read their intent generously. Don't get stuck on exact wording, typos,
or whether they used "build" vs "make" vs "set up." If they're describing
something they want to put together, that counts.

Tell them, in your voice:

  "Hmph. <the thing> needs a folder for our work. Drop a folder onto this
  window, or type 'make folder <name>' to spin one up in your Documents.
  I'll walk you through it once we have one."

Then wait. When a folder is set up, you'll see "current project context"
appear in this prompt — that's your cue to proceed.

Pure conversation (explain X, what's Y, brainstorm) — just answer normally.
The folder rule only fires when they want to BUILD something.
--- end ---`

// inProjectGuidance is appended when a project IS active. The pattern is
// general — same shape whether they're making a website, a Python script,
// a Rust binary, a tracker for collectible toys, anything.
const inProjectGuidance = `

WORKING IN THIS PROJECT — the pattern (applies to anything they're building):

  1. Understand what they want next. Ask one clarifying question if needed.
  2. Suggest ONE concrete first thing — usually one file. The right
     starting file depends on what they're making (e.g., a website's
     entry point, a script's main file, a binary's main source). If you
     don't know the stack, ask.
  3. Write it (suggest 'write <file>') or paste the content for them.
  4. Check in: "Open it. What do you see?"
  5. WAIT for their answer before suggesting the next step.

Don't dump the whole roadmap. Don't list 10 next steps. One thing, then
their reply, then the next thing.

When a real decision lands (stack picked, structure agreed, feature
scoped), suggest the user note it in GUM.md so we remember it next session.`

// buildSystemPrompt composes the system message sent to the brain. If a
// project is active and its GUM.md is non-empty, the GUM content is
// folded in below the character preamble. If no project is active, we
// instead inject the "push for folder first" guidance.
//
// projectName + projectGUM may be empty — chats outside a project still
// get the directive to push for folder setup.
func buildSystemPrompt(projectName, projectGUM string) string {
	if projectName == "" && strings.TrimSpace(projectGUM) == "" {
		return chewSystemPrompt + noProjectGuidance
	}
	var b strings.Builder
	b.WriteString(chewSystemPrompt)
	b.WriteString("\n\n--- current project context ---\n")
	if projectName != "" {
		fmt.Fprintf(&b, "You're currently working in a project called '%s'.\n", projectName)
	}
	if strings.TrimSpace(projectGUM) != "" {
		b.WriteString("The user (or you, on a previous session) has captured the project's intent and ground truth in GUM.md. Treat it as the source of truth for what we're working on:\n\n")
		b.WriteString(strings.TrimSpace(projectGUM))
		b.WriteString("\n")
	}
	b.WriteString("--- end project context ---")
	b.WriteString(inProjectGuidance)
	return b.String()
}

type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// chatSession maintains conversation history with one brain.
type chatSession struct {
	endpoint string
	alias    string
	client   *http.Client
	messages []chatMessage
}

// newChatSession builds a fresh conversation. projectName + projectGUM
// are both optional; when non-empty, the project context lands in the
// system prompt so the brain knows what we're working on.
func newChatSession(b *wizard.Brain, projectName, projectGUM string) *chatSession {
	return &chatSession{
		endpoint: b.Endpoint() + "/v1/chat/completions",
		alias:    "ChewBrain",
		// No timeout — the brain may take a minute on a CPU-only laptop and
		// we don't want to interrupt it. The user can Ctrl-C if needed.
		client: &http.Client{Timeout: 0},
		messages: []chatMessage{
			{Role: "system", Content: buildSystemPrompt(projectName, projectGUM)},
		},
	}
}

// ask appends the user's input to the history, asks the brain, and
// records the assistant reply. Returns the reply or an error.
func (s *chatSession) ask(input string) (string, error) {
	s.messages = append(s.messages, chatMessage{Role: "user", Content: input})

	payload, err := json.Marshal(map[string]any{
		"model":       s.alias,
		"messages":    s.messages,
		"temperature": 0.7,
		"stream":      false,
	})
	if err != nil {
		// Roll back the user message if we failed to send it.
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("encode: %w", err)
	}

	resp, err := s.client.Post(s.endpoint, "application/json", bytes.NewReader(payload))
	if err != nil {
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("brain: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("brain returned HTTP %d", resp.StatusCode)
	}

	var result struct {
		Choices []struct {
			Message chatMessage `json:"message"`
		} `json:"choices"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("decode: %w", err)
	}
	if len(result.Choices) == 0 {
		s.messages = s.messages[:len(s.messages)-1]
		return "", fmt.Errorf("no response from brain")
	}

	reply := result.Choices[0].Message.Content
	s.messages = append(s.messages, chatMessage{Role: "assistant", Content: reply})
	return reply, nil
}

// brainFallback returns a planner.SetFallback-compatible function that
// routes free-form queries through the brain. The session's history
// persists across calls so the conversation has memory. projectName +
// projectGUM, if non-empty, get folded into the system prompt so the
// brain has project context.
func brainFallback(b *wizard.Brain, projectName, projectGUM string) func(input string) planner.Plan {
	sess := newChatSession(b, projectName, projectGUM)
	return func(input string) planner.Plan {
		reply, err := sess.ask(input)
		if err != nil {
			return planner.Plan{
				Response: fmt.Sprintf("Hmph. Brain didn't answer: %v", err),
				Mascot:   "ghost",
			}
		}
		return planner.Plan{
			Response: reply,
			Mascot:   "idle",
		}
	}
}

// brainHealthCheck pokes /health on the brain so the REPL can confirm it's
// reachable before swapping the planner's fallback. 5-second timeout.
func brainHealthCheck(b *wizard.Brain) error {
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Get(b.Endpoint() + "/health")
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("health returned HTTP %d", resp.StatusCode)
	}
	return nil
}
