package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	appLayer "agent-gui/app"
	"agent-gui/internal/store"
	"agent-gui/internal/usage"
)

const (
	usageBucket         = "usage"
	usageSettingsBucket = "usage_settings"
	usageSettingsKey    = "settings"
)

// usageStore keeps the ledger in the encrypted database.
type usageStore struct{}

func (usageStore) Get(key string) (usage.Entry, bool, error) {
	var e usage.Entry
	if !store.Has(usageBucket, key) {
		return e, false, nil
	}
	err := store.Get(usageBucket, key, &e)
	return e, err == nil, err
}

func (usageStore) Put(key string, e usage.Entry) error { return store.Put(usageBucket, key, e) }

func (usageStore) All() ([]usage.Entry, error) {
	var out []usage.Entry
	err := store.Scan(usageBucket, func(_ string, raw []byte) error {
		var e usage.Entry
		if err := json.Unmarshal(raw, &e); err == nil {
			out = append(out, e)
		}
		return nil
	})
	return out, err
}

// GetUsageSettings returns the user's prices and limits.
func (a *App) GetUsageSettings() usage.Settings {
	s := usage.DefaultSettings()
	if store.Has(usageSettingsBucket, usageSettingsKey) {
		if err := store.Get(usageSettingsBucket, usageSettingsKey, &s); err != nil {
			a.logf("could not read usage settings: %v", err)
		}
	}
	if s.Prices == nil {
		s.Prices = []usage.Price{}
	}
	return s
}

// SaveUsageSettings validates and stores prices and limits. It returns an
// error message or "".
func (a *App) SaveUsageSettings(s usage.Settings) string {
	l := s.Limits
	for _, v := range []float64{float64(l.RunTokens), float64(l.RunRequests), l.RunCost, float64(l.DailyTokens),
		l.DailyCost, float64(l.MonthlyTokens), l.MonthlyCost, float64(l.WarnPercent)} {
		if v < 0 {
			return "limits cannot be negative"
		}
	}
	for _, p := range s.Prices {
		if strings.TrimSpace(p.Provider) == "" || strings.TrimSpace(p.Model) == "" {
			return "every price needs a provider and a model (or * for all its models)"
		}
		if p.Input < 0 || p.Output < 0 || p.Cached < 0 {
			return "prices cannot be negative"
		}
	}
	return errString(store.Put(usageSettingsBucket, usageSettingsKey, s))
}

// GetUsageReport returns recorded usage matching the filter.
func (a *App) GetUsageReport(f usage.Filter) (usage.Report, error) { return a.ledger.Query(f) }

// meter applies limits to one run and records what it consumes.
type meter struct {
	app                     *App
	provider, model, convID string
	settings                usage.Settings
	total                   usage.Totals
	warned                  map[string]bool
	warn                    func(string)
}

// errBudget marks a run ended by a usage limit.
var errBudget = errors.New("usage limit")

func (a *App) newMeter(provider, model, convID string, warn func(string)) *meter {
	return &meter{app: a, provider: provider, model: model, convID: convID,
		settings: a.GetUsageSettings(), warned: map[string]bool{}, warn: warn}
}

// before is called ahead of every model request.
func (m *meter) before() error {
	day, month, err := m.app.ledger.Periods(time.Now())
	if err != nil {
		return fmt.Errorf("%w: usage records cannot be read: %v", errBudget, err)
	}
	_, priced := m.settings.PriceFor(m.provider, m.model)
	warnings, err := m.settings.Check(m.total, day, month, priced)
	if err != nil {
		return fmt.Errorf("%w: %v", errBudget, err)
	}
	for _, w := range warnings {
		if !m.warned[w] {
			m.warned[w] = true
			m.warn(w)
		}
	}
	return nil
}

// record adds one request to the run and the ledger.
func (m *meter) record(u appLayer.RequestUsage) {
	t := m.settings.Measure(m.provider, m.model, usage.Sample{
		Input: u.Input, Output: u.Output, Cached: u.Cached, Estimated: u.Estimated,
	})
	m.total.Add(t)
	key := usage.Key{Day: usage.Day(time.Now()), Provider: m.provider, Model: m.model, ConversationID: m.convID}
	if err := m.app.ledger.Record(key, t); err != nil {
		m.app.logf("could not record usage: %v", err)
	}
}
