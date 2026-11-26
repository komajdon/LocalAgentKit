package tools

import (
	"path/filepath"
	"testing"
)

func TestSafePathLexicalBoundary(t *testing.T) {
	root := filepath.Join(t.TempDir(), "work")
	tests := []struct {
		in string
		ok bool
	}{
		{"a.txt", true},
		{"sub/../a.txt", true},
		{root, true},
		{"", false},
		{"../outside", false},
		{"sub/../../outside", false},
		{root + "-sibling/x", false},
		{filepath.Dir(root), false},
	}
	for _, tt := range tests {
		if _, err := safePath(tt.in, root); (err == nil) != tt.ok {
			t.Errorf("safePath(%q) err = %v, want ok=%v", tt.in, err, tt.ok)
		}
	}
}
