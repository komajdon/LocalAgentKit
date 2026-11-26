package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestKeyIsCreatedOnlyForAFreshDirectory(t *testing.T) {
	dir := t.TempDir()
	key, err := loadOrCreateKey(dir)
	if err != nil || len(key) != 32 {
		t.Fatalf("create: %v", err)
	}
	again, err := loadOrCreateKey(dir)
	if err != nil || !bytes.Equal(key, again) {
		t.Fatalf("reload: %v", err)
	}
}

func TestDamagedOrMissingKeyIsNeverReplaced(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "agent.key")
	if err := os.WriteFile(keyPath, []byte("truncated"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateKey(dir); err == nil {
		t.Fatal("a damaged key must be an error")
	}
	if got, _ := os.ReadFile(keyPath); string(got) != "truncated" {
		t.Fatalf("damaged key was overwritten: %q", got)
	}

	// Data without a key must not get a fresh key that cannot read it.
	if err := os.Remove(keyPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "agent.db"), []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := loadOrCreateKey(dir); err == nil {
		t.Fatal("missing key with existing data must be an error")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Fatal("a new key was written")
	}
}

func TestUninitializedStoreReturnsErrors(t *testing.T) {
	var v string
	if Put("b", "k", "v") == nil || Get("b", "k", &v) == nil || Delete("b", "k") == nil || Has("b", "k") {
		t.Fatal("operations on a closed store must fail without panicking")
	}
	if Scan("b", func(string, []byte) error { return nil }) == nil {
		t.Fatal("Scan must fail")
	}
}
