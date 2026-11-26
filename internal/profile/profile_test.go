package profile

import "testing"

func TestStricterNeverLoosens(t *testing.T) {
	for _, tc := range []struct {
		global, restriction, want string
	}{
		{"", "", "ask"}, {"", "allow", "ask"}, {"allow", "", "allow"}, {"allow", "ask", "ask"},
		{"allow", "deny", "deny"}, {"deny", "allow", "deny"}, {"deny", "ask", "deny"}, {"ask", "bogus", "ask"},
	} {
		if got := Stricter(tc.global, tc.restriction); got != tc.want {
			t.Errorf("Stricter(%q,%q) = %q, want %q", tc.global, tc.restriction, got, tc.want)
		}
	}
	if Lower(0, 5) != 5 || Lower(5, 0) != 5 || Lower(5, 9) != 5 || Lower(9, 5) != 5 {
		t.Error("Lower")
	}
}

func TestValidation(t *testing.T) {
	if (Profile{ID: "x", Name: "X", Permissions: map[string]string{"shell": "allow"}}).Validate() == nil {
		t.Error("a profile granted allow")
	}
	if (Profile{ID: "readonly", Name: "mine"}).Validate() == nil {
		t.Error("a built-in was replaced")
	}
	if (Workspace{ID: "w", Name: "W", Path: "relative"}).Validate() == nil {
		t.Error("relative path")
	}
	if (Workspace{ID: "w", Name: "W", Path: "/w", Ignore: []string{"["}}).Validate() == nil {
		t.Error("bad pattern")
	}
	for _, p := range BuiltIns() {
		if p.ID == "readonly" && (p.Allows("shell") || p.Allows("write_file") || p.Allows("db__query") || !p.Allows("read_file")) {
			t.Error("read-only profile tools")
		}
	}
}

func TestForAndIgnored(t *testing.T) {
	list := []Workspace{{ID: "a", Path: "/w"}, {ID: "b", Path: "/w/sub"}}
	for dir, want := range map[string]string{"/w": "a", "/w/x": "a", "/w/sub/deep": "b", "/wx": "", "/": ""} {
		got, ok := For(list, dir)
		if got.ID != want || ok != (want != "") {
			t.Errorf("For(%q) = %q", dir, got.ID)
		}
	}
	patterns := []string{".env", "*.pem", "secrets/", "build/out"}
	for rel, want := range map[string]bool{
		".env": true, "a/.ENV": true, "k.pem": true, "secrets/x": true, "a/secrets/b/c": true,
		"build/out/f": true, "other/build/out": false, "env": false, "src/main.go": false, ".": false,
	} {
		if Ignored(patterns, rel) != want {
			t.Errorf("Ignored(%q) = %v", rel, !want)
		}
	}
}
