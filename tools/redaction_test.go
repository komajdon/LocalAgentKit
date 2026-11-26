package tools

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"agent-gui/domain"
)

type failingTransport struct{}

func (failingTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("dial tcp: lookup failed")
}

func TestSearchErrorsDoNotLeakTheAPIKey(t *testing.T) {
	orig := http.DefaultClient.Transport
	http.DefaultClient.Transport = failingTransport{}
	defer func() { http.DefaultClient.Transport = orig }()

	const key = "serp-super-secret-key"
	for _, provider := range []string{"serpapi", "brave"} {
		out := NewWebSearch(provider, key).Execute(context.Background(), domain.Args{"query": "x"})
		if !strings.HasPrefix(out, "ERROR") || strings.Contains(out, key) {
			t.Errorf("%s leaked the key: %s", provider, out)
		}
	}
}
