package main

import (
	"errors"
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/config"
)

type lister struct {
	scripted
	calls int
	err   error
}

func (l *lister) ListModels() ([]string, error) {
	l.calls++
	return []string{"a"}, l.err
}

func TestModelListIsCachedAndRefreshable(t *testing.T) {
	l := &lister{}
	a, _ := testApp(t, l)
	a.ListModels()
	if got := a.ListModels(); l.calls != 1 || len(got.Models) != 1 {
		t.Fatalf("calls=%d %+v", l.calls, got)
	}
	a.RefreshModels()
	l.err = errors.New("offline")
	if got := a.RefreshModels(); l.calls != 3 || got.Error == "" || len(got.Models) != 1 {
		t.Fatalf("stale list with error expected: %+v", got)
	}
	if r := a.TestConnection(); r.OK || r.Error != "offline" {
		t.Fatalf("connection = %+v", r)
	}
}

func TestEffectiveModelProfile(t *testing.T) {
	cfg := config.Default() // 8192, auto
	no, yes := false, true
	for name, tc := range map[string]struct {
		detected domain.Capabilities
		override ModelOverride
		limit    int
		tools    string
	}{
		"nothing known":   {domain.Capabilities{}, ModelOverride{}, 8192, "auto"},
		"small window":    {domain.Capabilities{ContextWindow: 4096}, ModelOverride{}, 4096, "auto"},
		"large window":    {domain.Capabilities{ContextWindow: 200000, NativeTools: &yes}, ModelOverride{}, 8192, "auto"},
		"no native tools": {domain.Capabilities{NativeTools: &no}, ModelOverride{}, 8192, "text"},
		"override wins":   {domain.Capabilities{ContextWindow: 4096, NativeTools: &no}, ModelOverride{32000, "native"}, 32000, "native"},
	} {
		p := effective(cfg, "m", tc.detected, tc.override)
		if p.ContextLimit != tc.limit || p.ToolCalling != tc.tools {
			t.Errorf("%s: %d %s", name, p.ContextLimit, p.ToolCalling)
		}
	}
}

func TestModelSettingsValidation(t *testing.T) {
	a, _ := testApp(t, &scripted{})
	if a.SaveModelSettings(ModelSettings{TimeoutSeconds: -1}) == "" {
		t.Fatal("negative timeout accepted")
	}
	if a.SaveModelSettings(ModelSettings{Overrides: map[string]ModelOverride{"p/m": {ToolCalling: "magic"}}}) == "" {
		t.Fatal("bad override accepted")
	}
	if msg := a.SaveModelSettings(ModelSettings{Overrides: map[string]ModelOverride{"p/m": {}, "p/n": {ContextLimit: 9}}}); msg != "" {
		t.Fatal(msg)
	}
	if got := a.GetModelSettings().Overrides; len(got) != 1 || got["p/n"].ContextLimit != 9 {
		t.Fatalf("overrides = %v", got)
	}
}
