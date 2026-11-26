package tools

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"sync"

	"agent-gui/domain"
	"agent-gui/internal/diff"
)

// MaxUndoBytes is the largest previous content kept for undo.
const MaxUndoBytes = 1 << 20

// Change is a file modification the agent made.
type Change struct {
	Path      string      `json:"path"` // absolute
	Dir       string      `json:"dir"`  // the work directory it was made in
	Kind      string      `json:"kind"` // created | modified | deleted
	Diff      string      `json:"diff"`
	Before    []byte      `json:"before,omitempty"`
	Mode      fs.FileMode `json:"mode,omitempty"`
	AfterHash string      `json:"after_hash"` // "" when the file is gone
	Undoable  bool        `json:"undoable"`
}

// Journal receives each change after it is made.
type Journal func(Change)

// writes serialises compare-and-write, so two calls cannot interleave.
var writes sync.Mutex

type fileState struct {
	exists bool
	data   []byte
	mode   fs.FileMode
	hash   string
	large  bool
}

func contentHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// read captures the current state of a path inside the workspace.
func (w workspace) read(path string) (fileState, string, error) {
	root, rel, err := w.open(path)
	if err != nil {
		return fileState{}, "", err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(rel)
	if errors.Is(err, fs.ErrNotExist) {
		return fileState{}, rel, nil
	}
	if err != nil {
		return fileState{}, "", err
	}
	if !info.Mode().IsRegular() {
		return fileState{}, "", fmt.Errorf("%q is not a regular file", path)
	}
	f, err := root.Open(rel)
	if err != nil {
		return fileState{}, "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	var kept []byte
	large := info.Size() > MaxUndoBytes
	if large {
		_, err = io.Copy(h, f)
	} else {
		kept, err = io.ReadAll(f)
		h.Write(kept)
	}
	if err != nil {
		return fileState{}, "", err
	}
	return fileState{exists: true, data: kept, mode: info.Mode().Perm(), hash: hex.EncodeToString(h.Sum(nil)), large: large}, rel, nil
}

// apply runs change only if the file is still in the state the plan saw.
func (w workspace) apply(ctx context.Context, path string, seen fileState, change func() (string, error), record Change, journal Journal) string {
	if ctx.Err() != nil {
		return "ERROR: " + ctx.Err().Error()
	}
	writes.Lock()
	defer writes.Unlock()
	now, _, err := w.read(path)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if now.exists != seen.exists || now.hash != seen.hash {
		return "ERROR: conflict: " + path + " changed after the change was prepared; nothing was written. Read it again."
	}
	message, err := change()
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if journal != nil {
		journal(record)
	}
	return message
}

func (t *writeFileTool) Plan(_ context.Context, args domain.Args) (domain.Plan, error) {
	path, content := args.String("path"), []byte(args.String("content"))
	seen, rel, err := t.ws.read(path)
	if err != nil {
		return domain.Plan{}, err
	}
	record := Change{
		Path: filepath.Join(t.ws.dir, rel), Dir: t.ws.dir, Kind: "created", Mode: seen.mode,
		Before: seen.data, AfterHash: contentHash(content), Undoable: !seen.large,
	}
	if seen.exists {
		record.Kind = "modified"
	}
	if seen.large {
		record.Diff = fmt.Sprintf("(%s is too large to compare; %d bytes after)\n", rel, len(content))
	} else {
		record.Diff = diff.Unified(filepath.ToSlash(rel), string(seen.data), string(content))
	}
	return domain.Plan{Preview: record.Diff, Apply: func(ctx context.Context) string {
		return t.ws.apply(ctx, path, seen, func() (string, error) {
			root, rel, err := t.ws.open(path)
			if err != nil {
				return "", err
			}
			defer func() { _ = root.Close() }()
			if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
				return "", err
			}
			if err := root.WriteFile(rel, content, 0o644); err != nil {
				return "", err
			}
			return fmt.Sprintf("Written %d bytes to %s", len(content), record.Path), nil
		}, record, t.journal)
	}}, nil
}

func (t *deleteFileTool) Plan(_ context.Context, args domain.Args) (domain.Plan, error) {
	path := args.String("path")
	root, rel, err := t.ws.open(path)
	if err != nil {
		return domain.Plan{}, err
	}
	defer func() { _ = root.Close() }()
	if rel == "." {
		return domain.Plan{}, errors.New("refusing to delete the work directory itself")
	}
	info, err := root.Lstat(rel)
	if err != nil {
		return domain.Plan{}, err
	}
	abs := filepath.Join(t.ws.dir, rel)
	remove := func() (string, error) {
		root, rel, err := t.ws.open(path)
		if err != nil {
			return "", err
		}
		defer func() { _ = root.Close() }()
		// Remove deletes a symlink itself, never its target.
		if err := root.Remove(rel); err != nil {
			return "", err
		}
		return "Deleted: " + abs, nil
	}
	// Links and empty directories are removed without an undo record.
	if !info.Mode().IsRegular() {
		return domain.Plan{Preview: "Remove " + rel + " (not a regular file; cannot be undone)", Apply: func(context.Context) string {
			message, err := remove()
			if err != nil {
				return "ERROR: " + err.Error()
			}
			return message
		}}, nil
	}
	seen, _, err := t.ws.read(path)
	if err != nil {
		return domain.Plan{}, err
	}
	record := Change{Path: abs, Dir: t.ws.dir, Kind: "deleted", Mode: seen.mode, Before: seen.data, Undoable: !seen.large,
		Diff: diff.Unified(filepath.ToSlash(rel), string(seen.data), "")}
	return domain.Plan{Preview: record.Diff, Apply: func(ctx context.Context) string {
		return t.ws.apply(ctx, path, seen, remove, record, t.journal)
	}}, nil
}

// Undo restores the state before a change. It refuses when the file is no
// longer as the agent left it, so later edits are never overwritten.
func Undo(c Change, protected ...string) error {
	if !c.Undoable {
		return errors.New("this change cannot be undone: the previous content was not kept")
	}
	ws := workspace{dir: c.Dir, protected: protected}
	writes.Lock()
	defer writes.Unlock()
	now, _, err := ws.read(c.Path)
	if err != nil {
		return err
	}
	if (c.AfterHash == "") == now.exists || (now.exists && now.hash != c.AfterHash) {
		return errors.New("conflict: the file has changed since; it was left as it is")
	}
	root, rel, err := ws.open(c.Path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	if c.Kind == "created" {
		return root.Remove(rel)
	}
	if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return err
	}
	mode := c.Mode
	if mode == 0 {
		mode = 0o644
	}
	return root.WriteFile(rel, c.Before, mode)
}
