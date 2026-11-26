package app

import (
	"fmt"
	"strings"

	"agent-gui/domain"
)

// The context window is what gets sent to the model. It is cut from the
// history for each request; the history itself is never shortened, because it
// is what gets saved.

// estimateTokens uses the rough heuristic of 1 token ≈ 4 characters.
func estimateTokens(msgs []domain.Message) int {
	total := 0
	for _, m := range msgs {
		total += len(m.Content)
	}
	return total / 4
}

// startsTurn reports whether m is something the user said, as opposed to a
// tool result fed back to the model.
func startsTurn(m domain.Message) bool {
	return m.Role == domain.RoleUser && !strings.HasPrefix(m.Content, toolResultPrefix)
}

// splitTurns groups history so that a user message stays with the replies,
// tool calls and tool results that followed it.
func splitTurns(history []domain.Message) [][]domain.Message {
	var turns [][]domain.Message
	start := 0
	for i := 1; i < len(history); i++ {
		if startsTurn(history[i]) {
			turns = append(turns, history[start:i])
			start = i
		}
	}
	if len(history) > 0 {
		turns = append(turns, history[start:])
	}
	return turns
}

// window returns the system prompt plus as many of the most recent whole turns
// as fit. A quarter of the limit is left for the reply. The current turn is
// always included; if it alone is too large, its tool results are shortened,
// oldest first.
func (a *ConversationalAgent) window() []domain.Message {
	history := a.conv.History()
	if a.contextLimit <= 0 || len(history) == 0 {
		return a.conv.Full()
	}
	budget := a.contextLimit*3/4 - len(a.conv.SystemPrompt)/4

	turns := splitTurns(history)
	last := append([]domain.Message(nil), turns[len(turns)-1]...)
	used := estimateTokens(last)
	first := len(turns) - 1
	for first > 0 {
		cost := estimateTokens(turns[first-1])
		if used+cost > budget {
			break
		}
		used += cost
		first--
	}

	var kept []domain.Message
	for _, t := range turns[first : len(turns)-1] {
		kept = append(kept, t...)
	}
	// The newest message is what the model must respond to, so it stays whole.
	for i := 0; i < len(last)-1 && used > budget; i++ {
		if last[i].Role != domain.RoleUser || startsTurn(last[i]) || len(last[i].Content) < 400 {
			continue
		}
		head, _, _ := strings.Cut(last[i].Content, ":\n")
		short := fmt.Sprintf("%s:\n[result omitted to fit the context limit: %d bytes]", head, len(last[i].Content))
		used -= (len(last[i].Content) - len(short)) / 4
		last[i].Content = short
	}

	full := &domain.Conversation{SystemPrompt: a.conv.SystemPrompt, Messages: append(kept, last...)}
	return full.Full()
}
