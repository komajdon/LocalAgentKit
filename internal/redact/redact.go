// Package redact removes credentials from text before it is shown, logged or
// sent to a model.
package redact

import (
	"regexp"
	"strings"
)

const mask = "[REDACTED]"

var patterns = []struct {
	re   *regexp.Regexp
	repl string
}{
	// Query parameters such as ?api_key=… or &token=…
	{regexp.MustCompile(`(?i)([?&](?:api[_-]?key|key|token|access[_-]?token|secret|password)=)[^&\s"']+`), "${1}" + mask},
	// Authorization headers and bare bearer tokens.
	{regexp.MustCompile(`(?i)(bearer\s+)[A-Za-z0-9._~+/=-]{8,}`), "${1}" + mask},
	{regexp.MustCompile(`(?i)((?:authorization|x-api-key|x-subscription-token)["']?\s*[:=]\s*["']?)[^\s"',}]+(\s+[A-Za-z0-9._~+/=\[\]-]{8,})?`), "${1}" + mask},
	// Passwords in connection URLs: scheme://user:password@host
	{regexp.MustCompile(`([a-z][a-z0-9+.-]*://[^/\s:@"']*:)[^/\s@"']+@`), "${1}" + mask + "@"},
	// Common API key shapes.
	{regexp.MustCompile(`\b(?:sk-[A-Za-z0-9_-]{16,}|gsk_[A-Za-z0-9]{16,}|gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|AIza[A-Za-z0-9_-]{30,}|hf_[A-Za-z0-9]{20,}|nvapi-[A-Za-z0-9_-]{20,}|csk-[A-Za-z0-9]{20,})\b`), mask},
}

// String removes the given secrets, plus anything that looks like a
// credential, from s.
func String(s string, secrets ...string) string {
	for _, secret := range secrets {
		// Very short values would blank out unrelated text.
		if len(secret) >= 6 {
			s = strings.ReplaceAll(s, secret, mask)
		}
	}
	for _, p := range patterns {
		s = p.re.ReplaceAllString(s, p.repl)
	}
	return s
}

type redacted struct {
	msg string
	err error
}

func (r redacted) Error() string { return r.msg }

// Unwrap keeps errors.Is and errors.As working.
func (r redacted) Unwrap() error { return r.err }

// Error returns err with a redacted message.
func Error(err error, secrets ...string) error {
	if err == nil {
		return nil
	}
	msg := String(err.Error(), secrets...)
	if msg == err.Error() {
		return err
	}
	return redacted{msg, err}
}
