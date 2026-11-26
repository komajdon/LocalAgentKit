package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"agent-gui/internal/config"
)

func TestAgentWritesAreReviewableAndUndoable(t *testing.T) {
	p := &scripted{reply: `TOOL_CALL: {"tool":"write_file","args":{"path":"a.txt","content":"new\n"}}`}
	a, _ := testApp(t, p)
	a.cfg.ToolPermissions = map[string]config.ToolPerm{"write_file": config.PermAllow}
	id := newConv(t, a)
	dir := stored(t, id).WorkDir
	file := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(file, []byte("old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	wait(t, send(t, a, id, "write").done)

	list, err := a.ListFileChanges(id)
	if err != nil || len(list) != 1 || !list[0].Undoable || !strings.Contains(list[0].Diff, "+new") {
		t.Fatalf("changes = %+v %v", list, err)
	}
	if msg := a.UndoFileChange(list[0].ID); msg != "" {
		t.Fatal(msg)
	}
	if data, _ := os.ReadFile(file); string(data) != "old\n" {
		t.Fatalf("after undo: %q", data)
	}
	if a.UndoFileChange(list[0].ID) == "" {
		t.Fatal("a second undo was accepted")
	}
	if err := a.DeleteConversation(id); err != nil {
		t.Fatal(err)
	}
	if left, _ := a.ListFileChanges(id); len(left) != 0 {
		t.Fatal("journal outlived the conversation")
	}
}

func TestGitStatus(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	a, _ := testApp(t, &scripted{})
	id := newConv(t, a)
	if s := a.GetGitStatus(id); s.Repository || s.Error != "" {
		t.Fatalf("plain directory: %+v", s)
	}
	dir := stored(t, id).WorkDir
	if out, err := exec.Command("git", "-C", dir, "init", "-q").CombinedOutput(); err != nil {
		t.Fatal(string(out))
	}
	if err := os.WriteFile(filepath.Join(dir, "f"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	s := a.GetGitStatus(id)
	if !s.Repository || !strings.Contains(strings.Join(s.Lines, "\n"), "?? f") {
		t.Fatalf("status = %+v", s)
	}
}
