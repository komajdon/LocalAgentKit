package llm

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agent-gui/domain"
)

func fastRetry(t *testing.T) {
	t.Helper()
	orig := baseDelay
	baseDelay = time.Millisecond
	t.Cleanup(func() { baseDelay = orig })
}

// script serves handler(n) for the n-th request.
func script(t *testing.T, handler func(n int32, w http.ResponseWriter)) (string, *atomic.Int32) {
	t.Helper()
	var n atomic.Int32
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		handler(n.Add(1), w)
	}))
	t.Cleanup(s.Close)
	return s.URL, &n
}

const (
	openAIGood    = "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\ndata: [DONE]\n"
	openAIPartial = "data: {\"choices\":[{\"delta\":{\"content\":\"par\"}}]}\n\n"
	ollamaPartial = "{\"message\":{\"content\":\"par\"},\"done\":false}\n"
)

func chat(p domain.LLMProvider) (string, string, error) {
	var seen strings.Builder
	out, _, err := p.ChatStream(context.Background(), "m", nil, func(c string) { seen.WriteString(c) })
	return out, seen.String(), err
}

func TestInterruptedStreamIsNotRetriedAndKeepsPartialText(t *testing.T) {
	fastRetry(t)
	for name, tc := range map[string]struct {
		body string
		make func(string) domain.LLMProvider
	}{
		"openai": {openAIPartial, func(u string) domain.LLMProvider { return NewOpenAIProvider(u, "") }},
		"ollama": {ollamaPartial, func(u string) domain.LLMProvider { return NewOllamaProvider(u) }},
	} {
		t.Run(name, func(t *testing.T) {
			for _, abrupt := range []bool{false, true} {
				url, n := script(t, func(_ int32, w http.ResponseWriter) {
					_, _ = io.WriteString(w, tc.body)
					if abrupt {
						w.(http.Flusher).Flush()
						panic(http.ErrAbortHandler)
					}
				})
				out, seen, err := chat(tc.make(url))
				if !errors.Is(err, domain.ErrInterrupted) || out != "par" || seen != "par" || n.Load() != 1 {
					t.Fatalf("abrupt=%v: out=%q seen=%q requests=%d err=%v", abrupt, out, seen, n.Load(), err)
				}
			}
		})
	}
}

func TestFailureBeforeOutputIsRetried(t *testing.T) {
	fastRetry(t)
	url, n := script(t, func(n int32, w http.ResponseWriter) {
		switch n {
		case 1:
			w.WriteHeader(http.StatusServiceUnavailable)
		case 2: // cut off before any text
			panic(http.ErrAbortHandler)
		default:
			_, _ = io.WriteString(w, openAIGood)
		}
	})
	out, seen, err := chat(NewOpenAIProvider(url, ""))
	if err != nil || out != "ok" || seen != "ok" || n.Load() != 3 {
		t.Fatalf("out=%q seen=%q requests=%d err=%v", out, seen, n.Load(), err)
	}
}

func TestPermanentErrorsAndAttemptLimit(t *testing.T) {
	fastRetry(t)
	for status, want := range map[int]int32{
		http.StatusUnauthorized: 1, http.StatusBadRequest: 1, http.StatusNotFound: 1,
		http.StatusTooManyRequests: maxAttempts, http.StatusBadGateway: maxAttempts,
	} {
		// The body mentions 503: classification must use the status, not text.
		url, n := script(t, func(_ int32, w http.ResponseWriter) { http.Error(w, "timeout 503", status) })
		_, _, err := chat(NewOpenAIProvider(url, ""))
		var httpErr *HTTPError
		if !errors.As(err, &httpErr) || httpErr.Status != status || n.Load() != want {
			t.Errorf("status %d: requests=%d want=%d err=%v", status, n.Load(), want, err)
		}
	}
}

func TestLongRetryAfterGivesUp(t *testing.T) {
	fastRetry(t)
	url, n := script(t, func(_ int32, w http.ResponseWriter) {
		w.Header().Set("Retry-After", "3600")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	if _, _, err := chat(NewOpenAIProvider(url, "")); err == nil || n.Load() != 1 {
		t.Fatalf("requests=%d err=%v", n.Load(), err)
	}
}

func TestRetryAfterAndBackoff(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	for in, want := range map[string]time.Duration{
		"5": 5 * time.Second, "": 0, "soon": 0, "-1": 0,
		now.Add(time.Minute).Format(http.TimeFormat): time.Minute,
	} {
		if got := retryAfter(in, now); got != want {
			t.Errorf("retryAfter(%q) = %v, want %v", in, got, want)
		}
	}
	for attempt := range 12 {
		ideal := min(baseDelay<<attempt, maxBackoff)
		if d := backoff(attempt); d < ideal/2 || d > ideal {
			t.Errorf("backoff(%d) = %v outside [%v,%v]", attempt, d, ideal/2, ideal)
		}
	}
}

func TestSilentStreamTimesOut(t *testing.T) {
	orig := idleTimeout
	idleTimeout = 100 * time.Millisecond
	defer func() { idleTimeout = orig }()
	release := make(chan struct{})
	url, _ := script(t, func(_ int32, w http.ResponseWriter) {
		_, _ = io.WriteString(w, openAIPartial)
		w.(http.Flusher).Flush()
		<-release
	})
	defer close(release)
	out, _, err := chat(NewOpenAIProvider(url, ""))
	if !errors.Is(err, domain.ErrInterrupted) || out != "par" {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
