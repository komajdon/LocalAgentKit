package store

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestRotateKey(t *testing.T) {
	dir := t.TempDir()
	open(t, dir)
	if err := Put("b", "k", "secret"); err != nil {
		t.Fatal(err)
	}
	before := mustKey(t)
	if err := RotateKey(); err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(before, mustKey(t)) || read(t) != "secret" {
		t.Fatal("rotation failed")
	}
	open(t, dir)
	if read(t) != "secret" {
		t.Fatal("data unreadable after restart")
	}
}

func TestInterruptedRotationRecovers(t *testing.T) {
	dir := t.TempDir()
	open(t, dir)
	if err := Put("b", "k", "secret"); err != nil {
		t.Fatal(err)
	}
	// Stop after the database was rewritten but before the key files swapped.
	beforeKeySwap = func() { panic("power cut") }
	defer func() { beforeKeySwap = func() {} }()
	func() {
		defer func() { _ = recover() }()
		_ = RotateKey()
	}()
	open(t, dir)
	if read(t) != "secret" {
		t.Fatal("new key not adopted")
	}

	// Stop before the database was rewritten: a stray next key is discarded.
	if err := os.WriteFile(filepath.Join(dir, nextKeyFile), bytes.Repeat([]byte{1}, 32), 0o600); err != nil {
		t.Fatal(err)
	}
	open(t, dir)
	if read(t) != "secret" {
		t.Fatal("old key replaced by an unused one")
	}
	if _, err := os.Stat(filepath.Join(dir, nextKeyFile)); !os.IsNotExist(err) {
		t.Fatal("stray key kept")
	}
}
