package usage

import (
	"math"
	"strings"
	"testing"
	"time"
)

type memory map[string]Entry

func (m memory) Get(k string) (Entry, bool, error) { e, ok := m[k]; return e, ok, nil }
func (m memory) Put(k string, e Entry) error       { m[k] = e; return nil }
func (m memory) All() ([]Entry, error) {
	var out []Entry
	for _, e := range m {
		out = append(out, e)
	}
	return out, nil
}

func priced() Settings {
	s := DefaultSettings()
	s.Prices = []Price{
		{Provider: "openai", Model: "big", Input: 10, Output: 30, Cached: 1},
		{Provider: "openai", Model: "*", Input: 1, Output: 2},
	}
	return s
}

func TestMeasureUsesOnlyEnteredPrices(t *testing.T) {
	s := priced()
	sample := Sample{Input: 1_000_000, Output: 500_000, Cached: 400_000}
	if got := s.Measure("openai", "big", sample); math.Abs(got.Cost-(6+0.4+15)) > 1e-9 || got.UnpricedRequests != 0 {
		t.Fatalf("big = %+v", got)
	}
	if got := s.Measure("openai", "other", sample); math.Abs(got.Cost-2) > 1e-9 {
		t.Fatalf("wildcard, cached at input price = %+v", got)
	}
	got := s.Measure("ollama", "llama", Sample{Input: 5, Estimated: true})
	if got.Cost != 0 || got.UnpricedRequests != 1 || got.EstimatedRequests != 1 {
		t.Fatalf("unpriced = %+v", got)
	}
}

func TestCheckLimits(t *testing.T) {
	s := priced()
	used := Totals{Requests: 3, Input: 800, Output: 100, Cost: 0.9}
	for name, tc := range map[string]struct {
		limits Limits
		priced bool
		err    string
		warn   string
	}{
		"none":                {Limits{}, false, "", ""},
		"run tokens":          {Limits{RunTokens: 900}, true, "tokens per run", ""},
		"run requests":        {Limits{RunRequests: 3}, true, "requests per run", ""},
		"run cost":            {Limits{RunCost: 0.5}, true, "cost limit per run", ""},
		"unpriced cost limit": {Limits{RunCost: 5}, false, "no price", ""},
		"soft daily":          {Limits{DailyTokens: 500}, true, "", "limit (500 tokens) was reached"},
		"hard daily":          {Limits{DailyTokens: 500, HardPeriodLimits: true}, true, "daily token", ""},
		"hard monthly cost":   {Limits{MonthlyCost: 0.9, HardPeriodLimits: true}, true, "monthly cost", ""},
		"warning":             {Limits{DailyCost: 1, WarnPercent: 80}, true, "", "90% of the daily cost"},
		"below warning":       {Limits{DailyCost: 10, WarnPercent: 80}, true, "", ""},
	} {
		s.Limits = tc.limits
		warnings, err := s.Check(used, used, used, tc.priced)
		if (err == nil) != (tc.err == "") || (err != nil && !strings.Contains(err.Error(), tc.err)) {
			t.Errorf("%s: err = %v", name, err)
		}
		if got := strings.Join(warnings, ";"); (tc.warn == "") != (got == "") || !strings.Contains(got, tc.warn) {
			t.Errorf("%s: warnings = %q", name, got)
		}
	}
}

func TestLedgerAggregatesAndFilters(t *testing.T) {
	l := NewLedger(memory{})
	add := func(day, model, conv string, in int) {
		if err := l.Record(Key{day, "p", model, conv}, Totals{Requests: 1, Input: in}); err != nil {
			t.Fatal(err)
		}
	}
	add("2026-10-31", "a", "c1", 1)
	add("2026-11-01", "a", "c1", 10)
	add("2026-11-26", "a", "c1", 100)
	add("2026-11-26", "a", "c1", 100)
	add("2026-11-26", "b", "c2", 1000)

	r, err := l.Query(Filter{})
	if err != nil || len(r.Entries) != 4 || r.Total.Input != 1211 || r.Entries[0].Day != "2026-11-26" {
		t.Fatalf("all = %+v %v", r, err)
	}
	for f, want := range map[Filter]int{
		{Model: "a"}: 211, {ConversationID: "c2"}: 1000, {From: "2026-11-01", To: "2026-11-01"}: 10, {Provider: "x"}: 0,
	} {
		if got, _ := l.Query(f); got.Total.Input != want {
			t.Errorf("%+v = %d, want %d", f, got.Total.Input, want)
		}
	}
	day, month, err := l.Periods(time.Date(2026, 11, 26, 12, 0, 0, 0, time.Local))
	if err != nil || day.Input != 1200 || month.Input != 1210 {
		t.Fatalf("day=%d month=%d %v", day.Input, month.Input, err)
	}
}
