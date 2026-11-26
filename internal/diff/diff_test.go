package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// The output must be a patch that git accepts and that produces the new text.
func TestUnifiedAppliesWithGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not installed")
	}
	long := "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10\nl11\nl12\nl13\nl14\nl15\nl16\nl17\nl18\nl19\nl20\n"
	for name, tc := range map[string]struct{ old, new string }{
		"change":     {"a\nb\nc\n", "a\nB\nc\n"},
		"insert top": {"a\nb\n", "z\na\nb\n"},
		"delete end": {"a\nb\nc\n", "a\n"},
		"two hunks":  {long, strings.Replace(strings.Replace(long, "l2\n", "x\n", 1), "l18\n", "y\n", 1)},
		"near hunks": {long, strings.Replace(strings.Replace(long, "l5\n", "x\n", 1), "l10\n", "y\n", 1)},
		"append":     {long, long + "end\n"},
		"repeated":   {"x\nx\nx\n", "x\ny\nx\nx\n"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			file := filepath.Join(dir, "f")
			if err := os.WriteFile(file, []byte(tc.old), 0o600); err != nil {
				t.Fatal(err)
			}
			patch := Unified("f", tc.old, tc.new)
			cmd := exec.Command("git", "apply", "-p1", "-")
			cmd.Dir, cmd.Stdin = dir, strings.NewReader(patch)
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("%s\n%s", out, patch)
			}
			if got, _ := os.ReadFile(file); string(got) != tc.new {
				t.Fatalf("got %q want %q\n%s", got, tc.new, patch)
			}
		})
	}
}

func TestUnifiedSummaries(t *testing.T) {
	if got := Unified("f", "a\x00", "b"); !strings.Contains(got, "binary") {
		t.Fatal(got)
	}
	big := strings.Repeat("a\n", 3000)
	if got := Unified("f", big, strings.Repeat("b\n", 3000)); !strings.Contains(got, "too large") {
		t.Fatal(got[:80])
	}
	for _, tc := range [][2]string{{"", "a\n"}, {"a\n", ""}} {
		if got := Unified("f", tc[0], tc[1]); strings.Contains(got, "newline") {
			t.Fatal(got)
		}
	}
	if got := Unified("f", "same", "same"); !strings.Contains(got, "no change") {
		t.Fatal(got)
	}
}
