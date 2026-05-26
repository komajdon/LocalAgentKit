package domain

// Role represents a conversation participant.
type Role string

const (
	RoleSystem    Role = "system"
	RoleUser      Role = "user"
	RoleAssistant Role = "assistant"
)

// Message is a single turn in a conversation.
type Message struct {
	Role    Role   `json:"role"`
	Content string `json:"content"`
}

// Conversation holds the full history of an exchange.
type Conversation struct {
	SystemPrompt string
	Messages     []Message
}

func (c *Conversation) Add(m Message) {
	c.Messages = append(c.Messages, m)
}

func (c *Conversation) Reset() {
	c.Messages = nil
}

// History returns only the user/assistant messages (no system prompt).
func (c *Conversation) History() []Message {
	out := make([]Message, len(c.Messages))
	copy(out, c.Messages)
	return out
}

// SetHistory replaces the conversation messages.
func (c *Conversation) SetHistory(msgs []Message) {
	c.Messages = msgs
}

// Full returns system prompt followed by all messages.
func (c *Conversation) Full() []Message {
	out := make([]Message, 0, 1+len(c.Messages))
	if c.SystemPrompt != "" {
		out = append(out, Message{Role: RoleSystem, Content: c.SystemPrompt})
	}
	return append(out, c.Messages...)
}
