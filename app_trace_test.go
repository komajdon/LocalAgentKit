package main

import (
	"encoding/json"
	"strings"
	"testing"

	appLayer "agent-gui/app"
	"agent-gui/domain"
	"agent-gui/internal/config"
)

func TestTimelineAndDiagnosticsHideSecrets(t *testing.T) {
	p := &scripted{reply: `TOOL_CALL: {"tool":"nope","args":{}}` + "\n" +
		`TOOL_CALL: {"tool":"get_time","args":{"password":"hunter2hunter2"}}`}
	a, _ := testApp(t, p)
	a.cfg.APIKey = "sk-abcdefghijklmnopqrstuvwxyz"
	a.cfg.BaseURL = "http://user:pw123456@internal.example"
	a.cfg.ToolPermissions = map[string]config.ToolPerm{"get_time": config.PermAllow}
	id := newConv(t, a)
	wait(t, send(t, a, id, "PRIVATE-PROMPT").done)

	records, err := a.GetToolTimeline(id)
	if err != nil || len(records) != 2 {
		t.Fatalf("records = %+v %v", records, err)
	}
	byTool := map[string]string{}
	for _, r := range records {
		byTool[r.Tool] = r.Status
		if strings.Contains(r.Args, "hunter2") {
			t.Fatalf("secret in timeline: %s", r.Args)
		}
	}
	if byTool["nope"] != appLayer.ToolRejected || byTool["get_time"] != appLayer.ToolSucceeded {
		t.Fatalf("statuses = %v", byTool)
	}

	data, err := json.Marshal(a.diagnostics())
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"sk-abc", "pw123456", "internal.example", "PRIVATE-PROMPT", "hunter2"} {
		if strings.Contains(string(data), secret) {
			t.Errorf("diagnostics contain %q", secret)
		}
	}
}

func TestSafeArgs(t *testing.T) {
	got := safeArgs(domain.Args{"api_key": "abc", "url": "https://x/?token=zzz999", "path": "a.txt"})
	if strings.Contains(got, "abc") || strings.Contains(got, "zzz999") || !strings.Contains(got, "a.txt") {
		t.Fatal(got)
	}
}
