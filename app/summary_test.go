package app

import (
	"context"
	"errors"
	"strings"
	"testing"

	"agent-gui/domain"
	"agent-gui/internal/testutil"
)

// filled returns an agent whose history is several times its context limit.
func filled(p domain.LLMProvider, strategy Strategy) *ConversationalAgent {
	probe := limited(p, 0)
	a := limited(p, (len(probe.conv.SystemPrompt)/4+1500)*4/3)
	a.Strategy = strategy
	for _, q := range []string{"one", "two", "three", "four", "five", "six"} {
		a.InjectMessage(domain.RoleUser, q+" "+strings.Repeat("x", 1600))
		a.InjectMessage(domain.RoleAssistant, "answer "+q)
	}
	return a
}

func userTurns(msgs []domain.Message) int {
	n := 0
	for _, m := range msgs {
		if startsTurn(m) && !strings.HasPrefix(m.Content, "<conversation-summary>") {
			n++
		}
	}
	return n
}

func TestSummaryStrategies(t *testing.T) {
	for _, strategy := range []Strategy{StrategySummary, StrategyHybrid} {
		t.Run(string(strategy), func(t *testing.T) {
			p := &testutil.FakeProvider{Replies: []testutil.Reply{reply("THE SUMMARY"), reply("ok")}}
			a := filled(p, strategy)
			history := len(a.Messages())
			if err := a.Chat(context.Background(), "next"); err != nil {
				t.Fatal(err)
			}
			if len(p.Requests) != 2 || p.Requests[0][0].Content != summaryInstruction {
				t.Fatalf("requests = %d", len(p.Requests))
			}
			sent := p.Requests[1]
			if !strings.HasPrefix(sent[0].Content, "RULES") || strings.Contains(sent[0].Content, "THE SUMMARY") {
				t.Fatal("the summary must not enter the system prompt")
			}
			if sent[1].Role != domain.RoleUser || !strings.Contains(sent[1].Content, "THE SUMMARY") {
				t.Fatalf("summary missing: %.80q", sent[1].Content)
			}
			if got := userTurns(sent); strategy == StrategySummary && got != keepForSummary || got < 1 {
				t.Fatalf("verbatim turns = %d", got)
			}
			if a.Summary == nil || a.Summary.Covered == 0 || len(a.Messages()) != history+2 {
				t.Fatalf("summary=%+v history=%d", a.Summary, len(a.Messages()))
			}
			if u := a.CurrentUsage(); u.Summarised != a.Summary.Covered {
				t.Fatalf("usage = %+v", u)
			}
		})
	}
}

func TestFailedSummaryFallsBackAndWarns(t *testing.T) {
	p := &testutil.FakeProvider{Replies: []testutil.Reply{{Err: errors.New("boom")}, reply("ok")}}
	a := filled(p, StrategyHybrid)
	var warnings []string
	a.OnWarning = func(w string) { warnings = append(warnings, w) }
	if err := a.Chat(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	if a.Summary != nil || len(warnings) != 1 || a.CurrentUsage().Omitted == 0 {
		t.Fatalf("summary=%v warnings=%v", a.Summary, warnings)
	}
}

func TestSlidingNeverSummarisesAndStaleSummaryIsIgnored(t *testing.T) {
	p := &testutil.FakeProvider{Replies: []testutil.Reply{reply("ok")}}
	a := filled(p, StrategySliding)
	a.Summary = &Summary{Text: "old", Covered: 4}
	if err := a.Chat(context.Background(), "next"); err != nil {
		t.Fatal(err)
	}
	if len(p.Requests) != 1 || strings.Contains(p.Requests[0][1].Content, "old") {
		t.Fatal("sliding window used a summary")
	}
	a.Strategy = StrategyHybrid
	a.Summary = &Summary{Text: "beyond history", Covered: 9999}
	if s, _ := a.plan(); s != nil {
		t.Fatal("a summary of messages that no longer exist was used")
	}
}

func TestTokenEstimateIsCalibrated(t *testing.T) {
	a := limited(&testutil.FakeProvider{}, 0)
	msgs := []domain.Message{{Content: strings.Repeat("a", 1200)}}
	if a.tokens(msgs) != 300 {
		t.Fatal(a.tokens(msgs))
	}
	a.calibrate(msgs, 600)
	if a.tokens(msgs) != 600 {
		t.Fatal(a.tokens(msgs))
	}
	a.calibrate(msgs, 1_000_000) // implausible: clamped
	if a.tokens(msgs) != 800 {
		t.Fatal(a.tokens(msgs))
	}
}
