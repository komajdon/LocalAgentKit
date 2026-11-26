package main

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"agent-gui/internal/history"
	"agent-gui/internal/sandbox"
	"agent-gui/internal/store"
	"agent-gui/tools"
)

const (
	changeBucket = "changes"
	// maxChanges per conversation are kept; the oldest are forgotten.
	maxChanges = 200
	gitTimeout = 10 * time.Second
)

// storedChange is a journal entry.
type storedChange struct {
	ID             string       `json:"id"`
	ConversationID string       `json:"conversation_id"`
	RunID          string       `json:"run_id"`
	At             time.Time    `json:"at"`
	Undone         bool         `json:"undone"`
	Change         tools.Change `json:"change"`
}

// FileChange is what the review screen shows; the kept content stays in the
// backend.
type FileChange struct {
	ID       string    `json:"id"`
	At       time.Time `json:"at"`
	Path     string    `json:"path"`
	Kind     string    `json:"kind"`
	Diff     string    `json:"diff"`
	Undoable bool      `json:"undoable"`
	Undone   bool      `json:"undone"`
}

func changes(conversationID string) ([]storedChange, error) {
	var out []storedChange
	err := store.Scan(changeBucket, func(key string, raw []byte) error {
		if !strings.HasPrefix(key, conversationID+"/") {
			return nil
		}
		var c storedChange
		if json.Unmarshal(raw, &c) == nil {
			out = append(out, c)
		}
		return nil
	})
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, err
}

func (a *App) journalFor(conversationID, runID string) tools.Journal {
	return func(change tools.Change) {
		now := time.Now()
		c := storedChange{
			ID:             fmt.Sprintf("%s/%020d", conversationID, now.UnixNano()),
			ConversationID: conversationID, RunID: runID, At: now, Change: change,
		}
		if err := store.Put(changeBucket, c.ID, c); err != nil {
			a.logf("could not record a file change; it cannot be undone: %v", err)
			return
		}
		all, err := changes(conversationID)
		if err != nil {
			return
		}
		for len(all) > maxChanges {
			if err := store.Delete(changeBucket, all[0].ID); err != nil {
				return
			}
			all = all[1:]
		}
	}
}

func (a *App) forgetChanges(conversationID string) error {
	all, err := changes(conversationID)
	if err != nil {
		return err
	}
	for _, c := range all {
		if err := store.Delete(changeBucket, c.ID); err != nil {
			return err
		}
	}
	return nil
}

// ListFileChanges returns the agent's file changes in a conversation, newest
// first.
func (a *App) ListFileChanges(conversationID string) ([]FileChange, error) {
	all, err := changes(conversationID)
	if err != nil {
		return nil, err
	}
	out := []FileChange{}
	for i := len(all) - 1; i >= 0; i-- {
		c := all[i]
		out = append(out, FileChange{ID: c.ID, At: c.At, Path: c.Change.Path, Kind: c.Change.Kind,
			Diff: c.Change.Diff, Undoable: c.Change.Undoable && !c.Undone, Undone: c.Undone})
	}
	return out, nil
}

// UndoFileChange restores the file as it was before the change, unless it has
// been modified since. It returns an error message or "".
func (a *App) UndoFileChange(id string) string {
	var c storedChange
	if err := store.Get(changeBucket, id, &c); err != nil {
		return "no such change"
	}
	if a.runningIn(c.ConversationID) != nil {
		return "stop the agent before undoing"
	}
	if c.Undone {
		return "already undone"
	}
	if err := tools.Undo(c.Change, store.DataDir()); err != nil {
		return err.Error()
	}
	c.Undone = true
	return errString(store.Put(changeBucket, id, c))
}

// GitStatus describes the work directory's uncommitted changes.
type GitStatus struct {
	Repository bool     `json:"repository"`
	Lines      []string `json:"lines"`
	Error      string   `json:"error"`
}

// GetGitStatus runs git status for a conversation's work directory. A
// repository's configuration can make git run programs, so it runs sandboxed
// and without the network.
func (a *App) GetGitStatus(conversationID string) GitStatus {
	result := GitStatus{Lines: []string{}}
	conv, err := history.Load(conversationID)
	if err != nil {
		result.Error = err.Error()
		return result
	}
	cfg := a.config()
	opts := sandboxOptions(cfg)
	opts.WorkDir, opts.AllowNetwork, opts.Hidden = conv.WorkDir, false, []string{store.DataDir()}
	ctx, cancel := context.WithTimeout(a.ctx, gitTimeout)
	defer cancel()
	cmd, _, err := sandbox.Command(ctx, opts, conv.WorkDir,
		"git -c core.fsmonitor=false -c core.hooksPath=/dev/null status --porcelain=v1 --branch")
	if err != nil {
		result.Error = err.Error()
		return result
	}
	out := &sandbox.Output{Max: 64 * 1024}
	cmd.Stdout, cmd.Stderr = out, out
	if err := cmd.Run(); err != nil {
		if !strings.Contains(out.String(), "not a git repository") {
			result.Error = strings.TrimSpace(out.String())
			if result.Error == "" {
				result.Error = err.Error()
			}
		}
		return result
	}
	result.Repository = true
	result.Lines = strings.Split(strings.TrimRight(out.String(), "\n"), "\n")
	return result
}
