// Package backup reads and writes backup archives.
//
// A plain archive is a tar.gz holding the encrypted database. An encrypted
// archive additionally holds the database key, and the whole tar.gz is sealed
// with AES-256-GCM under a key derived from a passphrase with Argon2id:
//
//	magic | time u32 | memory u32 | threads u8 | salt[16] | nonce[12] | sealed
//
// Everything before the sealed part is authenticated.
package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/crypto/argon2"
)

// Names of the files an archive may hold. Others are ignored when reading.
const (
	Database = "agent.db"
	Key      = "agent.key"
)

const (
	magic      = "LAKBKP1\n"
	headerLen  = len(magic) + 4 + 4 + 1 + 16 + 12
	maxEntry   = 2 << 30 // refuse absurd sizes from a hostile archive
	argonTime  = 3
	argonMemKB = 64 * 1024
	argonProcs = 4
)

// MinPassphrase is the shortest passphrase accepted for a new backup.
const MinPassphrase = 8

var (
	ErrPassphraseRequired = errors.New("this backup is encrypted: enter its passphrase")
	ErrBadPassphrase      = errors.New("wrong passphrase, or the backup is damaged")
)

func pack(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, name := range []string{Database, Key} {
		data, ok := files[name]
		if !ok {
			continue
		}
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o600, Size: int64(len(data))}); err != nil {
			return nil, err
		}
		if _, err := tw.Write(data); err != nil {
			return nil, err
		}
	}
	if err := errors.Join(tw.Close(), gz.Close()); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func unpack(data []byte) (map[string][]byte, error) {
	gz, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("not a backup archive: %w", err)
	}
	tr := tar.NewReader(gz)
	files := map[string][]byte{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("damaged archive: %w", err)
		}
		if hdr.Name != Database && hdr.Name != Key {
			continue
		}
		if hdr.Size > maxEntry {
			return nil, errors.New("archive entry is too large")
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			return nil, fmt.Errorf("damaged archive: %w", err)
		}
		files[hdr.Name] = body
	}
	// Reading to the end verifies the gzip checksum.
	if _, err := io.Copy(io.Discard, gz); err != nil {
		return nil, fmt.Errorf("damaged archive: %w", err)
	}
	if len(files[Database]) == 0 {
		return nil, errors.New("the archive holds no database")
	}
	return files, nil
}

// writeAtomic replaces dest only once the new content is safely on disk.
func writeAtomic(dest string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(dest), ".backup-*") // mode 0600
	if err != nil {
		return err
	}
	defer func() { _ = os.Remove(f.Name()) }() // no-op after the rename
	_, werr := f.Write(data)
	if err := errors.Join(werr, f.Sync(), f.Close()); err != nil {
		return err
	}
	return os.Rename(f.Name(), dest)
}

// WritePlain writes an unencrypted archive of the database. It refuses to
// include the key, which would make the archive readable by anyone.
func WritePlain(dest string, database []byte) error {
	data, err := pack(map[string][]byte{Database: database})
	if err != nil {
		return err
	}
	return writeAtomic(dest, data)
}

func gcmFor(passphrase string, salt []byte, t, mem uint32, procs uint8) (cipher.AEAD, error) {
	block, err := aes.NewCipher(argon2.IDKey([]byte(passphrase), salt, t, mem, procs, 32))
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// WriteEncrypted writes a passphrase-protected archive that can be restored on
// any machine.
func WriteEncrypted(dest, passphrase string, database, key []byte) error {
	if len([]rune(passphrase)) < MinPassphrase {
		return fmt.Errorf("the passphrase must be at least %d characters", MinPassphrase)
	}
	plain, err := pack(map[string][]byte{Database: database, Key: key})
	if err != nil {
		return err
	}
	header := make([]byte, headerLen)
	n := copy(header, magic)
	binary.BigEndian.PutUint32(header[n:], argonTime)
	binary.BigEndian.PutUint32(header[n+4:], argonMemKB)
	header[n+8] = argonProcs
	if _, err := rand.Read(header[n+9:]); err != nil { // salt and nonce
		return err
	}
	salt, nonce := header[n+9:n+25], header[n+25:]
	gcm, err := gcmFor(passphrase, salt, argonTime, argonMemKB, argonProcs)
	if err != nil {
		return err
	}
	return writeAtomic(dest, gcm.Seal(header, nonce, plain, header))
}

// Read returns the files in an archive of either kind.
func Read(src, passphrase string) (map[string][]byte, error) {
	data, err := os.ReadFile(src)
	if err != nil {
		return nil, err
	}
	if !bytes.HasPrefix(data, []byte(magic)) {
		return unpack(data)
	}
	if passphrase == "" {
		return nil, ErrPassphraseRequired
	}
	if len(data) < headerLen {
		return nil, ErrBadPassphrase
	}
	header, n := data[:headerLen], len(magic)
	t, mem, procs := binary.BigEndian.Uint32(header[n:]), binary.BigEndian.Uint32(header[n+4:]), header[n+8]
	// Bound the work a crafted file can demand.
	if t == 0 || t > 10 || mem > 1024*1024 || procs == 0 || procs > 16 {
		return nil, ErrBadPassphrase
	}
	gcm, err := gcmFor(passphrase, header[n+9:n+25], t, mem, procs)
	if err != nil {
		return nil, err
	}
	plain, err := gcm.Open(nil, header[n+25:], data[headerLen:], header)
	if err != nil {
		return nil, ErrBadPassphrase
	}
	return unpack(plain)
}
