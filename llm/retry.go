package llm

import (
	"context"
	"strings"
	"time"
)

const (
	maxRetries  = 3
	retryDelay  = 1 * time.Second
)

// isTransient returns true for errors that are worth retrying
// (network blips, temporary server errors).
func isTransient(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, kw := range []string{
		"connection refused", "connection reset", "eof",
		"timeout", "temporary", "503", "502", "429",
		"broken pipe", "no such host",
	} {
		if strings.Contains(msg, kw) {
			return true
		}
	}
	return false
}

// withRetry calls fn up to maxRetries times on transient errors.
// Context cancellation stops retrying immediately.
func withRetry(ctx context.Context, fn func() error) error {
	var err error
	for attempt := 0; attempt < maxRetries; attempt++ {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		err = fn()
		if err == nil || !isTransient(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(retryDelay * time.Duration(attempt+1)):
		}
	}
	return err
}
