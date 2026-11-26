package redact

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestString(t *testing.T) {
	for _, in := range []string{
		`Get "https://x.test/s?q=1&api_key=SECRETVALUE": timeout`,
		`Authorization: Bearer SECRETVALUE123`,
		`header Bearer SECRETVALUE123`,
		`{"x-api-key":"SECRETVALUE"}`,
		`connect postgresql://admin:SECRETVALUE@db:5432/app failed`,
		`mongodb+srv://u:SECRETVALUE@cluster/x`,
		`invalid key sk-SECRETVALUE1234567890abc`,
		`token ghp_SECRETVALUE1234567890abcd`,
		`custom CUSTOMSECRET here`,
	} {
		out := String(in, "CUSTOMSECRET")
		if strings.Contains(out, "SECRET") && !strings.Contains(out, "[REDACTED]") ||
			strings.Contains(out, "SECRETVALUE") || strings.Contains(out, "CUSTOMSECRET") {
			t.Errorf("not redacted: %q -> %q", in, out)
		}
	}
}

func TestStringLeavesOrdinaryTextAlone(t *testing.T) {
	for _, in := range []string{
		"max_tokens=100 exceeded", "https://example.com:8080/path?q=a", "user@example.com", "",
	} {
		if out := String(in, "ab", ""); out != in {
			t.Errorf("%q changed to %q", in, out)
		}
	}
}

func TestErrorKeepsChain(t *testing.T) {
	err := fmt.Errorf("url ?api_key=abc123def: %w", context.Canceled)
	got := Error(err)
	if !errors.Is(got, context.Canceled) || strings.Contains(got.Error(), "abc123def") {
		t.Fatalf("got %v", got)
	}
	if Error(nil) != nil {
		t.Fatal("nil must stay nil")
	}
}
