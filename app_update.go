package main

// Version and update check.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// UpdateInfo is returned by CheckForUpdate.
type UpdateInfo struct {
	Available bool   `json:"available"`
	Current   string `json:"current"`
	Latest    string `json:"latest"`
	URL       string `json:"url"`
}

// GetVersion returns the version string embedded at build time.
func (a *App) GetVersion() string { return Version }

// CheckForUpdate fetches the latest GitHub release and compares it with the
// current build version. Returns immediately on any network error (no crash).
func (a *App) CheckForUpdate() UpdateInfo {
	info := UpdateInfo{Current: Version}
	if Version == "dev" {
		return info // skip update checks in dev builds
	}

	client := &http.Client{Timeout: 8 * time.Second}
	req, err := http.NewRequest(http.MethodGet,
		"https://api.github.com/repos/komajdon/LocalAgentKit/releases/latest", nil)
	if err != nil {
		return info
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := client.Do(req)
	if err != nil {
		return info
	}
	// Close the body on every status so the connection is not leaked.
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return info
	}

	var payload struct {
		TagName string `json:"tag_name"`
		HTMLURL string `json:"html_url"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil || payload.TagName == "" {
		return info
	}
	info.Latest = payload.TagName
	info.URL = payload.HTMLURL
	info.Available = isNewerVersion(payload.TagName, Version)
	return info
}

// isNewerVersion returns true when latest is semantically greater than current.
// Both strings may have a leading "v".
func isNewerVersion(latest, current string) bool {
	parse := func(s string) [3]int {
		s = strings.TrimPrefix(s, "v")
		parts := strings.SplitN(s, ".", 3)
		var nums [3]int
		for i, p := range parts {
			if i >= 3 {
				break
			}
			// A non-numeric part is deliberately left as zero.
			_, _ = fmt.Sscanf(p, "%d", &nums[i])
		}
		return nums
	}
	l, c := parse(latest), parse(current)
	for i := range l {
		if l[i] != c[i] {
			return l[i] > c[i]
		}
	}
	return false
}
