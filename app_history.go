package main

// Branching and the mapping between stored messages and transcript items.

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/history"
)

// displayItem mirrors the frontend Item type for serialising display history.
type displayItem struct {
	Type string         `json:"type"`
	Data map[string]any `json:"data"`
}

// BranchConversation creates a copy of the conversation up to the item at
// displayItemIndex. Branching from a user message sends that message again in
// the new branch, so it needs the agent to be idle.
func (a *App) BranchConversation(id string, displayItemIndex int) (*history.Conversation, error) {
	if a.GetRunState().RunID != "" {
		return nil, errors.New("stop the agent before branching")
	}
	src, err := history.Load(id)
	if err != nil {
		return nil, err
	}

	// Determine the cut point in the message slice.
	// Walk display items and saved messages in parallel so we know how many
	// saved messages to keep.
	items := parseDisplayItems(src.DisplayItems)
	if displayItemIndex < 0 || displayItemIndex >= len(items) {
		return nil, fmt.Errorf("invalid item index %d (max %d)", displayItemIndex, len(items)-1)
	}

	// Count how many visible user/assistant messages we need up to the branching
	// item. Branching from a user message copies the prior context and re-sends
	// that user message. Branching from an assistant message copies through that
	// assistant response and does not fabricate a user turn.
	visibleOrdinal := -1
	includeSelected := true
	resendText := ""
	msgCount := 0
	for i, it := range items {
		if it.Type != "msg" {
			continue
		}
		role, _ := it.Data["role"].(string)
		if role != "user" && role != "assistant" {
			continue
		}
		// Older transcripts stored replies that were only tool calls.
		if t, _ := it.Data["text"].(string); role == "assistant" && domain.StripToolCalls(t) == "" && i != displayItemIndex {
			continue
		}
		if i == displayItemIndex {
			text, _ := it.Data["text"].(string)
			visibleOrdinal = msgCount
			if role == "user" {
				includeSelected = false
				resendText = text
			}
			break
		}
		// There should be a matching SavedMessage for every display msg.
		msgCount++
	}
	cutMsgIdx := visibleMessageCut(src.Messages, visibleOrdinal, includeSelected)
	if cutMsgIdx < 0 || cutMsgIdx > len(src.Messages) {
		return nil, fmt.Errorf("could not map display item to message index")
	}

	now := time.Now()
	branch := &history.Conversation{
		ConvMeta: history.ConvMeta{
			ID:        conversationID("branch", 0),
			Title:     src.Title + " (branch)",
			WorkDir:   src.WorkDir,
			Model:     src.Model,
			ProfileID: src.ProfileID,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages:   make([]history.SavedMessage, cutMsgIdx),
		TokenUsage: src.TokenUsage,
	}
	copy(branch.Messages, src.Messages[:cutMsgIdx])
	if src.Summary != nil && src.Summary.Covered <= cutMsgIdx {
		branch.Summary = src.Summary
	}

	// Truncate display items to match the copied history. When re-sending a user
	// message, omit that original item so the new branch has one canonical turn.
	displayCut := displayItemIndex + 1
	if resendText != "" {
		displayCut = displayItemIndex
	}
	branchItems := make([]displayItem, displayCut)
	copy(branchItems, items[:displayCut])
	raw, _ := json.Marshal(branchItems)
	branch.DisplayItems = json.RawMessage(raw)

	if err := history.Save(branch); err != nil {
		return nil, err
	}

	a.setActive(branch.ID)
	if resendText != "" {
		if resp := a.startRun(branch.ID, resendText, nil); resp.Error != "" {
			return nil, errors.New(resp.Error)
		}
	}
	return branch, nil
}

func visibleMessageCut(messages []history.SavedMessage, visibleOrdinal int, includeSelected bool) int {
	if visibleOrdinal < 0 {
		return -1
	}
	seen := 0
	for i, m := range messages {
		if !isVisibleSavedMessage(m) {
			continue
		}
		if seen == visibleOrdinal {
			if includeSelected {
				return i + 1
			}
			return i
		}
		seen++
	}
	return -1
}

func isVisibleSavedMessage(m history.SavedMessage) bool {
	if m.Role != "user" && m.Role != "assistant" {
		return false
	}
	if m.Role == "assistant" {
		return visibleReply(m.Content) != ""
	}
	return !strings.HasPrefix(m.Content, "TOOL_RESULT")
}

// userMessageIndex returns the slice index of the ordinal-th user message
// (0-based), or -1 if there are fewer user messages than that.
func userMessageIndex(msgs []history.SavedMessage, ordinal int) int {
	seen := 0
	for i, m := range msgs {
		if m.Role != string(domain.RoleUser) {
			continue
		}
		if strings.HasPrefix(m.Content, "TOOL_RESULT") {
			continue // protocol echo, not a real user message
		}
		if seen == ordinal {
			return i
		}
		seen++
	}
	return -1
}

// truncateDisplayItems drops display items from the ordinal-th user message item
// onward, so the saved transcript matches the truncated message history.
func truncateDisplayItems(raw json.RawMessage, ordinal int) json.RawMessage {
	items := parseDisplayItems(raw)
	seen := 0
	cut := len(items)
	for i, it := range items {
		if it.Type != "msg" {
			continue
		}
		if role, _ := it.Data["role"].(string); role != "user" {
			continue
		}
		if seen == ordinal {
			cut = i
			break
		}
		seen++
	}
	out, err := json.Marshal(items[:cut])
	if err != nil {
		return raw
	}
	return json.RawMessage(out)
}

// parseDisplayItems safely parses existing display items from raw JSON.
func parseDisplayItems(raw json.RawMessage) []displayItem {
	if len(raw) == 0 {
		return nil
	}
	var items []displayItem
	_ = json.Unmarshal(raw, &items)
	return items
}
