// Package profile defines agent profiles and named workspaces.
//
// Both can only narrow what the global settings allow: a permission can be made
// stricter, a tool removed or a limit lowered, never the reverse.
package profile

import (
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"slices"
	"strings"
)

// Profile configures how the agent behaves in a conversation.
type Profile struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	BuiltIn      bool   `json:"built_in"`
	Instructions string `json:"instructions"`
	Model        string `json:"model"` // "" keeps the configured model
	// Tools lists the enabled tools; empty means all of them.
	Tools        []string          `json:"tools"`
	Permissions  map[string]string `json:"permissions"` // tool → ask | deny
	ContextLimit int               `json:"context_limit"`
	Strategy     string            `json:"context_strategy"`
	MaxRounds    int               `json:"max_rounds"`
}

// Workspace is a named directory with its own rules, which also apply to
// directories below it.
type Workspace struct {
	ID          string            `json:"id"`
	Name        string            `json:"name"`
	Path        string            `json:"path"`
	ProfileID   string            `json:"profile_id"`
	Permissions map[string]string `json:"permissions"`
	// Ignore holds patterns the file tools may neither list nor open.
	Ignore []string `json:"ignore"`
}

// DefaultID is the profile used when none is chosen.
const DefaultID = "general"

var readTools = []string{"read_file", "list_dir", "search_files", "read_document", "get_time"}

// BuiltIns are the profiles that always exist.
func BuiltIns() []Profile {
	return []Profile{
		{ID: DefaultID, Name: "General Assistant", BuiltIn: true},
		{ID: "coding", Name: "Coding Agent", BuiltIn: true,
			Instructions: "You help with software. Read the relevant files before changing them, make focused changes and check them."},
		{ID: "research", Name: "Research Agent", BuiltIn: true,
			Instructions: "You research questions. Search, read sources, say where each claim comes from and what you could not confirm.",
			Tools:        append([]string{"search_web"}, readTools...)},
		{ID: "readonly", Name: "Read-Only Assistant", BuiltIn: true,
			Instructions: "You can read files but not change them, run commands or use external services.",
			Tools:        slices.Clone(readTools)},
	}
}

var rank = map[string]int{"allow": 0, "ask": 1, "deny": 2}

// Stricter returns the global permission tightened by any restrictions. An
// unset global permission means ask; an unset restriction means nothing.
func Stricter(global string, restrictions ...string) string {
	if _, ok := rank[global]; !ok {
		global = "ask"
	}
	for _, r := range restrictions {
		if n, ok := rank[r]; ok && n > rank[global] {
			global = r
		}
	}
	return global
}

// Lower returns the smaller of two limits, where 0 means unset.
func Lower(a, b int) int {
	if a <= 0 || (b > 0 && b < a) {
		return b
	}
	return a
}

// Allows reports whether the profile enables the tool.
func (p Profile) Allows(tool string) bool {
	return len(p.Tools) == 0 || slices.Contains(p.Tools, tool)
}

func checkPermissions(m map[string]string) error {
	for tool, v := range m {
		if v != "ask" && v != "deny" {
			return fmt.Errorf("%s: a profile or workspace can only require asking or deny, not %q", tool, v)
		}
	}
	return nil
}

// Validate checks a custom profile.
func (p Profile) Validate() error {
	if strings.TrimSpace(p.ID) == "" || strings.TrimSpace(p.Name) == "" {
		return errors.New("a profile needs a name")
	}
	for _, b := range BuiltIns() {
		if b.ID == p.ID {
			return errors.New("built-in profiles cannot be changed; make a custom one")
		}
	}
	if p.ContextLimit < 0 || p.MaxRounds < 0 {
		return errors.New("limits cannot be negative")
	}
	if p.Strategy != "" && p.Strategy != "sliding" && p.Strategy != "summary" && p.Strategy != "hybrid" {
		return errors.New("unknown context strategy")
	}
	return checkPermissions(p.Permissions)
}

// Validate checks a workspace.
func (w Workspace) Validate() error {
	if strings.TrimSpace(w.ID) == "" || strings.TrimSpace(w.Name) == "" {
		return errors.New("a workspace needs a name")
	}
	if !filepath.IsAbs(w.Path) {
		return errors.New("a workspace needs an absolute directory")
	}
	for _, pattern := range w.Ignore {
		if _, err := path.Match(pattern, "x"); err != nil {
			return fmt.Errorf("bad ignore pattern %q", pattern)
		}
	}
	return checkPermissions(w.Permissions)
}

// For returns the workspace governing dir: the one with the longest path that
// is dir or contains it.
func For(workspaces []Workspace, dir string) (Workspace, bool) {
	var best Workspace
	found := false
	for _, w := range workspaces {
		rel, err := filepath.Rel(filepath.Clean(w.Path), filepath.Clean(dir))
		if err == nil && filepath.IsLocal(rel) && len(w.Path) > len(best.Path) {
			best, found = w, true
		}
	}
	return best, found
}

// Ignored reports whether the slash-separated relative path, or a directory
// above it, matches a pattern. A pattern without a slash matches any name.
func Ignored(patterns []string, rel string) bool {
	if rel == "." || len(patterns) == 0 {
		return false
	}
	parts := strings.Split(rel, "/")
	for i := range parts {
		prefix := strings.Join(parts[:i+1], "/")
		for _, p := range patterns {
			p = strings.TrimSuffix(p, "/")
			subject := parts[i]
			if strings.Contains(p, "/") {
				subject = prefix
			}
			// Compared without case so a case-insensitive disk cannot be used
			// to reach an ignored file.
			if ok, _ := path.Match(strings.ToLower(p), strings.ToLower(subject)); ok {
				return true
			}
		}
	}
	return false
}
