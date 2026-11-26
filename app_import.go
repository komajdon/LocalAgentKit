package main

// Importing conversations from files.

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
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

// importedChat is one conversation in any of the supported layouts.
type importedChat struct {
	// Simple layout: {title|name, messages: [{role, content}]}.
	Title    string `json:"title"`
	Name     string `json:"name"`
	Messages []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	} `json:"messages"`
	// ChatGPT export: messages form a tree; current_node ends the shown branch.
	Mapping     map[string]chatGPTNode `json:"mapping"`
	CurrentNode string                 `json:"current_node"`
	// Claude export.
	ChatMessages []struct {
		Sender  string `json:"sender"`
		Text    string `json:"text"`
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	} `json:"chat_messages"`
}

type chatGPTNode struct {
	Parent  string `json:"parent"`
	Message *struct {
		Author struct {
			Role string `json:"role"`
		} `json:"author"`
		Content struct {
			Parts []any `json:"parts"`
		} `json:"content"`
	} `json:"message"`
}

// chatGPTBranch walks from the current node to the root, then reverses.
func chatGPTBranch(mapping map[string]chatGPTNode, current string) []history.SavedMessage {
	var out []history.SavedMessage
	seen := map[string]bool{} // a damaged file must not loop forever
	for id := current; id != "" && !seen[id]; id = mapping[id].Parent {
		seen[id] = true
		m := mapping[id].Message
		if m == nil {
			continue
		}
		var parts []string
		for _, p := range m.Content.Parts {
			// Non-text parts, such as images, are objects and are skipped.
			if text, ok := p.(string); ok && text != "" {
				parts = append(parts, text)
			}
		}
		if len(parts) > 0 {
			out = append(out, history.SavedMessage{Role: m.Author.Role, Content: strings.Join(parts, "\n")})
		}
	}
	slices.Reverse(out)
	return out
}

func (c importedChat) messages() (source string, out []history.SavedMessage) {
	switch {
	case len(c.Mapping) > 0:
		return "chatgpt", chatGPTBranch(c.Mapping, c.CurrentNode)
	case len(c.ChatMessages) > 0:
		for _, m := range c.ChatMessages {
			role := m.Sender
			if role == "human" {
				role = "user"
			}
			text := m.Text
			if text == "" {
				var parts []string
				for _, block := range m.Content {
					if block.Type == "text" && block.Text != "" {
						parts = append(parts, block.Text)
					}
				}
				text = strings.Join(parts, "\n")
			}
			if text != "" {
				out = append(out, history.SavedMessage{Role: role, Content: text})
			}
		}
		return "claude", out
	}
	for _, m := range c.Messages {
		out = append(out, history.SavedMessage{Role: m.Role, Content: m.Content})
	}
	if c.Title == "" && c.Name != "" {
		return "claude", out
	}
	return "chatgpt", out
}

func (a *App) importRawItem(raw json.RawMessage) *history.Conversation {
	var chat importedChat
	if err := json.Unmarshal(raw, &chat); err != nil {
		return nil
	}
	source, all := chat.messages()
	// Only what the user and the assistant said is imported. Imported text
	// must never pose as a system prompt or a tool result.
	msgs := []history.SavedMessage{}
	for _, m := range all {
		if m.Role == "assistant" || (m.Role == "user" && !strings.HasPrefix(m.Content, "TOOL_RESULT")) {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return nil
	}

	title := chat.Title
	if title == "" {
		title = chat.Name
	}
	if title == "" {
		title = map[string]string{"chatgpt": "ChatGPT import", "claude": "Claude import"}[source]
	}
	now := time.Now()
	return &history.Conversation{
		ConvMeta:     history.ConvMeta{ID: conversationID(source, 0), Title: title, CreatedAt: now, UpdatedAt: now},
		Messages:     msgs,
		DisplayItems: displayItemsFromMessages(msgs),
	}
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
