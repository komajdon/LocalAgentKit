package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agent-gui/internal/store"
)

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "lak-history-*")
	if err != nil {
		panic(err)
	}
	if err := store.Init(dir); err != nil {
		panic(err)
	}
	code := m.Run()
	_ = store.Close()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

func clear(tb testing.TB) {
	tb.Helper()
	for _, b := range []string{bucket, indexBucket, schemaBucket} {
		keys, err := store.Hashes(b)
		if err != nil {
			tb.Fatal(err)
		}
		for k := range keys {
			if err := store.Delete(b, k); err != nil {
				tb.Fatal(err)
			}
		}
	}
}

func conv(id, title string, size int) *Conversation {
	return &Conversation{
		ConvMeta:   ConvMeta{ID: id, Title: title, UpdatedAt: time.Unix(int64(len(id)), 0)},
		Messages:   []SavedMessage{{Role: "user", Content: strings.Repeat("word ", size/5) + "needle-" + id}},
		TokenUsage: TokenUsage{PromptTokens: 3, CompletionTokens: 2},
	}
}

// legacy writes a conversation the way version 1 did, without an index.
func legacy(tb testing.TB, c *Conversation) {
	tb.Helper()
	if err := store.Put(bucket, c.ID, c); err != nil {
		tb.Fatal(err)
	}
}

func titles(t *testing.T) string {
	t.Helper()
	list, err := List()
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, m := range list {
		out = append(out, m.Title)
	}
	return strings.Join(out, ",")
}

func TestMigrationBacksUpThenIndexes(t *testing.T) {
	clear(t)
	legacy(t, conv("a", "A", 50))
	legacy(t, conv("bb", "B", 50))
	backups := t.TempDir()
	if err := Migrate(backups); err != nil {
		t.Fatal(err)
	}
	if got := titles(t); got != "B,A" {
		t.Fatalf("titles = %q", got)
	}
	made, _ := filepath.Glob(filepath.Join(backups, "pre-migration-v1-*.db"))
	if len(made) != 1 {
		t.Fatalf("pre-migration backups = %v", made)
	}
	// A second start neither backs up again nor changes anything.
	if err := Migrate(backups); err != nil {
		t.Fatal(err)
	}
	if again, _ := filepath.Glob(filepath.Join(backups, "*")); len(again) != 1 {
		t.Fatalf("backups = %v", again)
	}
}

func TestIndexRepairsChangesMadeWithoutIt(t *testing.T) {
	clear(t)
	if err := Save(conv("a", "A", 50)); err != nil {
		t.Fatal(err)
	}
	if err := Save(conv("bb", "B", 50)); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	// An older version renames one, deletes one and adds one.
	legacy(t, conv("a", "A renamed", 50))
	if err := store.Delete(bucket, "bb"); err != nil {
		t.Fatal(err)
	}
	legacy(t, conv("ccc", "C", 50))
	if err := Migrate(t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := titles(t); got != "C,A renamed" {
		t.Fatalf("titles = %q", got)
	}
}

func TestNewerSchemaIsRefused(t *testing.T) {
	clear(t)
	if err := store.Put(schemaBucket, schemaKey, SchemaVersion+1); err != nil {
		t.Fatal(err)
	}
	if err := Migrate(t.TempDir()); err == nil {
		t.Fatal("newer data accepted")
	}
	clear(t)
}

func TestSaveDeleteAndSearch(t *testing.T) {
	clear(t)
	c := conv("a", "Alpha", 50)
	c.Tags = []string{"Work"}
	if err := Save(c); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{"alph", "work", "NEEDLE-a"} {
		if got, err := Search(q); err != nil || len(got) != 1 {
			t.Errorf("Search(%q) = %v %v", q, got, err)
		}
	}
	if got, _ := Search("absent"); len(got) != 0 {
		t.Errorf("unexpected %v", got)
	}
	if err := Delete("a"); err != nil {
		t.Fatal(err)
	}
	if titles(t) != "" {
		t.Fatal("index entry survived delete")
	}
}

// listFull is how listing worked before the index.
func listFull() (n int) {
	_ = store.Scan(bucket, func(_ string, raw []byte) error {
		var c Conversation
		if json.Unmarshal(raw, &c) == nil {
			n++
		}
		return nil
	})
	return n
}

func BenchmarkList(b *testing.B) {
	clear(b)
	for i := range 500 {
		if err := Save(conv(fmt.Sprint("c", i), "title", 40_000)); err != nil {
			b.Fatal(err)
		}
	}
	b.Run("index", func(b *testing.B) {
		for b.Loop() {
			if list, err := List(); err != nil || len(list) != 500 {
				b.Fatal(len(list), err)
			}
		}
	})
	b.Run("full-conversations", func(b *testing.B) {
		for b.Loop() {
			if listFull() != 500 {
				b.Fatal("count")
			}
		}
	})
	clear(b)
}
