package store

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	bolt "go.etcd.io/bbolt"
)

const (
	pendingDir = "restore-pending"
	asideDir   = "pre-restore-"
)

// Snapshot writes a consistent copy of the open database to path.
func Snapshot(path string) error {
	if db == nil {
		return ErrNotOpen
	}
	return db.View(func(tx *bolt.Tx) error { return tx.CopyFile(path, 0o600) })
}

// verify checks that every value in the database at path decrypts with key.
func verify(path string, key []byte) error {
	candidate, err := bolt.Open(path, 0o600, &bolt.Options{ReadOnly: true, Timeout: time.Second})
	if err != nil {
		return fmt.Errorf("not a usable database: %w", err)
	}
	defer func() { _ = candidate.Close() }()
	return candidate.View(func(tx *bolt.Tx) error {
		for err := range tx.Check() {
			return fmt.Errorf("database is corrupt: %w", err)
		}
		return tx.ForEach(func(_ []byte, b *bolt.Bucket) error {
			return b.ForEach(func(_, v []byte) error {
				if _, err := decryptWith(key, v); err != nil {
					return errors.New("the backup's data does not match its encryption key")
				}
				return nil
			})
		})
	})
}

// StageRestore validates a backup and queues it for the next start; the open
// database is not touched. A nil key means the backup was made without one and
// must match the current key.
func StageRestore(database, key []byte) error {
	if db == nil {
		return ErrNotOpen
	}
	checkKey := key
	if checkKey == nil {
		checkKey = aesKey
	} else if len(key) != 32 {
		return errors.New("the backup's encryption key is damaged")
	}

	tmp, err := os.MkdirTemp(dataDir, pendingDir+"-*")
	if err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }() // no-op once renamed
	if err := writeNew(filepath.Join(tmp, dbFile), database); err != nil {
		return err
	}
	if err := verify(filepath.Join(tmp, dbFile), checkKey); err != nil {
		return err
	}
	if key != nil {
		if err := writeNew(filepath.Join(tmp, keyFile), key); err != nil {
			return err
		}
	}
	pending := filepath.Join(dataDir, pendingDir)
	if err := os.RemoveAll(pending); err != nil { // an earlier, unapplied restore
		return err
	}
	return os.Rename(tmp, pending)
}

// applyPendingRestore swaps a staged backup in before the database is opened.
// The replaced files are kept in a pre-restore-<time> folder.
func applyPendingRestore(dir string) error {
	pending := filepath.Join(dir, pendingDir)
	staged, err := exists(filepath.Join(pending, dbFile))
	if err != nil || !staged {
		return err
	}
	hasKey, err := exists(filepath.Join(pending, keyFile))
	if err != nil {
		return err
	}
	aside := filepath.Join(dir, asideDir+time.Now().Format("2006-01-02_150405.000"))
	if err := os.Mkdir(aside, 0o700); err != nil {
		return err
	}
	// A restore that brings its own key replaces the current key files too.
	replaced := []string{dbFile}
	if hasKey {
		replaced = append(replaced, keyFile, markerFile)
	}
	for _, name := range replaced {
		found, err := exists(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		if found {
			if err := os.Rename(filepath.Join(dir, name), filepath.Join(aside, name)); err != nil {
				return err
			}
		}
	}
	if hasKey {
		if err := os.Rename(filepath.Join(pending, keyFile), filepath.Join(dir, keyFile)); err != nil {
			return err
		}
	}
	// The database moves last: until it does, the restore is retried.
	if err := os.Rename(filepath.Join(pending, dbFile), filepath.Join(dir, dbFile)); err != nil {
		return err
	}
	return os.RemoveAll(pending)
}
