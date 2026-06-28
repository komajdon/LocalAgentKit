package history

import (
	"encoding/json"
	"sort"
	"strings"
	"time"

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
	Role    string `json:"role"`
	Content string `json:"content"`
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

// List returns metadata for all conversations, sorted newest-first.
func List() ([]ConvMeta, error) {
	var metas []ConvMeta
	err := store.Scan(bucket, func(_ string, raw []byte) error {
		var c Conversation
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil
		}
		metas = append(metas, c.ConvMeta)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(metas, func(i, j int) bool {
		return metas[i].UpdatedAt.After(metas[j].UpdatedAt)
	})
	return metas, nil
}

// Search returns conversations whose title or message content contains query (case-insensitive).
func Search(query string) ([]ConvMeta, error) {
	if query == "" {
		return List()
	}
	q := strings.ToLower(query)
	var results []ConvMeta

	err := store.Scan(bucket, func(_ string, raw []byte) error {
		var c Conversation
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil
		}
		if strings.Contains(strings.ToLower(c.Title), q) {
			results = append(results, c.ConvMeta)
			return nil
		}
		for _, m := range c.Messages {
			if strings.Contains(strings.ToLower(m.Content), q) {
				results = append(results, c.ConvMeta)
				return nil
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(results, func(i, j int) bool {
		return results[i].UpdatedAt.After(results[j].UpdatedAt)
	})
	return results, nil
}

// TotalUsage sums cumulative token usage across every conversation — the basis
// for the shared account-wide usage budget.
func TotalUsage() (promptTokens, completionTokens int64) {
	_ = store.Scan(bucket, func(_ string, raw []byte) error {
		var c Conversation
		if err := json.Unmarshal(raw, &c); err != nil {
			return nil
		}
		promptTokens += int64(c.TokenUsage.PromptTokens)
		completionTokens += int64(c.TokenUsage.CompletionTokens)
		return nil
	})
	return promptTokens, completionTokens
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
	return store.Put(bucket, c.ID, c)
}

// Delete removes a conversation by ID.
func Delete(id string) error {
	return store.Delete(bucket, id)
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
