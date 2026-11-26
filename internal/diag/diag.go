// Package diag writes the application's structured log.
//
// The log holds names, timings, counts and error categories: no prompts,
// replies, arguments or results. Every string is passed through redaction as a
// second line of defence. Nothing is sent anywhere.
package diag

import (
	"bytes"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"

	"agent-gui/internal/redact"
)

const (
	fileName = "app.log"
	maxSize  = 2 * 1024 * 1024 // the previous file is kept as app.log.1
)

// rotating is a size-limited log file.
type rotating struct {
	mu   sync.Mutex
	path string
	max  int64
}

func (r *rotating) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if info, err := os.Stat(r.path); err == nil && info.Size()+int64(len(p)) > r.max {
		if err := os.Rename(r.path, r.path+".1"); err != nil {
			return 0, err
		}
	}
	f, err := os.OpenFile(r.path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return 0, err
	}
	n, err := f.Write(p)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return n, err
}

func scrub(_ []string, a slog.Attr) slog.Attr {
	switch a.Value.Kind() {
	case slog.KindString:
		a.Value = slog.StringValue(redact.String(a.Value.String()))
	case slog.KindAny:
		if err, ok := a.Value.Any().(error); ok {
			a.Value = slog.StringValue(redact.String(err.Error()))
		}
	}
	return a
}

func logger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{ReplaceAttr: scrub}))
}

// Discard is a logger that records nothing.
func Discard() *slog.Logger { return logger(io.Discard) }

// Open returns a logger writing to dir.
func Open(dir string) (*slog.Logger, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return Discard(), err
	}
	return logger(&rotating{path: filepath.Join(dir, fileName), max: maxSize}), nil
}

// Tail returns the last lines of the log in dir, redacted again.
func Tail(dir string, lines int) []string {
	data, err := os.ReadFile(filepath.Join(dir, fileName))
	if err != nil {
		return []string{}
	}
	all := bytes.Split(bytes.TrimSpace(data), []byte("\n"))
	if len(all) > lines {
		all = all[len(all)-lines:]
	}
	out := make([]string, 0, len(all))
	for _, l := range all {
		out = append(out, redact.String(string(l)))
	}
	return out
}
