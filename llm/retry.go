package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math/rand/v2"
	"net"
	"net/http"
	"strconv"
	"syscall"
	"time"

	"agent-gui/domain"
)

const (
	maxAttempts   = 4
	maxBackoff    = 30 * time.Second
	maxRetryAfter = time.Minute
	maxErrorBody  = 4096
)

// baseDelay is shortened by tests.
var baseDelay = time.Second

// HTTPError is a non-200 reply from a provider.
type HTTPError struct {
	Provider   string
	Status     int
	Body       string
	RetryAfter time.Duration // zero when the server did not say
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s error %d: %s", e.Provider, e.Status, e.Body)
}

func newHTTPError(provider string, resp *http.Response) *HTTPError {
	body, _ := io.ReadAll(io.LimitReader(resp.Body, maxErrorBody))
	return &HTTPError{
		Provider:   provider,
		Status:     resp.StatusCode,
		Body:       string(body),
		RetryAfter: retryAfter(resp.Header.Get("Retry-After"), time.Now()),
	}
}

// retryAfter parses a Retry-After header: seconds or an HTTP date.
func retryAfter(v string, now time.Time) time.Duration {
	if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
		return time.Duration(secs) * time.Second
	}
	if at, err := http.ParseTime(v); err == nil && at.After(now) {
		return at.Sub(now)
	}
	return 0
}

// isTransient reports whether an error is worth retrying.
func isTransient(err error) bool {
	if err == nil || errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.Status {
		case http.StatusRequestTimeout, http.StatusTooEarly, http.StatusTooManyRequests,
			http.StatusInternalServerError, http.StatusBadGateway,
			http.StatusServiceUnavailable, http.StatusGatewayTimeout, 529:
			return true
		}
		return false
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var dnsErr *net.DNSError
	if errors.As(err, &dnsErr) {
		return true
	}
	for _, target := range []error{
		io.EOF, io.ErrUnexpectedEOF, domain.ErrInterrupted,
		syscall.ECONNREFUSED, syscall.ECONNRESET, syscall.EPIPE,
	} {
		if errors.Is(err, target) {
			return true
		}
	}
	return false
}

// backoff doubles each attempt, with jitter so clients do not retry in step.
func backoff(attempt int) time.Duration {
	d := min(baseDelay<<attempt, maxBackoff)
	return d/2 + rand.N(d/2+1)
}

// withRetry runs fn until it succeeds, fails permanently or runs out of
// attempts. emitted reports whether fn has already shown output to the caller;
// once it has, retrying would repeat that output, so the error is returned.
func withRetry(ctx context.Context, emitted func() bool, fn func() error) error {
	var err error
	for attempt := range maxAttempts {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil || emitted() || !isTransient(err) || attempt == maxAttempts-1 {
			return err
		}
		delay := backoff(attempt)
		var httpErr *HTTPError
		if errors.As(err, &httpErr) && httpErr.RetryAfter > 0 {
			// Asking sooner than the server allows is pointless, and waiting
			// very long would look like a hang.
			if httpErr.RetryAfter > maxRetryAfter {
				return err
			}
			delay = httpErr.RetryAfter
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(delay):
		}
	}
	return err
}
