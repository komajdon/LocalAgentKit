// Package diff renders line differences in unified format.
package diff

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	context  = 3
	maxCells = 4_000_000 // bounds the comparison table
)

type op struct {
	kind byte // ' ', '-' or '+'
	line string
}

func lines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.SplitAfter(strings.TrimSuffix(s, "\n"), "\n")
}

// Unified returns the difference from old to new, labelled with name. Binary or
// very large inputs are summarised instead.
func Unified(name, old, new string) string {
	header := fmt.Sprintf("--- a/%s\n+++ b/%s\n", name, name)
	summary := func(why string) string {
		return header + fmt.Sprintf("(%s: %d bytes before, %d bytes after)\n", why, len(old), len(new))
	}
	if old == new {
		return header + "(no change)\n"
	}
	if !utf8.ValidString(old) || !utf8.ValidString(new) || strings.ContainsRune(old+new, 0) {
		return summary("binary content")
	}
	a, b := lines(old), lines(new)
	start := 0
	for start < len(a) && start < len(b) && a[start] == b[start] {
		start++
	}
	endA, endB := len(a), len(b)
	for endA > start && endB > start && a[endA-1] == b[endB-1] {
		endA, endB = endA-1, endB-1
	}
	n, m := endA-start, endB-start
	if n > 0 && m > maxCells/n {
		return summary("too large to compare line by line")
	}

	// Longest common subsequence of the differing middle.
	table := make([][]int32, n+1)
	for i := range table {
		table[i] = make([]int32, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[start+i] == b[start+j] {
				table[i][j] = table[i+1][j+1] + 1
			} else {
				table[i][j] = max(table[i+1][j], table[i][j+1])
			}
		}
	}
	var ops []op
	for _, l := range a[:start] {
		ops = append(ops, op{' ', l})
	}
	for i, j := 0, 0; i < n || j < m; {
		switch {
		case i < n && j < m && a[start+i] == b[start+j]:
			ops = append(ops, op{' ', a[start+i]})
			i, j = i+1, j+1
		case i < n && (j >= m || table[i+1][j] >= table[i][j+1]):
			ops = append(ops, op{'-', a[start+i]})
			i++
		default:
			ops = append(ops, op{'+', b[start+j]})
			j++
		}
	}
	for _, l := range a[endA:] {
		ops = append(ops, op{' ', l})
	}
	return header + hunks(ops, old, new)
}

func hunks(ops []op, old, new string) string {
	var out strings.Builder
	oldLine, newLine := 1, 1
	for i := 0; i < len(ops); {
		if ops[i].kind == ' ' {
			oldLine, newLine, i = oldLine+1, newLine+1, i+1
			continue
		}
		start := max(i-context, 0)
		end, quiet := i, 0
		for end < len(ops) && quiet <= 2*context {
			if ops[end].kind == ' ' {
				quiet++
			} else {
				quiet = 0
			}
			end++
		}
		if quiet > context {
			end -= quiet - context
		}
		a0, b0 := oldLine-(i-start), newLine-(i-start)
		var body strings.Builder
		na, nb := 0, 0
		for _, o := range ops[start:end] {
			if o.kind != '+' {
				na++
			}
			if o.kind != '-' {
				nb++
			}
			body.WriteByte(o.kind)
			body.WriteString(strings.TrimSuffix(o.line, "\n") + "\n")
		}
		if na == 0 {
			a0--
		}
		if nb == 0 {
			b0--
		}
		fmt.Fprintf(&out, "@@ -%d,%d +%d,%d @@\n%s", a0, na, b0, nb, body.String())
		for _, o := range ops[i:end] {
			if o.kind != '+' {
				oldLine++
			}
			if o.kind != '-' {
				newLine++
			}
		}
		i = end
	}
	// A file being created or emptied has no earlier ending to compare with.
	if old != "" && new != "" && strings.HasSuffix(old, "\n") != strings.HasSuffix(new, "\n") {
		out.WriteString("\\ The newline at end of file differs\n")
	}
	return out.String()
}
