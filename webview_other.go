//go:build !linux || !(desktop || production || dev)

package main

// HardenWebView is a no-op outside Linux desktop builds: WebView2 and WKWebView
// do not grant permissions without asking. Plain `go build` and `go test` use
// this stub too, so they do not need the GTK development headers.
func HardenWebView() {}
