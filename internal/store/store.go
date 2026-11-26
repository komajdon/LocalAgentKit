// Package store provides an encrypted, embedded key-value database.
// All values are AES-256-GCM encrypted before being written to disk,
// so the on-disk file is never human-readable plain text.
//
// The encryption key is generated randomly on first run. It is kept in
// <dataDir>/agent.key (mode 0600) by default, which protects the database
// only from someone who gets the database without the key file, or in the OS
// keychain if the user moves it there. Losing the key means losing the data.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"

	bolt "go.etcd.io/bbolt"
)

var (
	db      *bolt.DB
	dataDir string
	aesKey  []byte
	once    sync.Once
	initErr error
)

// ErrNotOpen is returned when the store failed to open or has been closed.
var ErrNotOpen = errors.New("store: data store is not open")

// Init opens (or creates) the encrypted database in dir.
// Must be called once before any other store function.
func Init(dir string) error {
	once.Do(func() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			initErr = fmt.Errorf("store: create data dir: %w", err)
			return
		}
		dataDir = dir
		if err := applyPendingRestore(dir); err != nil {
			initErr = fmt.Errorf("store: apply restore: %w", err)
			return
		}
		if err := recoverRotation(dir); err != nil {
			initErr = fmt.Errorf("store: recover key rotation: %w", err)
			return
		}
		key, err := loadOrCreateKey(dir)
		if err != nil {
			initErr = fmt.Errorf("store: load encryption key: %w", err)
			return
		}
		aesKey = key

		// A timeout, so a second instance reports the lock instead of hanging.
		db, err = bolt.Open(filepath.Join(dir, "agent.db"), 0600, &bolt.Options{Timeout: 3 * time.Second})
		if err != nil {
			initErr = fmt.Errorf("store: open db: %w", err)
			return
		}
	})
	return initErr
}

// Close closes the database.
func Close() error {
	if db == nil {
		return nil
	}
	err := db.Close()
	db = nil
	return err
}

// DataDir returns the directory where the database should be stored.
// Priority order:
//  1. Inside a Flatpak sandbox: $XDG_DATA_HOME/ai-agent (writable per-app dir)
//  2. data/ folder next to the running executable (standard install)
//  3. ~/.ai-agent/data (fallback when exe path cannot be resolved)
func DataDir() string {
	// Flatpak sets FLATPAK_ID and XDG_DATA_HOME; /app is read-only so we must
	// use the sandboxed data dir (~/.var/app/<id>/data) for all writes.
	if os.Getenv("FLATPAK_ID") != "" {
		if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
			return filepath.Join(xdg, "ai-agent")
		}
	}
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		// Portable layout: keep data next to the binary when that location is
		// writable (zip/tar/AppImage installs). System packages put the binary
		// in a read-only prefix like /usr/bin, so fall back to a per-user dir.
		beside := filepath.Join(filepath.Dir(exe), "data")
		if isWritable(beside) {
			return beside
		}
	}
	if xdg := os.Getenv("XDG_DATA_HOME"); xdg != "" {
		return filepath.Join(xdg, "ai-agent")
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".local", "share", "ai-agent")
	}
	return filepath.Join(os.TempDir(), "ai-agent")
}

// isWritable reports whether dir can be created and written to.
func isWritable(dir string) bool {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return false
	}
	probe := filepath.Join(dir, ".write-test")
	f, err := os.OpenFile(probe, os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return false
	}
	_ = f.Close()
	_ = os.Remove(probe)
	return true
}

// Put JSON-marshals v, encrypts it, and stores it at bucket/key.
func Put(bucket, key string, v any) error {
	if db == nil {
		return ErrNotOpen
	}
	data, err := encryptJSON(v)
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		b, err := tx.CreateBucketIfNotExists([]byte(bucket))
		if err != nil {
			return err
		}
		return b.Put([]byte(key), data)
	})
}

// Get decrypts the value at bucket/key and JSON-unmarshals it into out.
// Returns an error if the bucket or key does not exist.
func Get(bucket, key string, out any) error {
	if db == nil {
		return ErrNotOpen
	}
	return db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return fmt.Errorf("store: bucket %q not found", bucket)
		}
		raw := b.Get([]byte(key))
		if raw == nil {
			return fmt.Errorf("store: key %q not found in %q", key, bucket)
		}
		return decryptJSON(raw, out)
	})
}

// Has reports whether a value exists at bucket/key.
func Has(bucket, key string) bool {
	found := false
	if db == nil {
		return found
	}
	_ = db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		found = b != nil && b.Get([]byte(key)) != nil
		return nil
	})
	return found
}

// Delete removes key from bucket (no-op if either does not exist).
func Delete(bucket, key string) error {
	if db == nil {
		return ErrNotOpen
	}
	return db.Update(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.Delete([]byte(key))
	})
}

// Scan calls fn for every decrypted raw-JSON value in bucket.
// Corrupt or unreadable entries are silently skipped.
func Scan(bucket string, fn func(key string, rawJSON []byte) error) error {
	if db == nil {
		return ErrNotOpen
	}
	return db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			plain, err := decrypt(v)
			if err != nil {
				return nil // skip corrupt entry
			}
			return fn(string(k), plain)
		})
	})
}

// ── Crypto ────────────────────────────────────────────────────────────────────

func encryptJSON(v any) ([]byte, error) {
	plain, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return encrypt(plain)
}

func decryptJSON(data []byte, out any) error {
	plain, err := decrypt(data)
	if err != nil {
		return err
	}
	return json.Unmarshal(plain, out)
}

func encrypt(plain []byte) ([]byte, error) { return encryptWith(aesKey, plain) }

func encryptWith(key, plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, err
	}
	return gcm.Seal(nonce, nonce, plain, nil), nil
}

func decrypt(data []byte) ([]byte, error) { return decryptWith(aesKey, data) }

func decryptWith(key, data []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonceSize := gcm.NonceSize()
	if len(data) < nonceSize {
		return nil, fmt.Errorf("store: ciphertext too short")
	}
	return gcm.Open(nil, data[:nonceSize], data[nonceSize:], nil)
}
