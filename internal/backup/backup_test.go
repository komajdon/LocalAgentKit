package backup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

var (
	dbData  = bytes.Repeat([]byte("database-bytes "), 100)
	keyData = bytes.Repeat([]byte{0xAB}, 32)
)

func TestPlainArchiveHasNoKey(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := WritePlain(dest, dbData); err != nil {
		t.Fatal(err)
	}
	files, err := Read(dest, "")
	if err != nil || !bytes.Equal(files[Database], dbData) {
		t.Fatal(err)
	}
	if _, ok := files[Key]; ok {
		t.Fatal("plain archive contains a key")
	}
}

func TestEncryptedRoundTripAndRejection(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "b.lakbackup")
	const pass = "correct horse battery"
	if err := WriteEncrypted(dest, "short", dbData, keyData); err == nil {
		t.Fatal("short passphrase accepted")
	}
	if err := WriteEncrypted(dest, pass, dbData, keyData); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(dest)
	if bytes.Contains(raw, keyData) || bytes.Contains(raw, []byte("database-bytes")) {
		t.Fatal("content visible in the encrypted file")
	}
	if info, _ := os.Stat(dest); info.Mode().Perm()&0o077 != 0 && filepath.Separator == '/' {
		t.Fatalf("mode %v", info.Mode())
	}
	files, err := Read(dest, pass)
	if err != nil || !bytes.Equal(files[Key], keyData) || !bytes.Equal(files[Database], dbData) {
		t.Fatal(err)
	}
	if _, err := Read(dest, ""); !errors.Is(err, ErrPassphraseRequired) {
		t.Fatal(err)
	}
	if _, err := Read(dest, "wrong passphrase"); !errors.Is(err, ErrBadPassphrase) {
		t.Fatal(err)
	}
	for _, i := range []int{len(magic) + 1, headerLen - 1, len(raw) - 1} {
		bad := append([]byte(nil), raw...)
		bad[i] ^= 1
		if err := os.WriteFile(dest, bad, 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Read(dest, pass); err == nil {
			t.Fatalf("tampering at byte %d not detected", i)
		}
	}
}

func TestDamagedPlainArchiveIsRejected(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "b.tar.gz")
	if err := WritePlain(dest, dbData); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(dest)
	if err := os.WriteFile(dest, raw[:len(raw)/2], 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(dest, ""); err == nil {
		t.Fatal("truncated archive accepted")
	}
}
