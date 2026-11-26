package store

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/zalando/go-keyring"
)

// The key lives either in keyFile (the default, which keeps the data folder
// portable) or in the OS keychain, in which case markerFile names the entry.
// If both exist the file wins: it is only removed after the keychain copy has
// been read back.
const (
	keyFile    = "agent.key"
	markerFile = "agent.keyring"
	dbFile     = "agent.db"

	keyringService = "LocalAgentKit"
)

// KeyLocation says where the encryption key is kept.
type KeyLocation string

const (
	KeyInFile     KeyLocation = "file"
	KeyInKeychain KeyLocation = "keychain"
)

// keychain is the part of the OS keychain used here.
type keychain interface {
	Get(account string) (string, error)
	Set(account, secret string) error
	Delete(account string) error
}

type osKeychain struct{}

func (osKeychain) Get(a string) (string, error) { return keyring.Get(keyringService, a) }
func (osKeychain) Set(a, s string) error        { return keyring.Set(keyringService, a, s) }
func (osKeychain) Delete(a string) error        { return keyring.Delete(keyringService, a) }

// chain is replaced by tests.
var chain keychain = osKeychain{}

func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if errors.Is(err, fs.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// loadOrCreateKey returns the 32-byte key for dir. A new key is generated only
// when there is neither a key nor a database: an existing key is never
// replaced, because whatever it encrypted would be lost for good.
func loadOrCreateKey(dir string) ([]byte, error) {
	path := filepath.Join(dir, keyFile)
	data, err := os.ReadFile(path)
	switch {
	case err == nil && len(data) == 32:
		return data, nil
	case err == nil:
		return nil, fmt.Errorf("%s is damaged (%d bytes, expected 32); restore it from a backup", path, len(data))
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	account, err := os.ReadFile(filepath.Join(dir, markerFile))
	switch {
	case err == nil:
		return keyFromKeychain(strings.TrimSpace(string(account)))
	case !errors.Is(err, fs.ErrNotExist):
		return nil, err
	}

	if found, err := exists(filepath.Join(dir, dbFile)); err != nil {
		return nil, err
	} else if found {
		return nil, fmt.Errorf("%s is missing but the database exists; restore the key from a backup", path)
	}
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	return key, writeNew(path, key)
}

func keyFromKeychain(account string) ([]byte, error) {
	secret, err := chain.Get(account)
	if err != nil {
		return nil, fmt.Errorf("the encryption key is in the system keychain, which could not provide it (is it unlocked?): %w", err)
	}
	key, err := hex.DecodeString(secret)
	if err != nil || len(key) != 32 {
		return nil, errors.New("the encryption key in the system keychain is damaged; restore from a backup")
	}
	return key, nil
}

// writeNew durably writes a file that must not exist yet.
func writeNew(path string, data []byte) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	_, werr := f.Write(data)
	return errors.Join(werr, f.Sync(), f.Close())
}

// Location reports where the key of the open store is kept.
func Location() KeyLocation {
	if found, _ := exists(filepath.Join(dataDir, keyFile)); !found {
		if found, _ := exists(filepath.Join(dataDir, markerFile)); found {
			return KeyInKeychain
		}
	}
	return KeyInFile
}

// MoveKeyToKeychain stores the key in the OS keychain and, only after reading
// it back, removes the key file.
func MoveKeyToKeychain() error {
	if db == nil {
		return ErrNotOpen
	}
	if Location() == KeyInKeychain {
		return nil
	}
	var id [16]byte
	if _, err := io.ReadFull(rand.Reader, id[:]); err != nil {
		return err
	}
	account := hex.EncodeToString(id[:])
	if err := chain.Set(account, hex.EncodeToString(aesKey)); err != nil {
		return fmt.Errorf("could not store the key in the system keychain: %w", err)
	}
	got, err := keyFromKeychain(account)
	if err != nil || subtle.ConstantTimeCompare(got, aesKey) != 1 {
		_ = chain.Delete(account)
		return fmt.Errorf("the system keychain did not return the key; it stays in the key file: %v", err)
	}
	marker := filepath.Join(dataDir, markerFile)
	_ = os.Remove(marker) // stale marker from an interrupted move
	if err := writeNew(marker, []byte(account)); err != nil {
		return err
	}
	return os.Remove(filepath.Join(dataDir, keyFile))
}

// MoveKeyToFile writes the key back beside the database.
func MoveKeyToFile() error {
	if db == nil {
		return ErrNotOpen
	}
	if Location() == KeyInFile {
		return nil
	}
	marker := filepath.Join(dataDir, markerFile)
	account, err := os.ReadFile(marker)
	if err != nil {
		return err
	}
	if err := writeNew(filepath.Join(dataDir, keyFile), aesKey); err != nil {
		return err
	}
	if err := os.Remove(marker); err != nil {
		return err
	}
	// The data is safe either way; a leftover keychain entry is harmless.
	_ = chain.Delete(strings.TrimSpace(string(account)))
	return nil
}

// CurrentKey returns a copy of the open store's key, for encrypted backups.
func CurrentKey() ([]byte, error) {
	if db == nil {
		return nil, ErrNotOpen
	}
	return append([]byte(nil), aesKey...), nil
}
