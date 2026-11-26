package tools

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"agent-gui/domain"
)

const (
	maxReadBytes   = 2 * 1024 * 1024 // 2 MB per file read
	maxSearchBytes = 512 * 1024      // 512 KB per file during search
	maxSearchHits  = 200
)

type readFileTool struct{ ws workspace }
type writeFileTool struct{ ws workspace }
type listDirTool struct{ ws workspace }
type searchFilesTool struct{ ws workspace }
type deleteFileTool struct{ ws workspace }

// The protected arguments name directories the tool must never touch.
func NewReadFile(workDir string, protected ...string) domain.Tool {
	return &readFileTool{workspace{workDir, protected}}
}
func NewWriteFile(workDir string, protected ...string) domain.Tool {
	return &writeFileTool{workspace{workDir, protected}}
}
func NewListDir(workDir string, protected ...string) domain.Tool {
	return &listDirTool{workspace{workDir, protected}}
}
func NewSearchFiles(workDir string, protected ...string) domain.Tool {
	return &searchFilesTool{workspace{workDir, protected}}
}
func NewDeleteFile(workDir string, protected ...string) domain.Tool {
	return &deleteFileTool{workspace{workDir, protected}}
}

func (t *readFileTool) Name() string        { return "read_file" }
func (t *readFileTool) Description() string { return "Read the contents of a file" }
func (t *readFileTool) Parameters() string  { return `{"path": "absolute or relative file path"}` }
func (t *readFileTool) Execute(args map[string]string) string {
	f, size, err := t.ws.openRegular(args["path"])
	if err != nil {
		return "ERROR: " + err.Error()
	}
	defer func() { _ = f.Close() }()
	if size > maxReadBytes {
		return fmt.Sprintf("ERROR: file is too large (%d bytes). Max allowed is %d bytes.", size, maxReadBytes)
	}
	// The limit also holds if the file grows while it is read.
	data, err := io.ReadAll(io.LimitReader(f, maxReadBytes))
	if err != nil {
		return "ERROR: " + err.Error()
	}
	return string(data)
}

func (t *writeFileTool) Name() string { return "write_file" }
func (t *writeFileTool) Description() string {
	return "Write content to a file (creates or overwrites)"
}
func (t *writeFileTool) Parameters() string {
	return `{"path": "file path", "content": "file content"}`
}
func (t *writeFileTool) Execute(args map[string]string) string {
	root, rel, err := t.ws.open(args["path"])
	if err != nil {
		return "ERROR: " + err.Error()
	}
	defer func() { _ = root.Close() }()
	if err := root.MkdirAll(filepath.Dir(rel), 0o755); err != nil {
		return "ERROR: " + err.Error()
	}
	if err := root.WriteFile(rel, []byte(args["content"]), 0o644); err != nil {
		return "ERROR: " + err.Error()
	}
	return fmt.Sprintf("Written %d bytes to %s", len(args["content"]), filepath.Join(t.ws.dir, rel))
}

func (t *listDirTool) Name() string        { return "list_dir" }
func (t *listDirTool) Description() string { return "List files and directories at a path" }
func (t *listDirTool) Parameters() string {
	return `{"path": "directory path (default: conversation work dir)"}`
}
func (t *listDirTool) Execute(args map[string]string) string {
	target := args["path"]
	if target == "" {
		target = t.ws.dir
	}
	root, rel, err := t.ws.open(target)
	if err != nil {
		return "ERROR: " + err.Error()
	}
	defer func() { _ = root.Close() }()
	entries, err := fs.ReadDir(root.FS(), filepath.ToSlash(rel))
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
			fmt.Fprintf(&sb, "[FILE] %s (%d bytes)\n", e.Name(), size)
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
		dir = t.ws.dir
	}

	ext := args["ext"]
	pattern := strings.ToLower(args["pattern"])
	var results []string

	err := t.ws.walk(dir, func(path, rel string, info fs.FileInfo, root *os.Root) error {
		if ext != "" && !strings.HasSuffix(path, ext) {
			return nil
		}
		if info.Size() > maxSearchBytes {
			return nil // skip large files
		}
		if len(results) >= maxSearchHits {
			return fs.SkipAll
		}
		data, err := root.ReadFile(rel)
		if err != nil {
			return nil
		}
		for i, line := range strings.Split(string(data), "\n") {
			if strings.Contains(strings.ToLower(line), pattern) {
				results = append(results, fmt.Sprintf("%s:%d: %s", path, i+1, strings.TrimSpace(line)))
				if len(results) >= maxSearchHits {
					return fs.SkipAll
				}
			}
		}
		return nil
	})
	if err != nil {
		return "ERROR: " + err.Error()
	}

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
	root, rel, err := t.ws.open(args["path"])
	if err != nil {
		return "ERROR: " + err.Error()
	}
	defer func() { _ = root.Close() }()
	if rel == "." {
		return "ERROR: refusing to delete the work directory itself"
	}
	// Remove deletes a symlink itself, never its target.
	if err := root.Remove(rel); err != nil {
		return "ERROR: " + err.Error()
	}
	return "Deleted: " + filepath.Join(t.ws.dir, rel)
}

// resolve joins path with workDir if path is relative.
func resolve(path, workDir string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(workDir, path)
}
