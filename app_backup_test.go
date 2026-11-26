package main

import (
	"path/filepath"
	"testing"
	"time"

	"agent-gui/internal/backup"
	"agent-gui/internal/config"
	"agent-gui/internal/history"
)

func TestAutoBackupExcludesKeyAndPrunes(t *testing.T) {
	if err := history.Save(&history.Conversation{ConvMeta: history.ConvMeta{ID: "b1", Title: "t", UpdatedAt: time.Now()}}); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for i := 0; i < config.DefaultBackupRetention; i++ {
		name := filepath.Join(dir, "backup-2000-01-0"+string(rune('1'+i))+"_000000.tar.gz")
		if err := backup.WritePlain(name, []byte("old")); err != nil {
			t.Fatal(err)
		}
	}
	if err := writeAutoBackup(dir, config.DefaultBackupRetention); err != nil {
		t.Fatal(err)
	}
	all, _ := filepath.Glob(filepath.Join(dir, "backup-*.tar.gz"))
	if len(all) != config.DefaultBackupRetention {
		t.Fatalf("kept %d", len(all))
	}
	files, err := backup.Read(all[len(all)-1], "")
	if err != nil || files[backup.Key] != nil || len(files[backup.Database]) == 0 {
		t.Fatalf("newest archive: err=%v key=%v", err, files[backup.Key] != nil)
	}
	// A real snapshot passes restore validation against the current key.
	if err := stageBackup(all[len(all)-1], ""); err != nil {
		t.Fatal(err)
	}
}

func TestEncryptedExportRestores(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "x.lakbackup")
	if err := exportBackupTo(dest, "a long passphrase"); err != nil {
		t.Fatal(err)
	}
	if err := stageBackup(dest, "not the passphrase"); err == nil {
		t.Fatal("wrong passphrase accepted")
	}
	if err := stageBackup(dest, "a long passphrase"); err != nil {
		t.Fatal(err)
	}
}
