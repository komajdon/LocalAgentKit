package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agent-gui/domain"
)

type journal []Change

func (j *journal) add(c Change) { *j = append(*j, c) }

func plan(t *testing.T, tool domain.Tool, args domain.Args) domain.Plan {
	t.Helper()
	p, err := tool.(domain.Planner).Plan(context.Background(), args)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func body(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "<missing>"
	}
	return string(data)
}

func TestPlanApplyUndo(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f.txt")
	if err := os.WriteFile(file, []byte("one\ntwo\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	var j journal
	write := &writeFileTool{ws: workspace{dir: dir}, journal: j.add}
	del := &deleteFileTool{ws: workspace{dir: dir}, journal: j.add}

	p := plan(t, write, domain.Args{"path": "f.txt", "content": "one\n2\n"})
	if !strings.Contains(p.Preview, "-two") || !strings.Contains(p.Preview, "+2") || body(file) != "one\ntwo\n" {
		t.Fatalf("preview must not write: %q", p.Preview)
	}
	if out := p.Apply(context.Background()); !strings.HasPrefix(out, "Written") {
		t.Fatal(out)
	}
	plan(t, write, domain.Args{"path": "new/g.txt", "content": "g"}).Apply(context.Background())
	plan(t, del, domain.Args{"path": "f.txt"}).Apply(context.Background())
	if len(j) != 3 || j[0].Kind != "modified" || j[1].Kind != "created" || j[2].Kind != "deleted" {
		t.Fatalf("journal = %+v", j)
	}

	for i := len(j) - 1; i >= 0; i-- {
		if err := Undo(j[i]); err != nil {
			t.Fatal(err)
		}
	}
	if body(file) != "one\ntwo\n" || body(filepath.Join(dir, "new/g.txt")) != "<missing>" {
		t.Fatalf("after undo: %q", body(file))
	}
	if info, _ := os.Stat(file); info.Mode().Perm() != 0o600 {
		t.Errorf("mode = %v", info.Mode())
	}
}

func TestConflictsNeverOverwriteUserChanges(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "f")
	user := func(s string) {
		t.Helper()
		if err := os.WriteFile(file, []byte(s), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	user("base")
	var j journal
	write := &writeFileTool{ws: workspace{dir: dir}, journal: j.add}

	p := plan(t, write, domain.Args{"path": "f", "content": "agent"})
	user("user edited after the preview")
	if out := p.Apply(context.Background()); !strings.Contains(out, "conflict") || body(file) != "user edited after the preview" || len(j) != 0 {
		t.Fatalf("out=%q body=%q", out, body(file))
	}

	plan(t, write, domain.Args{"path": "f", "content": "agent"}).Apply(context.Background())
	user("user edited after the agent")
	if err := Undo(j[0]); err == nil || body(file) != "user edited after the agent" {
		t.Fatalf("undo overwrote the user: %v %q", err, body(file))
	}
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}
	if err := Undo(j[0]); err == nil || body(file) != "<missing>" {
		t.Fatal("undo recreated a file the user removed")
	}
}

func TestPlanRespectsWorkspaceBoundary(t *testing.T) {
	work, outside, appData := layout(t)
	symlink(t, outside, filepath.Join(work, "link"))
	write := NewWriteFile(work, appData).(domain.Planner)
	for _, path := range []string{"../outside/x", "link/x", "appdata/key"} {
		if p, err := write.Plan(context.Background(), domain.Args{"path": path, "content": "x"}); err == nil {
			t.Errorf("%s planned: %s", path, p.Preview)
		}
	}
	if err := Undo(Change{Dir: work, Path: filepath.Join(outside, "secret.txt"), Undoable: true, Kind: "modified"}); err == nil {
		t.Error("undo escaped the work directory")
	}
}
