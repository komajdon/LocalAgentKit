package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-gui/internal/config"
	"agent-gui/internal/profile"
)

func results(t *testing.T, id string) string {
	t.Helper()
	var all []string
	for _, m := range stored(t, id).Messages {
		all = append(all, m.Content)
	}
	return strings.Join(all, "\n")
}

func TestProfilesAndWorkspacesOnlyTighten(t *testing.T) {
	p := &scripted{}
	a, _ := testApp(t, p)
	a.cfg.ToolPermissions = map[string]config.ToolPerm{"write_file": config.PermAllow, "read_file": config.PermAllow, "shell": config.PermDeny}
	id := newConv(t, a)
	dir := stored(t, id).WorkDir
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte("SECRET=1"), 0o600); err != nil {
		t.Fatal(err)
	}
	write := `TOOL_CALL: {"tool":"write_file","args":{"path":"out","content":"x"}}`

	// Read-only profile: the write tool does not exist, whatever is allowed.
	if err := a.SetConversationProfile(id, "readonly"); err != nil {
		t.Fatal(err)
	}
	p.reply = write
	wait(t, send(t, a, id, "q1").done)
	if !strings.Contains(results(t, id), "unknown tool 'write_file'") {
		t.Fatal(results(t, id))
	}

	// A workspace denies writing and hides .env, even with "allow" globally.
	if msg := a.SaveWorkspace(profile.Workspace{Name: "w", Path: dir, Ignore: []string{".env"},
		Permissions: map[string]string{"write_file": "deny"}}); msg != "" {
		t.Fatal(msg)
	}
	if err := a.SetConversationProfile(id, ""); err != nil {
		t.Fatal(err)
	}
	p.reply = write + "\n" + `TOOL_CALL: {"tool":"read_file","args":{"path":".env"}}`
	wait(t, send(t, a, id, "q2").done)
	got := results(t, id)
	if !strings.Contains(got, "PERMISSION DENIED") || strings.Contains(got, "SECRET=1") || !strings.Contains(got, "ignore patterns") {
		t.Fatal(got)
	}
	if _, err := os.Stat(filepath.Join(dir, "out")); err == nil {
		t.Fatal("the denied write happened")
	}

	for _, w := range a.ListWorkspaces() {
		a.DeleteWorkspace(w.ID)
	}
	if a.SaveProfile(profile.Profile{Name: "loose", Permissions: map[string]string{"shell": "allow"}}) == "" {
		t.Fatal("a profile loosened a permission")
	}
	if err := a.SetConversationProfile(id, "nope"); err == nil {
		t.Fatal("unknown profile accepted")
	}
	if recent := a.RecentWorkDirs(); len(recent) == 0 {
		t.Fatal("no recent directories")
	}
}
