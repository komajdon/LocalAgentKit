package main

import (
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/permission"
)

func TestRespondPermissionMatchesIDAndNeverBlocks(t *testing.T) {
	a := NewApp()
	pending := &pendingPrompt{id: "perm-2", reply: make(chan permission.Scope, 1)}
	a.pending = pending

	a.RespondPermission("perm-1", "once") // stale dialog
	if len(pending.reply) != 0 {
		t.Fatal("a response for another request was accepted")
	}
	a.RespondPermission("perm-2", "bogus")
	// A double click and a Stop must not deadlock on the full channel.
	a.RespondPermission("perm-2", "workspace")
	a.StopAgent()
	if got := <-pending.reply; got != "" {
		t.Fatalf("unknown decision should deny, got %q", got)
	}
}

func TestGrantStoreRoundTrip(t *testing.T) {
	m, err := permission.NewManager(grantStore{})
	if err != nil {
		t.Fatal(err)
	}
	args := domain.Args{"command": "make"}
	if err := m.Remember(permission.ScopeWorkspace, "/w", "shell", args); err != nil {
		t.Fatal(err)
	}
	again, err := permission.NewManager(grantStore{})
	if err != nil || again.Check("", "/w", "shell", args, false) != permission.Allow {
		t.Fatalf("grant not persisted: %v", err)
	}
	if err := again.RevokeAll(); err != nil {
		t.Fatal(err)
	}
}
