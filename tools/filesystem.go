package tools

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-gui/domain"
)

const (
	maxReadBytes   = 2 * 1024 * 1024  // 2 MB per file read
	maxSearchBytes = 512 * 1024        // 512 KB per file during search
	maxSearchHits  = 200
)

type readFileTool struct{ workDir string }
type writeFileTool struct{ workDir string }
type listDirTool struct{ workDir string }
type searchFilesTool struct{ workDir string }
type deleteFileTool struct{ workDir string }

func NewReadFile(workDir string) domain.Tool    { return &readFileTool{workDir} }
func NewWriteFile(workDir string) domain.Tool   { return &writeFileTool{workDir} }
func NewListDir(workDir string) domain.Tool     { return &listDirTool{workDir} }
func NewSearchFiles(workDir string) domain.Tool { return &searchFilesTool{workDir} }
func NewDeleteFile(workDir string) domain.Tool  { return &deleteFileTool{workDir} }

func (t *readFileTool) Name() string        { return "read_file" }
func (t *readFileTool) Description() string { return "Read the contents of a file" }
func (t *readFileTool) Parameters() string  { return `{"path": "absolute or relative file path"}` }
func (t *readFileTool) Execute(args map[string]string) string {
	path, err := safePath(args["path"], t.workDir)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	info, err := os.Stat(path)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if info.Size() > maxReadBytes {
		return fmt.Sprintf("ERROR: file is too large (%d bytes). Max allowed is %d bytes.", info.Size(), maxReadBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return string(data)
}

func (t *writeFileTool) Name() string        { return "write_file" }
func (t *writeFileTool) Description() string { return "Write content to a file (creates or overwrites)" }
func (t *writeFileTool) Parameters() string {
	return `{"path": "file path", "content": "file content"}`
}
func (t *writeFileTool) Execute(args map[string]string) string {
	path, err := safePath(args["path"], t.workDir)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return "ERROR: " + err.Error()
	}
	if err := os.WriteFile(path, []byte(args["content"]), 0644); err != nil {
		return "ERROR: " + err.Error()
	}
	return fmt.Sprintf("Written %d bytes to %s", len(args["content"]), path)
}

func (t *listDirTool) Name() string        { return "list_dir" }
func (t *listDirTool) Description() string { return "List files and directories at a path" }
func (t *listDirTool) Parameters() string {
	return `{"path": "directory path (default: conversation work dir)"}`
}
func (t *listDirTool) Execute(args map[string]string) string {
	target := args["path"]
	if target == "" {
		target = t.workDir
	}
	path, err := safePath(target, t.workDir)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	var sb strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			sb.WriteString("[DIR]  " + e.Name() + "\n")
		} else {
			info, _ := e.Info()
			size := int64(0)
			if info != nil {
				size = info.Size()
			}
			sb.WriteString(fmt.Sprintf("[FILE] %s (%d bytes)\n", e.Name(), size))
		}
	}
	return sb.String()
}

func (t *searchFilesTool) Name() string        { return "search_files" }
func (t *searchFilesTool) Description() string { return "Search for text pattern in files recursively" }
func (t *searchFilesTool) Parameters() string {
	return `{"pattern": "text to find", "dir": "directory (default: work dir)", "ext": "extension filter e.g. .go (optional)"}`
}
func (t *searchFilesTool) Execute(args map[string]string) string {
	dir := args["dir"]
	if dir == "" {
		dir = t.workDir
	}
	root, err := safePath(dir, t.workDir)
	if err != nil {
		return "ERROR: " + err.Error()
	}

	ext := args["ext"]
	pattern := strings.ToLower(args["pattern"])
	var results []string

	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if ext != "" && !strings.HasSuffix(path, ext) {
			return nil
		}
		if info.Size() > maxSearchBytes {
			return nil // skip large files
		}
		if len(results) >= maxSearchHits {
			return filepath.SkipAll
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
				if len(results) >= maxSearchHits {
					return filepath.SkipAll
				}
			}
		}
		return nil
	})

	if len(results) == 0 {
		return "No matches found."
	}
	out := strings.Join(results, "\n")
	if len(results) >= maxSearchHits {
		out += fmt.Sprintf("\n\n(results capped at %d — refine your search)", maxSearchHits)
	}
	return out
}

func (t *deleteFileTool) Name() string        { return "delete_file" }
func (t *deleteFileTool) Description() string { return "Delete a file" }
func (t *deleteFileTool) Parameters() string  { return `{"path": "file path to delete"}` }
func (t *deleteFileTool) Execute(args map[string]string) string {
	path, err := safePath(args["path"], t.workDir)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	if err := os.Remove(path); err != nil {
		return "ERROR: " + err.Error()
	}
	return "Deleted: " + path
}

// resolve joins path with workDir if path is relative.
func resolve(path, workDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workDir, path)
}

// safePath resolves path relative to workDir and rejects traversal outside it.
func safePath(path, workDir string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	abs := resolve(path, workDir)
	abs = filepath.Clean(abs)
	workDir = filepath.Clean(workDir)
	if !strings.HasPrefix(abs, workDir+string(filepath.Separator)) && abs != workDir {
		return "", fmt.Errorf("path %q is outside the allowed work directory", path)
	}
	return abs, nil
}
