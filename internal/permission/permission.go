// Package permission decides whether a tool call may run.
//
// A decision comes from, in order: a global "deny" setting, a global "allow"
// setting, a stored approval that matches the call, and otherwise the user.
//
// Approvals are scoped to one workspace. For sensitive tools an approval
// covers only the exact same arguments, so approving one command or file
// change never authorizes a different one. For read-only tools, which are
// already confined to the workspace, it covers the tool.
package permission

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"

	"agent-gui/domain"
)

// Scope is how long an approval lasts.
type Scope string

const (
	ScopeOnce      Scope = "once"
	ScopeSession   Scope = "session"   // until the app exits
	ScopeWorkspace Scope = "workspace" // persisted
)

// Decision is the outcome of Check.
type Decision int

const (
	Ask Decision = iota
	Allow
	Deny
)

// Setting values of the global per-tool permission.
const (
	SettingAllow = "allow"
	SettingDeny  = "deny"
)

// readOnly tools neither change anything nor send data off the machine. Every
// other tool, including all MCP tools and any added later, is sensitive.
var readOnly = map[string]bool{
	"read_file": true, "list_dir": true, "search_files": true,
	"read_document": true, "get_time": true,
}

// Details describes a call for the approval dialog.
type Details struct {
	Operation string `json:"operation"`
	Target    string `json:"target"`
	Impact    string `json:"impact"`
	Sensitive bool   `json:"sensitive"`
	Scope     string `json:"scope"` // what a lasting approval would cover
}

// Describe summarizes what a call would do.
func Describe(tool string, args domain.Args) Details {
	d := Details{Sensitive: !readOnly[tool]}
	switch tool {
	case "shell":
		d.Operation, d.Target = "Run a shell command", args.String("command")
		d.Impact = "Can change or delete files and run any program the sandbox allows."
	case "write_file":
		d.Operation, d.Target = "Write a file", args.String("path")
		d.Impact = fmt.Sprintf("Creates or overwrites the file with %d bytes.", len(args.String("content")))
	case "delete_file":
		d.Operation, d.Target = "Delete a file", args.String("path")
		d.Impact = "Permanently removes the file."
	case "search_web":
		d.Operation, d.Target = "Search the web", args.String("query")
		d.Impact = "Sends this query to an external search service."
	case "read_file", "read_document":
		d.Operation, d.Target = "Read a file", args.String("path")
		d.Impact = "The file's contents are sent to the model provider."
	case "list_dir":
		d.Operation, d.Target = "List a directory", args.String("path")
		d.Impact = "File names are sent to the model provider."
	case "search_files":
		d.Operation, d.Target = "Search files", args.String("pattern")
		d.Impact = "Matching lines are sent to the model provider."
	case "get_time":
		d.Operation, d.Impact = "Read the clock", "None."
	default:
		d.Operation = "Call an external tool"
		d.Target = tool
		d.Impact = "Effects depend on the external server and cannot be checked by this app."
	}
	if d.Sensitive {
		d.Scope = "only this exact request in this workspace"
	} else {
		d.Scope = "every use of " + tool + " in this workspace"
	}
	return d
}

// Grant is a remembered approval.
type Grant struct {
	ID        string `json:"id"`
	Workspace string `json:"workspace"`
	Tool      string `json:"tool"`
	Label     string `json:"label"`
	Scope     Scope  `json:"scope"`
}

// grantID identifies the calls that one approval covers.
func grantID(workspace, tool string, args domain.Args) string {
	key := []any{workspace, tool}
	if !readOnly[tool] {
		key = append(key, args) // encoding/json sorts map keys
	}
	// Arguments came from JSON, so marshalling them cannot fail.
	data, _ := json.Marshal(key)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// Persistence stores workspace-scoped grants.
type Persistence interface {
	Load() ([]Grant, error)
	Save([]Grant) error
}

// Manager remembers approvals. It is safe for concurrent use.
type Manager struct {
	mu     sync.Mutex
	grants map[string]Grant
	store  Persistence
}

// NewManager loads persisted grants. A load failure leaves none, so the user
// is asked again; it is also returned.
func NewManager(store Persistence) (*Manager, error) {
	m := &Manager{grants: map[string]Grant{}, store: store}
	if store == nil {
		return m, nil
	}
	saved, err := store.Load()
	for _, g := range saved {
		if g.Scope == ScopeWorkspace {
			m.grants[g.ID] = g
		}
	}
	return m, err
}

// Check decides a call given the global setting for the tool. When
// externalContent is true the conversation holds text someone else may have
// written, so a blanket "allow" no longer covers sensitive tools; they need an
// approval of the exact call.
func (m *Manager) Check(setting, workspace, tool string, args domain.Args, externalContent bool) Decision {
	switch {
	case setting == SettingDeny:
		return Deny
	case setting == SettingAllow && (readOnly[tool] || !externalContent):
		return Allow
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.grants[grantID(workspace, tool, args)]; ok {
		return Allow
	}
	return Ask
}

// Remember records an approval. ScopeOnce records nothing.
func (m *Manager) Remember(scope Scope, workspace, tool string, args domain.Args) error {
	if scope != ScopeSession && scope != ScopeWorkspace {
		return nil
	}
	label := Describe(tool, args).Target
	if readOnly[tool] {
		label = "all uses"
	}
	label = strings.Join(strings.Fields(label), " ")
	if r := []rune(label); len(r) > 120 {
		label = string(r[:120]) + "…"
	}
	g := Grant{ID: grantID(workspace, tool, args), Workspace: workspace, Tool: tool, Label: label, Scope: scope}

	m.mu.Lock()
	defer m.mu.Unlock()
	m.grants[g.ID] = g
	return m.saveLocked()
}

// List returns the remembered approvals in a stable order.
func (m *Manager) List() []Grant {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.listLocked()
}

func (m *Manager) listLocked() []Grant {
	out := make([]Grant, 0, len(m.grants))
	for _, g := range m.grants {
		out = append(out, g)
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := out[i], out[j]
		return a.Workspace+"\x00"+a.Tool+"\x00"+a.ID < b.Workspace+"\x00"+b.Tool+"\x00"+b.ID
	})
	return out
}

// Revoke removes one approval.
func (m *Manager) Revoke(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.grants, id)
	return m.saveLocked()
}

// RevokeAll removes every approval.
func (m *Manager) RevokeAll() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.grants = map[string]Grant{}
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	if m.store == nil {
		return nil
	}
	saved := []Grant{}
	for _, g := range m.listLocked() {
		if g.Scope == ScopeWorkspace {
			saved = append(saved, g)
		}
	}
	return m.store.Save(saved)
}
