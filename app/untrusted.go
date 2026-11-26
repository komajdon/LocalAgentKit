package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strings"
	"unicode/utf8"

	"agent-gui/domain"
)

// maxToolResultBytes caps how much of one tool result enters the conversation.
const maxToolResultBytes = 100 * 1024

// toolResultPrefix starts every history message that carries a tool result.
const toolResultPrefix = "TOOL_RESULT for "

// isExternalSource reports whether a tool returns content that someone other
// than the user may have written: web pages, documents and MCP servers.
func isExternalSource(tool string) bool {
	return tool == "search_web" || tool == "read_document" || strings.Contains(tool, "__")
}

// frameToolResult wraps a result as untrusted data and records its source.
// The closing tag carries a random id so the content cannot forge it.
func frameToolResult(tool, source, result string) string {
	if len(result) > maxToolResultBytes {
		cut := maxToolResultBytes
		for cut > 0 && !utf8.RuneStart(result[cut]) {
			cut--
		}
		result = fmt.Sprintf("%s\n[... truncated %d of %d bytes ...]", result[:cut], len(result)-cut, len(result))
	}
	var b [8]byte
	_, _ = rand.Read(b[:]) // never fails
	id := hex.EncodeToString(b[:])
	source = strings.Join(strings.Fields(source), " ")
	if r := []rune(source); len(r) > 200 {
		source = string(r[:200]) + "…"
	}
	return fmt.Sprintf("%s%s:\n<untrusted-data id=%q source=%q>\n%s\n</untrusted-data id=%q>",
		toolResultPrefix, tool, id, source, result, id)
}

// sourceOf names where a result came from.
func sourceOf(tool string, args map[string]string) string {
	for _, k := range []string{"path", "query", "command", "url", "pattern"} {
		if v := args[k]; v != "" {
			return tool + " " + k + "=" + v
		}
	}
	return tool
}

// hasExternalContent reports whether the history already holds output from an
// external source. It is derived from the history so it survives a reload.
func hasExternalContent(msgs []domain.Message) bool {
	for _, m := range msgs {
		rest, ok := strings.CutPrefix(m.Content, toolResultPrefix)
		if !ok || m.Role != domain.RoleUser {
			continue
		}
		if tool, _, found := strings.Cut(rest, ":\n"); found && isExternalSource(tool) {
			return true
		}
	}
	return false
}
