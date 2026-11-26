package diag

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLogRedactsAndRotates(t *testing.T) {
	dir := t.TempDir()
	log, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	log.Info("x", "url", "https://h/?api_key=SECRET123456", "error", errors.New("Bearer SECRET987654321"))
	got := strings.Join(Tail(dir, 10), "\n")
	if strings.Contains(got, "SECRET") || !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("log = %s", got)
	}
	r := &rotating{path: filepath.Join(dir, "r.log"), max: 10}
	for range 3 {
		if _, err := r.Write([]byte("12345678\n")); err != nil {
			t.Fatal(err)
		}
	}
	for _, f := range []string{"r.log", "r.log.1"} {
		if info, err := os.Stat(filepath.Join(dir, f)); err != nil || info.Size() != 9 {
			t.Fatalf("%s: %v %v", f, info, err)
		}
	}
	if len(Tail(t.TempDir(), 5)) != 0 {
		t.Fatal("missing log should be empty")
	}
}
