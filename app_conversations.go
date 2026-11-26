package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	appLayer "agent-gui/app"
	"agent-gui/domain"
	"agent-gui/internal/history"
)

// ConversationView is a conversation as shown, including an unfinished run.
type ConversationView struct {
	history.Conversation
	Running bool   `json:"running"`
	Seq     uint64 `json:"seq"` // last run event included in DisplayItems
}

func (a *App) setActive(id string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.activeID = id
}

func (a *App) activeConversation() string {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.activeID
}

func (a *App) ListConversations() ([]history.ConvMeta, error) {
	return history.List()
}

func (a *App) NewConversation(workDir string) (*history.Conversation, error) {
	if workDir == "" {
		workDir, _ = os.UserHomeDir()
	}
	now := time.Now()
	conv := &history.Conversation{
		ConvMeta: history.ConvMeta{
			ID:        fmt.Sprintf("%d", now.UnixMilli()),
			Title:     "New conversation",
			WorkDir:   workDir,
			CreatedAt: now,
			UpdatedAt: now,
		},
		Messages:     []history.SavedMessage{},
		DisplayItems: json.RawMessage("[]"),
	}
	a.convMu.Lock()
	defer a.convMu.Unlock()
	if err := history.Save(conv); err != nil {
		return nil, err
	}
	a.setActive(conv.ID)
	return conv, nil
}

// LoadConversation returns the conversation and makes it the active one.
func (a *App) LoadConversation(id string) (*ConversationView, error) {
	// convMu stops a run from finishing between the load and the snapshot.
	a.convMu.Lock()
	defer a.convMu.Unlock()
	conv, err := history.Load(id)
	if err != nil {
		return nil, err
	}
	view := &ConversationView{Conversation: *conv}
	if r := a.runningIn(id); r != nil {
		items, seq := r.transcript()
		raw, err := json.Marshal(items)
		if err != nil {
			return nil, err
		}
		view.DisplayItems, view.Running, view.Seq = raw, true, seq
	}
	a.setActive(id)
	return view, nil
}

// DeleteConversation removes a conversation, stopping its run first.
func (a *App) DeleteConversation(id string) error {
	a.convMu.Lock()
	defer a.convMu.Unlock()
	if r := a.runningIn(id); r != nil {
		r.mu.Lock()
		r.discard = true
		r.mu.Unlock()
		a.StopAgent()
	}
	a.mu.Lock()
	if a.activeID == id {
		a.activeID = ""
	}
	a.mu.Unlock()
	if err := a.forgetChanges(id); err != nil {
		a.logf("could not remove the change journal: %v", err)
	}
	return history.Delete(id)
}

func (a *App) SearchConversations(query string) ([]history.ConvMeta, error) {
	return history.Search(query)
}

func (a *App) ExportConversation(id, format string) (string, error) {
	conv, err := history.Load(id)
	if err != nil {
		return "", err
	}
	return history.Export(conv, format), nil
}

// SetConversationModel sets the active conversation's model override. A run in
// progress keeps the model it started with.
func (a *App) SetConversationModel(model string) error {
	id := a.activeConversation()
	if id == "" {
		return errors.New("no active conversation")
	}
	return a.updateConversation(id, func(c *history.Conversation) error {
		c.Model, c.UpdatedAt = model, time.Now()
		return nil
	})
}

// UpdateConversationPath changes the active conversation's work directory.
func (a *App) UpdateConversationPath(workDir string) error {
	id := a.activeConversation()
	if id == "" {
		return errors.New("no active conversation")
	}
	if a.runningIn(id) != nil {
		return errors.New("stop the agent before changing its work directory")
	}
	return a.updateConversation(id, func(c *history.Conversation) error {
		c.WorkDir, c.UpdatedAt = workDir, time.Now()
		return nil
	})
}

// GetContextUsage reports the active conversation's context usage.
func (a *App) GetContextUsage() appLayer.ContextUsage {
	id := a.activeConversation()
	if id == "" {
		return appLayer.ContextUsage{}
	}
	if r := a.runningIn(id); r != nil {
		r.mu.Lock()
		defer r.mu.Unlock()
		if r.usage != nil {
			return *r.usage
		}
	}
	conv, err := history.Load(id)
	if err != nil {
		return appLayer.ContextUsage{}
	}
	// An estimate; MCP servers are not started just to count their schemas.
	cfg := a.config()
	// No request to the provider: only the saved override applies here.
	model := modelFor(cfg, conv)
	profile := effective(cfg, model, domain.Capabilities{}, a.GetModelSettings().Overrides[overrideKey(cfg.Provider, model)])
	return a.agentFor(cfg, profile, conv, a.registryFor(a.ctx, cfg, conv.WorkDir, nil)).CurrentUsage()
}

func (a *App) RenameConversation(id, title string) error {
	title = strings.TrimSpace(title)
	if title == "" {
		return errors.New("title cannot be empty")
	}
	return a.updateConversation(id, func(c *history.Conversation) error {
		c.Title, c.UpdatedAt = title, time.Now()
		return nil
	})
}

// SetConversationPinned pins or unpins a conversation in the sidebar.
func (a *App) SetConversationPinned(id string, pinned bool) error {
	return a.updateConversation(id, func(c *history.Conversation) error {
		c.Pinned = pinned
		return nil
	})
}

// SetConversationTags replaces a conversation's tag list (trimmed, de-duplicated).
func (a *App) SetConversationTags(id string, tags []string) error {
	seen := map[string]bool{}
	clean := make([]string, 0, len(tags))
	for _, t := range tags {
		t = strings.TrimSpace(t)
		if t == "" || seen[strings.ToLower(t)] {
			continue
		}
		seen[strings.ToLower(t)] = true
		clean = append(clean, t)
	}
	return a.updateConversation(id, func(c *history.Conversation) error {
		c.Tags = clean
		return nil
	})
}
