//go:build !linux && !darwin

package sandbox

import (
	"errors"
	"runtime"
)

func platformBackend() (*backend, error) {
	return nil, errors.New("no isolation backend for " + runtime.GOOS)
}
