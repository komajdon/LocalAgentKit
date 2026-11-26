package store

import (
	"crypto/sha256"
	"encoding/hex"

	bolt "go.etcd.io/bbolt"
)

// A bucket can have an index bucket holding a small record per key. Both are
// written in one transaction, and the index record carries the hash of the
// stored ciphertext, so a record changed without its index (for example by an
// older version of the app) can be found without decrypting anything.

func hashOf(ciphertext []byte) string {
	sum := sha256.Sum256(ciphertext)
	return hex.EncodeToString(sum[:])
}

// PutIndexed stores v at bucket/key and index(hash) at indexBucket/key.
func PutIndexed(bucket, indexBucket, key string, v any, index func(hash string) any) error {
	if db == nil {
		return ErrNotOpen
	}
	data, err := encryptJSON(v)
	if err != nil {
		return err
	}
	entry, err := encryptJSON(index(hashOf(data)))
	if err != nil {
		return err
	}
	return db.Update(func(tx *bolt.Tx) error {
		for name, value := range map[string][]byte{bucket: data, indexBucket: entry} {
			b, err := tx.CreateBucketIfNotExists([]byte(name))
			if err != nil {
				return err
			}
			if err := b.Put([]byte(key), value); err != nil {
				return err
			}
		}
		return nil
	})
}

// DeleteIndexed removes a key and its index record together.
func DeleteIndexed(bucket, indexBucket, key string) error {
	if db == nil {
		return ErrNotOpen
	}
	return db.Update(func(tx *bolt.Tx) error {
		for _, name := range []string{bucket, indexBucket} {
			if b := tx.Bucket([]byte(name)); b != nil {
				if err := b.Delete([]byte(key)); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

// Hashes returns the ciphertext hash of every record in bucket, without
// decrypting.
func Hashes(bucket string) (map[string]string, error) {
	if db == nil {
		return nil, ErrNotOpen
	}
	out := map[string]string{}
	err := db.View(func(tx *bolt.Tx) error {
		b := tx.Bucket([]byte(bucket))
		if b == nil {
			return nil
		}
		return b.ForEach(func(k, v []byte) error {
			out[string(k)] = hashOf(v)
			return nil
		})
	})
	return out, err
}
