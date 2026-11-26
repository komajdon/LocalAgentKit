package llm

import (
	"net/http"
	"reflect"
	"testing"
)

func TestModelDiscoveryWithFallback(t *testing.T) {
	static := []string{"old-a", "old-b"}
	for name, tc := range map[string]struct {
		status int
		body   string
		want   []string
	}{
		"live":         {200, `{"data":[{"id":"new"}]}`, []string{"new"}},
		"unauthorized": {401, `no key`, static},
		"empty":        {200, `{"data":[]}`, static},
		"not json":     {200, `<html>`, static},
	} {
		url := serveStatus(t, tc.status, tc.body)
		got, err := NewFreeProvider(url, "", static).ListModels()
		if err != nil || !reflect.DeepEqual(got, tc.want) {
			t.Errorf("%s: %v %v", name, got, err)
		}
	}
	if got, _ := NewFreeProvider("http://127.0.0.1:1", "", static).ListModels(); !reflect.DeepEqual(got, static) {
		t.Errorf("unreachable: %v", got)
	}
}

func serveStatus(t *testing.T, status int, body string) string {
	t.Helper()
	url, _ := script(t, func(_ int32, w http.ResponseWriter) {
		w.WriteHeader(status)
		_, _ = w.Write([]byte(body))
	})
	return url
}
