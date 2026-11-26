package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestProviderErrorsDoNotEchoTheKey(t *testing.T) {
	const key = "my-private-provider-key"
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "bad credential: "+r.Header.Get("Authorization"), http.StatusUnauthorized)
	}))
	defer s.Close()
	p := NewOpenAIProvider(s.URL, key)
	_, _, chatErr := p.ChatStream(context.Background(), "m", nil, func(string) {})
	_, listErr := p.ListModels()
	for _, err := range []error{chatErr, listErr} {
		if err == nil || strings.Contains(err.Error(), key) {
			t.Errorf("err = %v", err)
		}
	}
}
