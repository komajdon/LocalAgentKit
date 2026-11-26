package main

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"agent-gui/internal/history"
	"agent-gui/internal/profile"
	"agent-gui/internal/store"
)

const (
	profileBucket   = "profiles"
	workspaceBucket = "workspaces"
	listKey         = "all"
	maxRecentDirs   = 10
)

func loadList[T any](a *App, bucket string) []T {
	var out []T
	if store.Has(bucket, listKey) {
		if err := store.Get(bucket, listKey, &out); err != nil {
			a.logf("could not read %s: %v", bucket, err)
		}
	}
	return out
}

// ListProfiles returns the built-in and custom profiles.
func (a *App) ListProfiles() []profile.Profile {
	return append(profile.BuiltIns(), loadList[profile.Profile](a, profileBucket)...)
}

// SaveProfile adds or replaces a custom profile. It returns an error message
// or "".
func (a *App) SaveProfile(p profile.Profile) string {
	p.BuiltIn = false
	if p.ID == "" {
		p.ID = fmt.Sprintf("profile-%d", time.Now().UnixNano())
	}
	if err := p.Validate(); err != nil {
		return err.Error()
	}
	a.convMu.Lock()
	defer a.convMu.Unlock()
	list := loadList[profile.Profile](a, profileBucket)
	i := slices.IndexFunc(list, func(x profile.Profile) bool { return x.ID == p.ID })
	if i < 0 {
		list = append(list, p)
	} else {
		list[i] = p
	}
	return errString(store.Put(profileBucket, listKey, list))
}

// DeleteProfile removes a custom profile. Conversations that used it fall back
// to their workspace's or the default profile.
func (a *App) DeleteProfile(id string) string {
	a.convMu.Lock()
	defer a.convMu.Unlock()
	list := slices.DeleteFunc(loadList[profile.Profile](a, profileBucket), func(p profile.Profile) bool { return p.ID == id })
	return errString(store.Put(profileBucket, listKey, list))
}

// ListWorkspaces returns the named workspaces.
func (a *App) ListWorkspaces() []profile.Workspace {
	return append([]profile.Workspace{}, loadList[profile.Workspace](a, workspaceBucket)...)
}

// SaveWorkspace adds or replaces a workspace. It returns an error message or "".
func (a *App) SaveWorkspace(w profile.Workspace) string {
	if w.ID == "" {
		w.ID = fmt.Sprintf("workspace-%d", time.Now().UnixNano())
	}
	if err := w.Validate(); err != nil {
		return err.Error()
	}
	a.convMu.Lock()
	defer a.convMu.Unlock()
	list := loadList[profile.Workspace](a, workspaceBucket)
	i := slices.IndexFunc(list, func(x profile.Workspace) bool { return x.ID == w.ID })
	if i < 0 {
		list = append(list, w)
	} else {
		list[i] = w
	}
	return errString(store.Put(workspaceBucket, listKey, list))
}

// DeleteWorkspace removes a workspace's rules; its files are not touched.
func (a *App) DeleteWorkspace(id string) string {
	a.convMu.Lock()
	defer a.convMu.Unlock()
	list := slices.DeleteFunc(loadList[profile.Workspace](a, workspaceBucket), func(w profile.Workspace) bool { return w.ID == id })
	return errString(store.Put(workspaceBucket, listKey, list))
}

// RecentWorkDirs lists the work directories of the most recent conversations.
func (a *App) RecentWorkDirs() []string {
	out := []string{}
	metas, err := history.List()
	if err != nil {
		return out
	}
	for _, m := range metas {
		if m.WorkDir != "" && !slices.Contains(out, m.WorkDir) && len(out) < maxRecentDirs {
			out = append(out, m.WorkDir)
		}
	}
	return out
}

// SetConversationProfile chooses a conversation's profile; "" follows the
// workspace.
func (a *App) SetConversationProfile(conversationID, profileID string) error {
	if profileID != "" && !slices.ContainsFunc(a.ListProfiles(), func(p profile.Profile) bool { return p.ID == profileID }) {
		return errors.New("no such profile")
	}
	return a.updateConversation(conversationID, func(c *history.Conversation) error {
		c.ProfileID = profileID
		return nil
	})
}

// rules are the profile and workspace that apply to a conversation.
type rules struct {
	profile   profile.Profile
	workspace profile.Workspace
}

func (a *App) rulesFor(conv *history.Conversation) rules {
	r := rules{}
	r.workspace, _ = profile.For(a.ListWorkspaces(), conv.WorkDir)
	id := conv.ProfileID
	if id == "" {
		id = r.workspace.ProfileID
	}
	all := a.ListProfiles()
	r.profile = all[0] // the default
	for _, p := range all {
		if p.ID == id {
			r.profile = p
		}
	}
	return r
}

// permission tightens the global setting for a tool.
func (r rules) permission(tool, global string) string {
	return profile.Stricter(global, r.profile.Permissions[tool], r.workspace.Permissions[tool])
}

// apply narrows a settings snapshot.
func (r rules) apply(cfg configSnapshot, conv *history.Conversation) (configSnapshot, string) {
	p := r.profile
	if extra := strings.TrimSpace(p.Instructions); extra != "" {
		cfg.SystemPrompt = strings.TrimSpace(cfg.SystemPrompt + "\n\n" + extra)
	}
	model := cfg.Model
	if p.Model != "" {
		model = p.Model
	}
	if conv.Model != "" {
		model = conv.Model
	}
	cfg.ContextLimit = profile.Lower(cfg.ContextLimit, p.ContextLimit)
	if p.Strategy != "" {
		cfg.ContextStrategy = p.Strategy
	}
	cfg.MaxRounds = profile.Lower(cfg.MaxRounds, p.MaxRounds)
	return cfg, model
}
