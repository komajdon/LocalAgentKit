package main

import (
	"strings"
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/usage"
)

func TestRunsAreRecordedAndLimited(t *testing.T) {
	g := newGated()
	close(g.release)
	a, s := testApp(t, g)
	id := newConv(t, a)
	wait(t, send(t, a, id, "q").done)

	r, err := a.GetUsageReport(usage.Filter{ConversationID: id})
	if err != nil || r.Total.Requests != 1 || r.Total.Input != 5 || r.Total.Output != 2 || r.Total.UnpricedRequests != 1 {
		t.Fatalf("report = %+v %v", r.Total, err)
	}

	set := usage.DefaultSettings()
	set.Limits = usage.Limits{DailyTokens: 1, HardPeriodLimits: true}
	if msg := a.SaveUsageSettings(set); msg != "" {
		t.Fatal(msg)
	}
	defer a.SaveUsageSettings(usage.DefaultSettings())
	wait(t, send(t, a, id, "again").done)
	events := s.all()
	last := events[len(events)-1]
	if msg, _ := last.ev.Data.(string); last.name != "chat:error" || !strings.Contains(msg, "daily token limit") {
		t.Fatalf("event %s %v", last.name, last.ev.Data)
	}
	if after, _ := a.GetUsageReport(usage.Filter{ConversationID: id}); after.Total.Requests != 1 {
		t.Fatal("a blocked request was made")
	}
	if got := stored(t, id).Messages; got[len(got)-1].Role != string(domain.RoleUser) {
		t.Fatal("the blocked input was not kept")
	}

	set.Limits = usage.Limits{RunTokens: -1}
	if a.SaveUsageSettings(set) == "" {
		t.Fatal("negative limit accepted")
	}
}
