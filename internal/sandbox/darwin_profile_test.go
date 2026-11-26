package sandbox

import (
	"slices"
	"strings"
	"testing"
)

func TestDarwinProfileRules(t *testing.T) {
	args := darwinProfile("/w", "/Users/u", "/s", []string{`/w/app "data"`}, false)
	profile := args[len(args)-1]
	// Order matters: each allow must follow the deny it carves an exception from,
	// and the protected directories must follow every allow.
	order := []string{
		"(deny file-write*)", "(allow file-write*", "(deny file-read*", `(allow file-read* (subpath (param "WORK"))`,
		"(allow file-read-metadata)", `(deny file-read* file-write* (subpath (param "HIDDEN0")))`, "(deny network*)",
	}
	last := -1
	for _, rule := range order {
		i := strings.Index(profile, rule)
		if i <= last {
			t.Fatalf("rule %q missing or out of order:\n%s", rule, profile)
		}
		last = i
	}
	writable := profile[strings.Index(profile, "(allow file-write*"):strings.Index(profile, "(deny file-read*")]
	if strings.Contains(writable, "/private") {
		t.Errorf("shared temporary directories must not be writable: %s", writable)
	}
	for _, dir := range deniedOnDarwin {
		if !strings.Contains(profile, `(subpath "`+dir+`")`) {
			t.Errorf("%s is not denied", dir)
		}
	}
	// Paths are parameters, never profile text.
	if strings.Contains(profile, "app ") || !slices.Contains(args, `HIDDEN0=/w/app "data"`) {
		t.Error("a path was spliced into the profile")
	}
	if p := darwinProfile("/w", "/h", "/s", nil, true); strings.Contains(p[len(p)-1], "network") {
		t.Error("network denied although allowed")
	}
}

func TestDarwinEnvMovesHomeOnlyWhenHidden(t *testing.T) {
	if got := darwinEnv("/s", false); !slices.Equal(got, []string{"/usr/bin/env", "TMPDIR=/s", "HOME=/s"}) {
		t.Errorf("hidden home: %v", got)
	}
	if got := darwinEnv("/s", true); !slices.Equal(got, []string{"/usr/bin/env", "TMPDIR=/s"}) {
		t.Errorf("visible home: %v", got)
	}
}
