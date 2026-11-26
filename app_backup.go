package main

import (
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"agent-gui/internal/backup"
	"agent-gui/internal/store"
)

// maxBackupArchives is how many automatic backups are kept.
const maxBackupArchives = 7

// snapshotBytes returns a consistent copy of the open database.
func snapshotBytes() ([]byte, error) {
	dir, err := os.MkdirTemp("", "lak-snapshot-*")
	if err != nil {
		return nil, err
	}
	defer func() { _ = os.RemoveAll(dir) }()
	path := filepath.Join(dir, backup.Database)
	if err := store.Snapshot(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

// writeAutoBackup saves a timestamped copy of the database, without the key,
// in dir and prunes old ones. Old backups are only removed after the new one
// was written.
func writeAutoBackup(dir string) error {
	data, err := snapshotBytes()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	name := "backup-" + time.Now().Format("2006-01-02_150405") + ".tar.gz"
	if err := backup.WritePlain(filepath.Join(dir, name), data); err != nil {
		return err
	}
	entries, err := filepath.Glob(filepath.Join(dir, "backup-*.tar.gz"))
	if err != nil {
		return err
	}
	sort.Strings(entries) // the timestamp format sorts oldest first
	for len(entries) > maxBackupArchives {
		if err := os.Remove(entries[0]); err != nil {
			return err
		}
		entries = entries[1:]
	}
	return nil
}

// ExportBackup writes a passphrase-encrypted backup to a file the user picks.
// It returns "ok", an error message, or "" if the dialog was cancelled.
func (a *App) ExportBackup(passphrase string) string {
	if len([]rune(passphrase)) < backup.MinPassphrase {
		return "Enter a passphrase of at least 8 characters; it is needed to restore."
	}
	dest, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{
		Title:           "Export Backup",
		DefaultFilename: "ai-agent.lakbackup",
		Filters:         []runtime.FileFilter{{DisplayName: "Encrypted backup (*.lakbackup)", Pattern: "*.lakbackup"}},
	})
	if err != nil || dest == "" {
		return ""
	}
	if err := exportBackupTo(dest, passphrase); err != nil {
		return "Backup failed: " + err.Error()
	}
	return "ok"
}

func exportBackupTo(dest, passphrase string) error {
	data, err := snapshotBytes()
	if err != nil {
		return err
	}
	key, err := store.CurrentKey()
	if err != nil {
		return err
	}
	return backup.WriteEncrypted(dest, passphrase, data, key)
}

// ImportBackup checks a backup the user picks and queues it to replace the
// current data at the next start, which keeps a copy of what it replaces.
// It returns "ok", an error message, or "" if the dialog was cancelled.
func (a *App) ImportBackup(passphrase string) string {
	src, err := runtime.OpenFileDialog(a.ctx, runtime.OpenDialogOptions{
		Title:   "Restore Backup",
		Filters: []runtime.FileFilter{{DisplayName: "Backups (*.lakbackup, *.tar.gz)", Pattern: "*.lakbackup;*.tar.gz"}},
	})
	if err != nil || src == "" {
		return ""
	}
	if err := stageBackup(src, passphrase); err != nil {
		return "Restore failed: " + err.Error()
	}
	a.mu.Lock()
	a.restoreStaged = true
	a.mu.Unlock()
	return "ok"
}

func stageBackup(src, passphrase string) error {
	files, err := backup.Read(src, passphrase)
	if err != nil {
		return err
	}
	// Archives from earlier versions are unencrypted and include the key.
	return store.StageRestore(files[backup.Database], files[backup.Key])
}

// GetKeyLocation reports where the encryption key is kept: "file" or
// "keychain".
func (a *App) GetKeyLocation() string { return string(store.Location()) }

// SetKeyLocation moves the encryption key. It returns an error message or "".
func (a *App) SetKeyLocation(location string) string {
	switch store.KeyLocation(location) {
	case store.KeyInKeychain:
		return errString(store.MoveKeyToKeychain())
	case store.KeyInFile:
		return errString(store.MoveKeyToFile())
	}
	return "unknown key location"
}
