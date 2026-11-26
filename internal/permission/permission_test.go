package permission

import "testing"

type memory struct{ grants []Grant }

func (m *memory) Load() ([]Grant, error) { return m.grants, nil }
func (m *memory) Save(g []Grant) error   { m.grants = g; return nil }

func newManager(t *testing.T, p Persistence) *Manager {
	t.Helper()
	m, err := NewManager(p)
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func cmd(c string) map[string]string { return map[string]string{"command": c} }

func TestSettingsOverrideGrants(t *testing.T) {
	m := newManager(t, nil)
	if err := m.Remember(ScopeSession, "/w", "shell", cmd("ls")); err != nil {
		t.Fatal(err)
	}
	if m.Check(SettingDeny, "/w", "shell", cmd("ls")) != Deny {
		t.Fatal("deny must win over a grant")
	}
	if m.Check(SettingAllow, "/w", "shell", cmd("rm")) != Allow {
		t.Fatal("allow setting ignored")
	}
	for _, setting := range []string{"", "ask", "garbage"} {
		if m.Check(setting, "/w", "shell", cmd("rm")) != Ask {
			t.Fatalf("setting %q should ask", setting)
		}
	}
}

func TestSensitiveGrantCoversOnlyTheExactCall(t *testing.T) {
	m := newManager(t, nil)
	if err := m.Remember(ScopeSession, "/w", "shell", cmd("ls")); err != nil {
		t.Fatal(err)
	}
	if m.Check("", "/w", "shell", cmd("ls")) != Allow {
		t.Fatal("same call should be allowed")
	}
	different := []struct {
		workspace, tool string
		args            map[string]string
	}{
		{"/w", "shell", cmd("ls; rm -rf .")},
		{"/w", "shell", map[string]string{"command": "ls", "dir": "sub"}},
		{"/other", "shell", cmd("ls")},
		{"/w", "evil__shell", cmd("ls")},
	}
	for _, d := range different {
		if m.Check("", d.workspace, d.tool, d.args) != Ask {
			t.Errorf("grant leaked to %+v", d)
		}
	}
}

func TestReadOnlyGrantCoversTool(t *testing.T) {
	m := newManager(t, nil)
	if err := m.Remember(ScopeSession, "/w", "read_file", map[string]string{"path": "a"}); err != nil {
		t.Fatal(err)
	}
	if m.Check("", "/w", "read_file", map[string]string{"path": "b"}) != Allow {
		t.Fatal("read-only grant should cover the tool")
	}
}

func TestPersistenceAndRevocation(t *testing.T) {
	store := &memory{}
	m := newManager(t, store)
	_ = m.Remember(ScopeOnce, "/w", "shell", cmd("once"))
	_ = m.Remember(ScopeSession, "/w", "shell", cmd("session"))
	_ = m.Remember(ScopeWorkspace, "/w", "shell", cmd("workspace"))
	if m.Check("", "/w", "shell", cmd("once")) != Ask {
		t.Fatal("once must not be remembered")
	}

	restarted := newManager(t, store)
	if restarted.Check("", "/w", "shell", cmd("session")) != Ask {
		t.Fatal("session grant survived a restart")
	}
	if restarted.Check("", "/w", "shell", cmd("workspace")) != Allow {
		t.Fatal("workspace grant lost")
	}
	if err := restarted.Revoke(restarted.List()[0].ID); err != nil {
		t.Fatal(err)
	}
	if restarted.Check("", "/w", "shell", cmd("workspace")) != Ask || len(store.grants) != 0 {
		t.Fatal("revocation did not take effect")
	}
}

func TestUnknownToolsAreSensitive(t *testing.T) {
	for _, tool := range []string{"shell", "write_file", "delete_file", "search_web", "db__query", "new_tool"} {
		if !Describe(tool, nil).Sensitive {
			t.Errorf("%s should be sensitive", tool)
		}
	}
}
