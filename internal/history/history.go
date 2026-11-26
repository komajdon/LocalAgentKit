package history

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

	"agent-gui/domain"
	"agent-gui/internal/store"
)

const bucket = "conversations"

type ConvMeta struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	WorkDir   string    `json:"work_dir"`
	Model     string    `json:"model,omitempty"` // per-conversation model override
	Pinned    bool      `json:"pinned,omitempty"`
	Tags      []string  `json:"tags,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type SavedMessage struct {
	Role       string            `json:"role"`
	Content    string            `json:"content"`
	ToolCalls  []domain.ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string            `json:"tool_call_id,omitempty"`
	ToolName   string            `json:"tool_name,omitempty"`
}

// Message converts to the agent's form.
func (m SavedMessage) Message() domain.Message {
	return domain.Message{Role: domain.Role(m.Role), Content: m.Content,
		ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID, ToolName: m.ToolName}
}

// Saved converts an agent message to its stored form.
func Saved(m domain.Message) SavedMessage {
	return SavedMessage{Role: string(m.Role), Content: m.Content,
		ToolCalls: m.ToolCalls, ToolCallID: m.ToolCallID, ToolName: m.ToolName}
}

// TokenUsage stores cumulative real token counts for a conversation.
type TokenUsage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
}

type Conversation struct {
	ConvMeta
	Messages     []SavedMessage  `json:"messages"`
	DisplayItems json.RawMessage `json:"display_items,omitempty"`
	TokenUsage   TokenUsage      `json:"token_usage,omitempty"`
}

func sortNewestFirst(metas []ConvMeta) {
	sort.Slice(metas, func(i, j int) bool {
		if !metas[i].UpdatedAt.Equal(metas[j].UpdatedAt) {
			return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
		}
		return metas[i].ID > metas[j].ID
	})
}

// List returns metadata for all conversations, newest first, from the index.
func List() ([]ConvMeta, error) {
	entries, err := index()
	if err != nil {
		return nil, err
	}
	var metas []ConvMeta
	for _, e := range entries {
		metas = append(metas, e.ConvMeta)
	}
	sortNewestFirst(metas)
	return metas, nil
}

// Search returns conversations whose title, tags or message content contain
// query, ignoring case. Conversations are decrypted only when the title and
// tags do not already match.
func Search(query string) ([]ConvMeta, error) {
	if query == "" {
		return List()
	}
	q := strings.ToLower(query)
	entries, err := index()
	if err != nil {
		return nil, err
	}
	var results []ConvMeta
	for key, e := range entries {
		if strings.Contains(strings.ToLower(e.Title+"\x00"+strings.Join(e.Tags, "\x00")), q) {
			results = append(results, e.ConvMeta)
			continue
		}
		c, err := Load(key)
		if err != nil {
			continue
		}
		for _, m := range c.Messages {
			if strings.Contains(strings.ToLower(m.Content), q) {
				results = append(results, e.ConvMeta)
				break
			}
		}
	}
	sortNewestFirst(results)
	return results, nil
}

// Load retrieves a single conversation by ID.
func Load(id string) (*Conversation, error) {
	var c Conversation
	if err := store.Get(bucket, id, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// Save persists a conversation to the encrypted store.
func Save(c *Conversation) error {
	return store.PutIndexed(bucket, indexBucket, c.ID, c, func(hash string) any { return entryFor(c, hash) })
}

// Delete removes a conversation by ID.
func Delete(id string) error {
	return store.DeleteIndexed(bucket, indexBucket, id)
}

// Export returns the conversation as Markdown, plain text, or JSON.
func Export(c *Conversation, format string) string {
	if format == "json" {
		b, _ := json.Marshal(c)
		return string(b)
	}
	var sb strings.Builder
	if format == "markdown" {
		sb.WriteString("# " + c.Title + "\n\n")
		sb.WriteString("**Working directory:** `" + c.WorkDir + "`  \n")
		sb.WriteString("**Created:** " + c.CreatedAt.Format("2006-01-02 15:04") + "\n\n---\n\n")
		for _, m := range c.Messages {
			switch m.Role {
			case "user":
				sb.WriteString("**User:**\n\n" + m.Content + "\n\n")
			case "assistant":
				sb.WriteString("**Assistant:**\n\n" + m.Content + "\n\n")
			}
		}
	} else {
		sb.WriteString(c.Title + "\n")
		sb.WriteString(strings.Repeat("=", len(c.Title)) + "\n\n")
		for _, m := range c.Messages {
			switch m.Role {
			case "user":
				sb.WriteString("[User]\n" + m.Content + "\n\n")
			case "assistant":
				sb.WriteString("[Assistant]\n" + m.Content + "\n\n")
			}
		}
	}
	return sb.String()
}
