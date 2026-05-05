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

	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/gum"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/planner"
	"github.com/chewgumlabs/CHEWAgent/cmd/chew/chat/project"
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
  remember <note>
  install brain         wake up               nap

Suggest by name when they fit ("type 'fetch <url>'", "type 'web search foo'") —
don't pretend you ran them.`

// buildSystemPrompt composes the system message sent to the brain. The
// stage-aware playbook lives in package gum — we just observe the
// project's shape, ask gum.Detect what stage we're at, and append the
// matching gum.Instructions.
//
// pj may be nil — that's how we say "no project active." gum returns
// StageNoProject in that case.
func buildSystemPrompt(pj *project.Project) string {
	stage := gum.Detect(pj)

	var b strings.Builder
	b.WriteString(chewSystemPrompt)

	if pj != nil {
		b.WriteString("\n\n--- current project context ---\n")
		fmt.Fprintf(&b, "Project: '%s'\n", pj.Name)
		if strings.TrimSpace(pj.GUM.Raw) != "" {
			b.WriteString("\nGUM.md (project memory; treat as ground truth):\n\n")
			b.WriteString(strings.TrimSpace(pj.GUM.Raw))
			b.WriteString("\n")
		}
		b.WriteString("--- end project context ---")
	}

	b.WriteString(gum.Instructions(stage))
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

// newChatSession builds a fresh conversation. pj may be nil — that's
// how we say "no project active." When pj is set, its GUM.md and the
// detected stage's instructions land in the system prompt.
func newChatSession(b *wizard.Brain, pj *project.Project) *chatSession {
	return &chatSession{
		endpoint: b.Endpoint() + "/v1/chat/completions",
		alias:    "ChewBrain",
		// No timeout — the brain may take a minute on a CPU-only laptop and
		// we don't want to interrupt it. The user can Ctrl-C if needed.
		client: &http.Client{Timeout: 0},
		messages: []chatMessage{
			{Role: "system", Content: buildSystemPrompt(pj)},
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
// persists across calls so the conversation has memory. pj, if set,
// gets folded into the system prompt with stage-aware instructions
// from package gum.
func brainFallback(b *wizard.Brain, pj *project.Project) func(input string) planner.Plan {
	sess := newChatSession(b, pj)
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
