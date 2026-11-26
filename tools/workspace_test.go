package tools

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/sandbox"
)

// layout builds work/, outside/secret.txt and work/appdata/key.
func layout(t *testing.T) (work, outside, appData string) {
	t.Helper()
	base := t.TempDir()
	work, outside = filepath.Join(base, "work"), filepath.Join(base, "outside")
	appData = filepath.Join(work, "appdata")
	for _, d := range []string{work, outside, appData} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for path, body := range map[string]string{
		filepath.Join(outside, "secret.txt"): "SECRET-OUTSIDE",
		filepath.Join(appData, "key"):        "SECRET-KEY",
		filepath.Join(work, "ok.txt"):        "hello",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return work, outside, appData
}

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skip("symlinks need extra privileges on Windows:", err)
		}
		t.Fatal(err)
	}
}

func run(tool domain.Tool, kv ...string) string {
	args := map[string]string{}
	for i := 0; i < len(kv); i += 2 {
		args[kv[i]] = kv[i+1]
	}
	return tool.Execute(args)
}

func refused(t *testing.T, name, out string) {
	t.Helper()
	if !strings.HasPrefix(out, "ERROR") || strings.Contains(out, "SECRET") {
		t.Errorf("%s was not refused: %q", name, out)
	}
}

func TestSymlinkEscapesAreRefused(t *testing.T) {
	work, outside, appData := layout(t)
	symlink(t, outside, filepath.Join(work, "dirlink"))
	symlink(t, filepath.Join("..", "outside", "secret.txt"), filepath.Join(work, "filelink"))
	symlink(t, filepath.Join("..", "outside", "new.txt"), filepath.Join(work, "dangling"))

	read := NewReadFile(work, appData)
	write := NewWriteFile(work, appData)
	refused(t, "read via dir link", run(read, "path", "dirlink/secret.txt"))
	refused(t, "read via file link", run(read, "path", "filelink"))
	refused(t, "write via dir link", run(write, "path", "dirlink/new.txt", "content", "x"))
	refused(t, "write via nested new dir", run(write, "path", "dirlink/a/b.txt", "content", "x"))
	refused(t, "write via dangling link", run(write, "path", "dangling", "content", "x"))
	refused(t, "list via dir link", run(NewListDir(work), "path", "dirlink"))
	refused(t, "delete via dir link", run(NewDeleteFile(work), "path", "dirlink/secret.txt"))
	refused(t, "shell dir link", run(NewShell(work, sandbox.Options{}), "command", "cat secret.txt", "dir", "dirlink"))
	refused(t, "shell dir outside", run(NewShell(work, sandbox.Options{}), "command", "cat secret.txt", "dir", outside))
	refused(t, "document via link", run(NewReadDocument(work), "path", "dirlink/x.pdf"))

	if out := run(NewSearchFiles(work, appData), "pattern", "secret"); out != "No matches found." {
		t.Errorf("search leaked: %q", out)
	}
	if got, err := os.ReadDir(outside); err != nil || len(got) != 1 {
		t.Errorf("outside directory changed: %v %v", got, err)
	}

	// Deleting a link removes the link, not what it points to.
	if out := run(NewDeleteFile(work), "path", "filelink"); !strings.HasPrefix(out, "Deleted") {
		t.Errorf("delete link: %q", out)
	}
	if _, err := os.Stat(filepath.Join(outside, "secret.txt")); err != nil {
		t.Error("link target was deleted")
	}
}

func TestProtectedDirectory(t *testing.T) {
	work, _, appData := layout(t)
	symlink(t, "appdata", filepath.Join(work, "alias"))
	for _, p := range []string{"appdata/key", "alias/key", filepath.Join(appData, "key")} {
		refused(t, "read "+p, run(NewReadFile(work, appData), "path", p))
		refused(t, "write "+p, run(NewWriteFile(work, appData), "path", p, "content", "x"))
		refused(t, "delete "+p, run(NewDeleteFile(work, appData), "path", p))
	}
	// A workspace placed inside the protected directory is refused too.
	refused(t, "workspace inside", run(NewReadFile(appData, appData), "path", "key"))
}

func TestWorkspaceNormalOperations(t *testing.T) {
	work, _, appData := layout(t)
	if out := run(NewReadFile(work, appData), "path", "ok.txt"); out != "hello" {
		t.Fatalf("read: %q", out)
	}
	if out := run(NewWriteFile(work, appData), "path", "new/deep/f.txt", "content", "needle"); !strings.HasPrefix(out, "Written") {
		t.Fatalf("write: %q", out)
	}
	if out := run(NewSearchFiles(work, appData), "pattern", "needle"); !strings.Contains(out, "f.txt:1") {
		t.Fatalf("search: %q", out)
	}
	if out := run(NewListDir(work, appData), "path", ""); !strings.Contains(out, "ok.txt") {
		t.Fatalf("list: %q", out)
	}
	refused(t, "delete root", run(NewDeleteFile(work), "path", "."))
	refused(t, "empty work dir", run(NewReadFile(""), "path", "workspace_test.go"))
	refused(t, "directory as file", run(NewReadFile(work), "path", "new"))
}

func FuzzWorkspaceRelStaysInside(f *testing.F) {
	for _, s := range []string{"a", "../a", "/etc/passwd", "a/../../b", "..", "./..", "a//b/./c"} {
		f.Add(s)
	}
	root := filepath.Join(os.TempDir(), "fuzz-root")
	f.Fuzz(func(t *testing.T, path string) {
		rel, err := (workspace{dir: root}).rel(path)
		if err == nil && !filepath.IsLocal(rel) {
			t.Fatalf("rel(%q) = %q escapes", path, rel)
		}
	})
}
