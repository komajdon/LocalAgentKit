package history

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"agent-gui/internal/store"
)

// SchemaVersion is the layout this code writes. Version 1 kept only full
// conversations; version 2 adds the index bucket.
const SchemaVersion = 2

const (
	indexBucket  = "conversation_index"
	schemaBucket = "schema"
	schemaKey    = "history"
)

// indexEntry is everything the sidebar and usage totals need.
type indexEntry struct {
	ConvMeta
	TokenUsage   TokenUsage `json:"token_usage"`
	MessageCount int        `json:"message_count"`
	Hash         string     `json:"hash"` // of the full record's ciphertext
}

func entryFor(c *Conversation, hash string) indexEntry {
	return indexEntry{ConvMeta: c.ConvMeta, TokenUsage: c.TokenUsage, MessageCount: len(c.Messages), Hash: hash}
}

func index() (map[string]indexEntry, error) {
	out := map[string]indexEntry{}
	err := store.Scan(indexBucket, func(key string, raw []byte) error {
		var e indexEntry
		if json.Unmarshal(raw, &e) == nil {
			out[key] = e
		}
		return nil
	})
	return out, err
}

// Migrate brings the stored history to SchemaVersion and repairs index records
// that do not match their conversation. Before the first upgrade of existing
// data it copies the database into backupDir; to roll back, put that copy back
// as agent.db. Data from a newer version is refused.
func Migrate(backupDir string) error {
	version := 1
	if store.Has(schemaBucket, schemaKey) {
		if err := store.Get(schemaBucket, schemaKey, &version); err != nil {
			return fmt.Errorf("read schema version: %w", err)
		}
	}
	if version > SchemaVersion {
		return fmt.Errorf("the data was written by a newer version of the app (schema %d, this version reads %d)", version, SchemaVersion)
	}
	full, err := store.Hashes(bucket)
	if err != nil {
		return err
	}
	if version < SchemaVersion && len(full) > 0 {
		if err := os.MkdirAll(backupDir, 0o700); err != nil {
			return err
		}
		name := fmt.Sprintf("pre-migration-v%d-%s.db", version, time.Now().Format("2006-01-02_150405.000"))
		if err := store.Snapshot(filepath.Join(backupDir, name)); err != nil {
			return fmt.Errorf("back up before migration: %w", err)
		}
	}
	if err := reconcile(full); err != nil {
		return err
	}
	if version == SchemaVersion && store.Has(schemaBucket, schemaKey) {
		return nil
	}
	return store.Put(schemaBucket, schemaKey, SchemaVersion)
}

// reconcile makes the index match the conversations, decrypting only those
// whose ciphertext changed.
func reconcile(full map[string]string) error {
	entries, err := index()
	if err != nil {
		return err
	}
	for key, hash := range full {
		if e, ok := entries[key]; ok && e.Hash == hash {
			continue
		}
		c, err := Load(key)
		if err != nil {
			continue // an unreadable conversation is left as it is
		}
		if err := Save(c); err != nil {
			return err
		}
	}
	for key := range entries {
		if _, ok := full[key]; !ok {
			if err := store.Delete(indexBucket, key); err != nil {
				return err
			}
		}
	}
	return nil
}
