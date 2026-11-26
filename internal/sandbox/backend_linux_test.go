//go:build linux

package sandbox

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCredentialsMaskedWhenHomeIsWorkDir(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if err := os.Mkdir(filepath.Join(home, ".ssh"), 0o700); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{".ssh/id_ed25519", ".netrc", "notes.txt"} {
		if err := os.WriteFile(filepath.Join(home, p), []byte("SECRET-"+p), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	o := Options{Mode: ModeStrict, WorkDir: home}
	if s := Probe(o); !s.Isolated {
		t.Skip("no isolation backend:", s.Detail)
	}
	out, _ := output(t, o, "cat .ssh/id_ed25519 .netrc notes.txt")
	if strings.Contains(out, "SECRET-.ssh") || strings.Contains(out, "SECRET-.netrc") {
		t.Fatalf("credentials visible:\n%s", out)
	}
	if !strings.Contains(out, "SECRET-notes.txt") {
		t.Fatalf("ordinary file should be readable:\n%s", out)
	}
}

func TestHomeIsWritableWhenOutsideWorkDir(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	o := Options{Mode: ModeStrict, WorkDir: t.TempDir()}
	if s := Probe(o); !s.Isolated {
		t.Skip("no isolation backend:", s.Detail)
	}
	if out, err := output(t, o, `echo x > "$HOME/cache" && echo $HOME`); err != nil || strings.TrimSpace(out) != "/tmp" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
