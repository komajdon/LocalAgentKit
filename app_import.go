package main

// Importing conversations from files.

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-gui/internal/history"
)

// ImportConversation opens a file dialog and imports conversations from:
//   - This app's own JSON format (a single Conversation JSON)
//   - ChatGPT conversations.json  (array of {title, messages: [{role, content}]})
//   - Claude claude_conversations.json (array of {name, messages: [{role, content}]})
//
// Returns the list of newly imported ConvMeta so the frontend can show a preview.
func (a *App) ImportConversation() string {
	src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title: "Import Conversation",
		Filters: []runtime.FileFilter{
			{DisplayName: "JSON files (*.json)", Pattern: "*.json"},
		},
	})
	if err != nil || src == "" {
		return ""
	}

	data, err := os.ReadFile(src)
	if err != nil {
		return "Cannot read file: " + err.Error()
	}

	imported := a.importFromJSON(data)
	if imported == 0 {
		return "No valid conversations found in the file."
	}
	return fmt.Sprintf("Imported %d conversation(s).", imported)
}

func (a *App) importFromJSON(data []byte) int {
	count := 0

	// Try own format first (single Conversation object).
	var conv history.Conversation
	if err := json.Unmarshal(data, &conv); err == nil && conv.ID != "" {
		conv.ID = conversationID("import", 0)
		conv.CreatedAt = time.Now()
		conv.UpdatedAt = time.Now()
		if conv.Title == "" {
			conv.Title = "Imported conversation"
		}
		if len(conv.DisplayItems) == 0 {
			conv.DisplayItems = displayItemsFromMessages(conv.Messages)
		}
		if err := history.Save(&conv); err == nil {
			count++
		}
		return count
	}

	// Try array — might be ChatGPT or Claude format.
	var raw []json.RawMessage
	if err := json.Unmarshal(data, &raw); err != nil {
		return count
	}

	for i, item := range raw {
		if c := a.importRawItem(item); c != nil {
			c.ID = conversationID("import", i)
			if err := history.Save(c); err == nil {
				count++
			}
		}
	}
	return count
}

func (a *App) importRawItem(raw json.RawMessage) *history.Conversation {
	// Both supported shapes carry a messages list; they differ only in whether
	// the conversation is named "title" (ChatGPT style) or "name" (Claude style).
	var chat struct {
		Title    string `json:"title"`
		Name     string `json:"name"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(raw, &chat); err != nil || len(chat.Messages) == 0 {
		return nil
	}

	prefix, title, fallback := "chatgpt", chat.Title, "ChatGPT import"
	if title == "" && chat.Name != "" {
		prefix, title, fallback = "claude", chat.Name, "Claude import"
	}
	if title == "" {
		title = fallback
	}

	now := time.Now()
	conv := &history.Conversation{
		ConvMeta: history.ConvMeta{
			ID:        conversationID(prefix, 0),
			Title:     title,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages: make([]history.SavedMessage, 0, len(chat.Messages)),
	}
	for _, m := range chat.Messages {
		if m.Role == "user" || m.Role == "assistant" {
			conv.Messages = append(conv.Messages, history.SavedMessage{Role: m.Role, Content: m.Content})
		}
	}
	conv.DisplayItems = displayItemsFromMessages(conv.Messages)
	return conv
}

func conversationID(prefix string, offset int) string {
	return fmt.Sprintf("%s-%d-%d", prefix, time.Now().UnixNano(), offset)
}

func displayItemsFromMessages(messages []history.SavedMessage) json.RawMessage {
	items := make([]displayItem, 0, len(messages))
	for _, m := range messages {
		if (m.Role != "user" && m.Role != "assistant") || strings.HasPrefix(m.Content, "TOOL_RESULT") {
			continue
		}
		items = append(items, displayItem{
			Type: "msg",
			Data: map[string]any{"role": m.Role, "text": m.Content, "streaming": false},
		})
	}
	raw, _ := json.Marshal(items)
	return json.RawMessage(raw)
}
