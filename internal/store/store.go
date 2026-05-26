// Package store provides an encrypted, embedded key-value database.
// All values are AES-256-GCM encrypted before being written to disk,
// so the on-disk file is never human-readable plain text.
//
// The encryption key is generated randomly on first run and persisted
// in <dataDir>/agent.key (mode 0600). Losing that file means losing
// access to the encrypted data.
package store

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sync"

	bolt "go.etcd.io/bbolt"
)

var (
	db      *bolt.DB
	aesKey  []byte
	once    sync.Once
	initErr error
)

// Init opens (or creates) the encrypted database in dir.
// Must be called once before any other store function.
func Init(dir string) error {
	once.Do(func() {
		if err := os.MkdirAll(dir, 0700); err != nil {
			initErr = fmt.Errorf("store: create data dir: %w", err)
			return
		}
		key, err := loadOrCreateKey(filepath.Join(dir, "agent.key"))
		if err != nil {
			initErr = fmt.Errorf("store: load encryption key: %w", err)
			return
		}
		aesKey = key

		db, err = bolt.Open(filepath.Join(dir, "agent.db"), 0600, nil)
		if err != nil {
			initErr = fmt.Errorf("store: open db: %w", err)
			return
		}
	})
	return initErr
}

// Close closes the database.
func Close() error {
	if db != nil {
		return db.Close()
	}
	return nil
}

// DataDir returns the directory where the database should be stored:
// a "data/" folder next to the running executable.
// Falls back to ~/.ai-agent/data if the executable path cannot be resolved.
func DataDir() string {
	exe, err := os.Executable()
	if err == nil {
		if resolved, err := filepath.EvalSymlinks(exe); err == nil {
			exe = resolved
		}
		return filepath.Join(filepath.Dir(exe), "data")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".ai-agent", "data")
}

// Put JSON-marshals v, encrypts it, and stores it at bucket/key.
func Put(bucket, key string, v any) error {
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

// Delete removes key from bucket (no-op if either does not exist).
func Delete(bucket, key string) error {
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

// ── Key management ────────────────────────────────────────────────────────────

// loadOrCreateKey reads the 32-byte key from path, or generates and saves a
// new random key if the file does not exist.
func loadOrCreateKey(path string) ([]byte, error) {
	data, err := os.ReadFile(path)
	if err == nil && len(data) == 32 {
		return data, nil
	}
	// Generate a new random 32-byte key.
	key := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, key); err != nil {
		return nil, err
	}
	if err := os.WriteFile(path, key, 0600); err != nil {
		return nil, err
	}
	return key, nil
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

func encrypt(plain []byte) ([]byte, error) {
	block, err := aes.NewCipher(aesKey)
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

func decrypt(data []byte) ([]byte, error) {
	block, err := aes.NewCipher(aesKey)
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
