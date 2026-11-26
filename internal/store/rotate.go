package store

import (
	"crypto/rand"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	bolt "go.etcd.io/bbolt"
)

// nextKeyFile holds the new key while the database is being re-encrypted.
const nextKeyFile = "agent.key.next"

// beforeKeySwap lets tests interrupt a rotation.
var beforeKeySwap = func() {}

// RotateKey re-encrypts every value under a new key. It requires the key to be
// in a file.
//
// The new key is written first, the database is rewritten in one transaction,
// then the new key replaces the old one. Whichever step is interrupted, one of
// the two key files matches the database, and Init picks it.
func RotateKey() error {
	if db == nil {
		return ErrNotOpen
	}
	if Location() != KeyInFile {
		return errors.New("move the key to a file before rotating it, then move it back")
	}
	next := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, next); err != nil {
		return err
	}
	nextPath := filepath.Join(dataDir, nextKeyFile)
	if err := os.Remove(nextPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	if err := writeNew(nextPath, next); err != nil {
		return err
	}
	old := aesKey
	err := db.Update(func(tx *bolt.Tx) error {
		return tx.ForEach(func(_ []byte, b *bolt.Bucket) error {
			// Collect first: a bucket must not change while it is iterated.
			updated := map[string][]byte{}
			if err := b.ForEach(func(k, v []byte) error {
				plain, err := decryptWith(old, v)
				if err != nil {
					return err
				}
				sealed, err := encryptWith(next, plain)
				if err != nil {
					return err
				}
				updated[string(k)] = sealed
				return nil
			}); err != nil {
				return err
			}
			for k, v := range updated {
				if err := b.Put([]byte(k), v); err != nil {
					return err
				}
			}
			return nil
		})
	})
	if err != nil {
		_ = os.Remove(nextPath)
		return err
	}
	aesKey = next
	beforeKeySwap()
	return os.Rename(nextPath, filepath.Join(dataDir, keyFile))
}

// recoverRotation finishes or discards a rotation that was interrupted.
func recoverRotation(dir string) error {
	nextPath := filepath.Join(dir, nextKeyFile)
	next, err := os.ReadFile(nextPath)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if len(next) == 32 {
		if found, err := exists(filepath.Join(dir, dbFile)); err != nil {
			return err
		} else if found && verify(filepath.Join(dir, dbFile), next) == nil {
			// The database was rewritten: the new key is the key.
			return os.Rename(nextPath, filepath.Join(dir, keyFile))
		}
	}
	return os.Remove(nextPath) // the database still uses the old key
}
