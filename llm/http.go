package llm

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"agent-gui/domain"
)

const (
	// headerTimeout covers a slow start, such as a local model being loaded.
	headerTimeout = 5 * time.Minute
	// listTimeout bounds model discovery.
	listTimeout = 30 * time.Second
)

// idleTimeout aborts a stream that has gone silent. There is no limit on the
// total length of a reply.
var idleTimeout = 5 * time.Minute

func newHTTPClient() *http.Client {
	t := http.DefaultTransport.(*http.Transport).Clone()
	t.ResponseHeaderTimeout = headerTimeout
	return &http.Client{Transport: t}
}

// idleReader closes the body when no data arrives for idleTimeout.
type idleReader struct {
	body  io.ReadCloser
	timer *time.Timer
}

func newIdleReader(body io.ReadCloser) *idleReader {
	return &idleReader{body, time.AfterFunc(idleTimeout, func() { _ = body.Close() })}
}

func (r *idleReader) Read(p []byte) (int, error) {
	n, err := r.body.Read(p)
	if n > 0 {
		r.timer.Reset(idleTimeout)
	}
	return n, err
}

func (r *idleReader) stop() { r.timer.Stop() }

// interrupted describes a stream that ended before the provider finished.
func interrupted(ctx context.Context, cause error) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if cause == nil {
		return domain.ErrInterrupted
	}
	return fmt.Errorf("%w: %w", domain.ErrInterrupted, cause)
}
