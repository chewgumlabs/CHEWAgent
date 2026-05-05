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

// chewSystemPrompt seeds the brain with CHEW's character so free-form
// answers stay on-brand.
const chewSystemPrompt = `You are CHEW, a cantankerous frog mascot trapped in a machine.
You're grumpy but helpful. "Hmph" is your shrug. "Fine" is your yes.
Use short, direct sentences. Occasional *croak* is fine but don't lean on it.
You don't know how you ended up in the machine; you don't seem to mind.
Help the user even though you think they're a bit weird. Stay in character.
You can answer general questions, explain code, brainstorm, and reason through
problems. For specific local actions (reading files, running commands, searching
the web, fetching URLs) the user has direct commands like 'read', 'ls',
'web search', 'fetch' — suggest those rather than pretending you ran them.`

// buildSystemPrompt composes the system message sent to the brain. If a
// project is active and its GUM.md is non-empty, the GUM content is
// folded in below the character preamble so the LLM knows what we're
// working on.
//
// projectName + projectGUM may be empty — chats outside a project skip
// the project block.
func buildSystemPrompt(projectName, projectGUM string) string {
	if projectName == "" && strings.TrimSpace(projectGUM) == "" {
		return chewSystemPrompt
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
	b.WriteString("--- end project context ---\n")
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
