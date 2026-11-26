package store

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"testing"
)

type fakeChain struct {
	m       map[string]string
	setErr  error
	corrupt bool
}

func (f *fakeChain) Get(a string) (string, error) {
	s, ok := f.m[a]
	if !ok {
		return "", errors.New("locked or missing")
	}
	return s, nil
}
func (f *fakeChain) Set(a, s string) error {
	if f.setErr != nil {
		return f.setErr
	}
	if f.corrupt {
		s = "00"
	}
	f.m[a] = s
	return nil
}
func (f *fakeChain) Delete(a string) error { delete(f.m, a); return nil }

// open (re)initialises the package-level store on dir.
func open(t *testing.T, dir string) {
	t.Helper()
	_ = Close()
	once = sync.Once{}
	initErr = nil
	if err := Init(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = Close(); once = sync.Once{}; initErr = nil })
}

func read(t *testing.T) string {
	t.Helper()
	var v string
	if err := Get("b", "k", &v); err != nil {
		t.Fatal(err)
	}
	return v
}

func useChain(t *testing.T, c keychain) {
	t.Helper()
	orig := chain
	chain = c
	t.Cleanup(func() { chain = orig })
}

func TestKeychainMigrationKeepsDataReadable(t *testing.T) {
	fake := &fakeChain{m: map[string]string{}}
	useChain(t, fake)
	dir := t.TempDir()
	open(t, dir) // a pre-existing, file-keyed store
	if err := Put("b", "k", "secret conversation"); err != nil {
		t.Fatal(err)
	}
	key, _ := CurrentKey()

	if err := MoveKeyToKeychain(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, keyFile)); !os.IsNotExist(err) || Location() != KeyInKeychain {
		t.Fatal("key file should be gone")
	}
	open(t, dir)
	if read(t) != "secret conversation" {
		t.Fatal("data lost after migration")
	}

	if err := MoveKeyToFile(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, keyFile))
	if err != nil || !bytes.Equal(got, key) || len(fake.m) != 0 {
		t.Fatalf("move back failed: %v %v", err, fake.m)
	}
}

func TestFailingKeychainKeepsTheKeyFile(t *testing.T) {
	for name, fake := range map[string]*fakeChain{
		"unavailable":     {m: map[string]string{}, setErr: errors.New("no service")},
		"returns garbage": {m: map[string]string{}, corrupt: true},
	} {
		t.Run(name, func(t *testing.T) {
			useChain(t, fake)
			dir := t.TempDir()
			open(t, dir)
			if err := MoveKeyToKeychain(); err == nil {
				t.Fatal("want error")
			}
			if _, err := os.Stat(filepath.Join(dir, keyFile)); err != nil || Location() != KeyInFile {
				t.Fatal("key file must remain")
			}
		})
	}
}

func TestLockedKeychainFailsWithoutCreatingAKey(t *testing.T) {
	fake := &fakeChain{m: map[string]string{}}
	useChain(t, fake)
	dir := t.TempDir()
	open(t, dir)
	if err := MoveKeyToKeychain(); err != nil {
		t.Fatal(err)
	}
	fake.m = map[string]string{}
	if _, err := loadOrCreateKey(dir); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Join(dir, keyFile)); !os.IsNotExist(err) {
		t.Fatal("a replacement key was written")
	}
}

func snapshot(t *testing.T) []byte {
	t.Helper()
	path := filepath.Join(t.TempDir(), "snap.db")
	if err := Snapshot(path); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func TestRestoreIsStagedThenAppliedAndKeepsTheOldData(t *testing.T) {
	source := t.TempDir()
	open(t, source)
	if err := Put("b", "k", "from backup"); err != nil {
		t.Fatal(err)
	}
	backup, key := snapshot(t), mustKey(t)

	target := t.TempDir()
	open(t, target)
	if err := Put("b", "k", "current data"); err != nil {
		t.Fatal(err)
	}
	if err := StageRestore(backup, nil); err == nil {
		t.Fatal("a database from another key must be rejected without that key")
	}
	if err := StageRestore([]byte("not a database"), key); err == nil {
		t.Fatal("garbage must be rejected")
	}
	if err := StageRestore(backup, key); err != nil {
		t.Fatal(err)
	}
	if read(t) != "current data" {
		t.Fatal("staging must not touch the open database")
	}

	open(t, target)
	if read(t) != "from backup" {
		t.Fatal("restore not applied")
	}
	kept, _ := filepath.Glob(filepath.Join(target, asideDir+"*", "*"))
	if len(kept) != 2 {
		t.Fatalf("old database and key should be kept: %v", kept)
	}
}

func TestRestoreWithoutKeyUsesCurrentKey(t *testing.T) {
	dir := t.TempDir()
	open(t, dir)
	_ = Put("b", "k", "old")
	backup := snapshot(t)
	_ = Put("b", "k", "new")
	if err := StageRestore(backup, nil); err != nil {
		t.Fatal(err)
	}
	open(t, dir)
	if read(t) != "old" {
		t.Fatal("restore not applied")
	}
}

func mustKey(t *testing.T) []byte {
	t.Helper()
	k, err := CurrentKey()
	if err != nil {
		t.Fatal(err)
	}
	return k
}
