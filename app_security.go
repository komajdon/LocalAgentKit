package main

import (
	"context"
	"fmt"
	"time"

	appLayer "agent-gui/app"
	"agent-gui/domain"
	"agent-gui/internal/permission"
	"agent-gui/internal/sandbox"
	"agent-gui/internal/store"
)

const (
	grantBucket       = "permissions"
	grantKey          = "grants"
	permissionTimeout = 5 * time.Minute
)

// grantStore persists workspace approvals apart from the settings, so saving
// settings can never overwrite or restore them.
type grantStore struct{}

func (grantStore) Load() ([]permission.Grant, error) {
	var grants []permission.Grant
	if !store.Has(grantBucket, grantKey) {
		return nil, nil
	}
	err := store.Get(grantBucket, grantKey, &grants)
	return grants, err
}

func (grantStore) Save(grants []permission.Grant) error {
	return store.Put(grantBucket, grantKey, grants)
}

// PermissionPrompt is what the approval dialog shows. Its ID ties a response
// to this request, so a late click cannot approve a different one.
type PermissionPrompt struct {
	ID          string      `json:"id"`
	Tool        string      `json:"tool"`
	Description string      `json:"description"`
	Args        domain.Args `json:"args"`
	Workspace   string      `json:"workspace"`
	// ConversationID is the conversation whose run is asking.
	ConversationID string `json:"conversation_id"`
	permission.Details
}

// pendingPrompt is the request currently waiting for the user.
type pendingPrompt struct {
	id    string
	reply chan permission.Scope // "" means denied
}

// authorize decides one tool call, prompting the user when needed.
func (a *App) authorize(ctx context.Context, conversationID, workspace string, req appLayer.PermissionRequest) bool {
	a.mu.Lock()
	setting := string(a.cfg.ToolPermissions[req.Tool])
	a.mu.Unlock()

	switch a.permissions.Check(setting, workspace, req.Tool, req.Args, req.ExternalContent) {
	case permission.Allow:
		a.emit("chat:tool_auto_allowed", req.Tool)
		return true
	case permission.Deny:
		a.emit("chat:tool_auto_denied", req.Tool)
		return false
	}

	details := permission.Describe(req.Tool, req.Args)
	if req.Tool == "shell" {
		details.Impact += " Current " + sandbox.Probe(sandboxOptions(a.config())).String() + "."
	}

	if req.ExternalContent && details.Sensitive {
		details.Impact += " This conversation contains web, document or MCP content, which can carry injected instructions: check the request is what you intended."
	}

	a.mu.Lock()
	a.promptSeq++
	pending := &pendingPrompt{id: fmt.Sprintf("perm-%d", a.promptSeq), reply: make(chan permission.Scope, 1)}
	a.pending = pending
	a.mu.Unlock()

	a.emit("chat:permission_request", PermissionPrompt{
		ID: pending.id, Tool: req.Tool, Description: req.Description,
		Args: req.Args, Workspace: workspace, Details: details, ConversationID: conversationID,
	})

	var scope permission.Scope
	select {
	case scope = <-pending.reply:
	case <-ctx.Done():
	case <-time.After(permissionTimeout):
	}

	a.mu.Lock()
	if a.pending == pending {
		a.pending = nil
	}
	a.mu.Unlock()
	a.emit("chat:permission_closed", pending.id)

	if scope == "" {
		return false
	}
	if err := a.permissions.Remember(scope, workspace, req.Tool, req.Args); err != nil {
		// The call itself was approved; only remembering it failed.
		a.logf("could not save approval: %v", err)
	}
	return true
}

// replyLocked delivers a decision without ever blocking. a.mu must be held.
func (a *App) replyLocked(scope permission.Scope) {
	if a.pending == nil {
		return
	}
	select {
	case a.pending.reply <- scope:
	default: // already answered
	}
}

// RespondPermission answers the prompt with the given ID. decision is "once",
// "session" or "workspace"; anything else denies.
func (a *App) RespondPermission(id, decision string) {
	scope := permission.Scope(decision)
	switch scope {
	case permission.ScopeOnce, permission.ScopeSession, permission.ScopeWorkspace:
	default:
		scope = ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.pending != nil && a.pending.id == id {
		a.replyLocked(scope)
	}
}

// ListPermissionGrants returns the remembered approvals.
func (a *App) ListPermissionGrants() []permission.Grant { return a.permissions.List() }

// RevokePermissionGrant removes one approval. It returns an error message or "".
func (a *App) RevokePermissionGrant(id string) string { return errString(a.permissions.Revoke(id)) }

// RevokeAllPermissionGrants removes every approval. It returns an error
// message or "".
func (a *App) RevokeAllPermissionGrants() string { return errString(a.permissions.RevokeAll()) }

func errString(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
