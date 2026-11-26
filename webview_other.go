//go:build !linux || !(desktop || production || dev)

package main

// EnableMediaAccess is a no-op outside Linux desktop builds. Plain `go build`
// and `go test` use it too, so they do not need the GTK development headers.
func EnableMediaAccess() {}
