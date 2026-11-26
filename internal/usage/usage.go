// Package usage records what model requests consumed and enforces limits.
//
// Token counts come from the provider when it reports them and are otherwise
// estimated, and the two are counted separately. Money figures are always
// computed from prices the user entered: no provider reports what it billed,
// and no price is ever assumed.
package usage

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Sample is the consumption of one model request.
type Sample struct {
	Input     int  `json:"input"`
	Output    int  `json:"output"`
	Cached    int  `json:"cached"` // part of Input served from the provider's cache
	Estimated bool `json:"estimated"`
}

// Key identifies one ledger row.
type Key struct {
	Day            string `json:"day"` // local date, YYYY-MM-DD
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	ConversationID string `json:"conversation_id"`
}

func (k Key) String() string {
	return strings.Join([]string{k.Day, k.Provider, k.Model, k.ConversationID}, "\x1f")
}

// Totals accumulates samples.
type Totals struct {
	Requests          int     `json:"requests"`
	EstimatedRequests int     `json:"estimated_requests"`
	Input             int     `json:"input"`
	Output            int     `json:"output"`
	Cached            int     `json:"cached"`
	Cost              float64 `json:"cost"`
	UnpricedRequests  int     `json:"unpriced_requests"` // not included in Cost
}

// Tokens is input plus output.
func (t Totals) Tokens() int { return t.Input + t.Output }

// Add merges other into t.
func (t *Totals) Add(o Totals) {
	t.Requests += o.Requests
	t.EstimatedRequests += o.EstimatedRequests
	t.Input += o.Input
	t.Output += o.Output
	t.Cached += o.Cached
	t.Cost += o.Cost
	t.UnpricedRequests += o.UnpricedRequests
}

// Entry is a ledger row.
type Entry struct {
	Key
	Totals
}

// Price is what a model costs per million tokens. Model "*" covers the other
// models of a provider.
type Price struct {
	Provider string  `json:"provider"`
	Model    string  `json:"model"`
	Input    float64 `json:"input_per_million"`
	Output   float64 `json:"output_per_million"`
	Cached   float64 `json:"cached_per_million"`
}

// Limits stop or warn about consumption. Zero means no limit.
type Limits struct {
	RunTokens     int     `json:"run_tokens"`
	RunRequests   int     `json:"run_requests"`
	RunCost       float64 `json:"run_cost"`
	DailyTokens   int     `json:"daily_tokens"`
	DailyCost     float64 `json:"daily_cost"`
	MonthlyTokens int     `json:"monthly_tokens"`
	MonthlyCost   float64 `json:"monthly_cost"`
	// WarnPercent of a daily or monthly limit triggers a warning.
	WarnPercent int `json:"warn_percent"`
	// HardPeriodLimits makes daily and monthly limits stop requests rather
	// than only warn. Per-run limits always stop the run.
	HardPeriodLimits bool `json:"hard_period_limits"`
}

// Settings holds the user's prices and limits.
type Settings struct {
	Currency string  `json:"currency"`
	Prices   []Price `json:"prices"`
	Limits   Limits  `json:"limits"`
}

// DefaultSettings warns at 80% and sets no limits.
func DefaultSettings() Settings {
	return Settings{Currency: "USD", Prices: []Price{}, Limits: Limits{WarnPercent: 80}}
}

// PriceFor finds the user's price for a model.
func (s Settings) PriceFor(provider, model string) (Price, bool) {
	var fallback Price
	found := false
	for _, p := range s.Prices {
		if p.Provider != provider {
			continue
		}
		if p.Model == model {
			return p, true
		}
		if p.Model == "*" {
			fallback, found = p, true
		}
	}
	return fallback, found
}

// Measure turns a sample into totals, pricing it if possible.
func (s Settings) Measure(provider, model string, sample Sample) Totals {
	t := Totals{Requests: 1, Input: sample.Input, Output: sample.Output, Cached: sample.Cached}
	if sample.Estimated {
		t.EstimatedRequests = 1
	}
	price, ok := s.PriceFor(provider, model)
	if !ok {
		t.UnpricedRequests = 1
		return t
	}
	cached := min(max(sample.Cached, 0), sample.Input)
	cachedPrice := price.Cached
	if cachedPrice == 0 {
		cachedPrice = price.Input
	}
	t.Cost = (float64(sample.Input-cached)*price.Input + float64(cached)*cachedPrice + float64(sample.Output)*price.Output) / 1e6
	return t
}

// Check decides whether another request may be made, given what the run, the
// day and the month have consumed. priced says whether the model has a price.
func (s Settings) Check(run, day, month Totals, priced bool) (warnings []string, err error) {
	l := s.Limits
	if !priced && (l.RunCost > 0 || (l.HardPeriodLimits && (l.DailyCost > 0 || l.MonthlyCost > 0))) {
		return nil, fmt.Errorf("a cost limit is set but this model has no price, so it cannot be enforced; add a price (0 for a free model) in Usage")
	}
	switch {
	case l.RunTokens > 0 && run.Tokens() >= l.RunTokens:
		return nil, fmt.Errorf("the limit of %d tokens per run was reached", l.RunTokens)
	case l.RunRequests > 0 && run.Requests >= l.RunRequests:
		return nil, fmt.Errorf("the limit of %d requests per run was reached", l.RunRequests)
	case l.RunCost > 0 && run.Cost >= l.RunCost:
		return nil, fmt.Errorf("the cost limit per run (%g %s) was reached", l.RunCost, s.Currency)
	}
	periods := []struct {
		name        string
		used, limit float64
		unit        string
	}{
		{"daily token", float64(day.Tokens()), float64(l.DailyTokens), "tokens"},
		{"daily cost", day.Cost, l.DailyCost, s.Currency},
		{"monthly token", float64(month.Tokens()), float64(l.MonthlyTokens), "tokens"},
		{"monthly cost", month.Cost, l.MonthlyCost, s.Currency},
	}
	for _, p := range periods {
		if p.limit <= 0 {
			continue
		}
		if p.used >= p.limit {
			msg := fmt.Sprintf("the %s limit (%g %s) was reached", p.name, p.limit, p.unit)
			if l.HardPeriodLimits {
				return nil, fmt.Errorf("%s", msg)
			}
			warnings = append(warnings, msg)
		} else if l.WarnPercent > 0 && p.used >= p.limit*float64(l.WarnPercent)/100 {
			warnings = append(warnings, fmt.Sprintf("%.0f%% of the %s limit is used", p.used/p.limit*100, p.name))
		}
	}
	return warnings, nil
}

// Storage persists ledger rows.
type Storage interface {
	Get(key string) (Entry, bool, error)
	Put(key string, e Entry) error
	All() ([]Entry, error)
}

// Ledger records consumption. It is safe for concurrent use.
type Ledger struct {
	mu      sync.Mutex
	storage Storage
}

// NewLedger returns a ledger over storage.
func NewLedger(storage Storage) *Ledger { return &Ledger{storage: storage} }

// Day formats the ledger date of t.
func Day(t time.Time) string { return t.Format("2006-01-02") }

// Record adds totals to a row.
func (l *Ledger) Record(key Key, totals Totals) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok, err := l.storage.Get(key.String())
	if err != nil {
		return err
	}
	if !ok {
		e = Entry{Key: key}
	}
	e.Add(totals)
	return l.storage.Put(key.String(), e)
}

// Filter selects rows. Empty fields match everything; From and To are
// inclusive dates.
type Filter struct {
	Provider       string `json:"provider"`
	Model          string `json:"model"`
	ConversationID string `json:"conversation_id"`
	From           string `json:"from"`
	To             string `json:"to"`
}

func (f Filter) matches(k Key) bool {
	return (f.Provider == "" || f.Provider == k.Provider) &&
		(f.Model == "" || f.Model == k.Model) &&
		(f.ConversationID == "" || f.ConversationID == k.ConversationID) &&
		(f.From == "" || k.Day >= f.From) &&
		(f.To == "" || k.Day <= f.To)
}

// Report is the rows matching a filter and their sum.
type Report struct {
	Entries []Entry `json:"entries"`
	Total   Totals  `json:"total"`
}

// Query returns the matching rows, newest day first.
func (l *Ledger) Query(f Filter) (Report, error) {
	all, err := l.storage.All()
	if err != nil {
		return Report{Entries: []Entry{}}, err
	}
	r := Report{Entries: []Entry{}}
	for _, e := range all {
		if f.matches(e.Key) {
			r.Entries = append(r.Entries, e)
			r.Total.Add(e.Totals)
		}
	}
	sort.Slice(r.Entries, func(i, j int) bool {
		a, b := r.Entries[i].Key, r.Entries[j].Key
		if a.Day != b.Day {
			return a.Day > b.Day
		}
		return a.String() < b.String()
	})
	return r, nil
}

// Periods returns what has been consumed on now's day and in its month.
func (l *Ledger) Periods(now time.Time) (day, month Totals, err error) {
	today := Day(now)
	r, err := l.Query(Filter{From: today[:8] + "01", To: today[:8] + "31"})
	if err != nil {
		return day, month, err
	}
	for _, e := range r.Entries {
		if e.Day == today {
			day.Add(e.Totals)
		}
	}
	return day, r.Total, nil
}
