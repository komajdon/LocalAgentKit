package tools

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"agent-gui/internal/profile"
)

// workspace confines file access to one directory.
//
// Every operation goes through os.Root, which resolves paths component by
// component and refuses to leave the root. This covers ".." traversal and
// symlinks, including symlinks swapped in while the operation runs. Symlinks
// with absolute targets are refused even when they point back inside the root.
//
// Protected directories (the app's own data) are refused even when they lie
// inside the workspace. Hard links are not detected.
type workspace struct {
	dir       string
	protected []string
	ignore    []string // patterns of paths that must not be listed or opened
}

var errIgnored = errors.New("path is excluded by the workspace's ignore patterns")

// isIgnored checks the path as given and, if it is a link, where it points.
func (w workspace) isIgnored(rel string) bool {
	if len(w.ignore) == 0 {
		return false
	}
	if profile.Ignored(w.ignore, filepath.ToSlash(rel)) {
		return true
	}
	root, err := filepath.EvalSymlinks(w.dir)
	if err != nil {
		return false
	}
	target, err := filepath.EvalSymlinks(filepath.Join(w.dir, rel))
	if err != nil {
		return false
	}
	inside, err := filepath.Rel(root, target)
	return err == nil && profile.Ignored(w.ignore, filepath.ToSlash(inside))
}

var errProtected = errors.New("path is inside a protected application directory")

// open returns the root handle for the workspace and path relative to it.
// The caller must close the root.
func (w workspace) open(path string) (*os.Root, string, error) {
	rel, err := w.rel(path)
	if err != nil {
		return nil, "", err
	}
	if w.isProtected(filepath.Join(w.dir, rel)) {
		return nil, "", errProtected
	}
	if w.isIgnored(rel) {
		return nil, "", errIgnored
	}
	root, err := os.OpenRoot(w.dir)
	if err != nil {
		return nil, "", fmt.Errorf("open work directory: %w", err)
	}
	return root, rel, nil
}

// rel converts a user-supplied path to a clean path relative to the workspace.
func (w workspace) rel(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	// An empty or relative work directory would silently mean the process's
	// current directory.
	if !filepath.IsAbs(w.dir) {
		return "", fmt.Errorf("no work directory is set for this conversation")
	}
	abs := filepath.Clean(resolve(path, w.dir))
	rel, err := filepath.Rel(filepath.Clean(w.dir), abs)
	if err != nil || !filepath.IsLocal(rel) {
		return "", fmt.Errorf("path %q is outside the allowed work directory", path)
	}
	return rel, nil
}

// isProtected reports whether abs, or any existing ancestor of it, is one of the
// protected directories. Comparing file identity rather than strings also
// covers symlinks and case-insensitive filesystems.
func (w workspace) isProtected(abs string) bool {
	var protected []os.FileInfo
	for _, p := range w.protected {
		if info, err := os.Stat(p); err == nil {
			protected = append(protected, info)
		}
	}
	if len(protected) == 0 {
		return false
	}
	for dir := filepath.Clean(abs); ; dir = filepath.Dir(dir) {
		if info, err := os.Stat(dir); err == nil {
			for _, p := range protected {
				if os.SameFile(info, p) {
					return true
				}
			}
		}
		if filepath.Dir(dir) == dir {
			return false
		}
	}
}

// openRegular opens a regular file for reading and returns its size. Pipes and
// devices are refused because reading them can block forever.
func (w workspace) openRegular(path string) (*os.File, int64, error) {
	root, rel, err := w.open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(rel)
	if err != nil {
		return nil, 0, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, fmt.Errorf("%q is not a regular file", path)
	}
	f, err := root.Open(rel)
	if err != nil {
		return nil, 0, err
	}
	return f, info.Size(), nil
}

// directory validates that path is an existing directory inside the workspace
// and returns its absolute path.
func (w workspace) directory(path string) (string, error) {
	root, rel, err := w.open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = root.Close() }()
	info, err := root.Stat(rel)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("%q is not a directory", path)
	}
	return filepath.Join(w.dir, rel), nil
}

// walk calls fn for every regular file below path, skipping protected
// directories and never following symlinked directories.
func (w workspace) walk(path string, fn func(abs, rel string, info fs.FileInfo, root *os.Root) error) error {
	root, start, err := w.open(path)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	return fs.WalkDir(root.FS(), filepath.ToSlash(start), func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if p == filepath.ToSlash(start) {
				return err
			}
			return nil // unreadable entries are skipped
		}
		rel := filepath.FromSlash(p)
		abs := filepath.Join(w.dir, rel)
		if d.IsDir() {
			if w.isProtected(abs) || (p != filepath.ToSlash(start) && w.isIgnored(rel)) {
				return fs.SkipDir
			}
			return nil
		}
		// Stat follows an in-root symlink to a file; escapes fail here.
		info, err := root.Stat(rel)
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		if w.isIgnored(rel) || (d.Type()&fs.ModeSymlink != 0 && w.isProtected(abs)) {
			return nil
		}
		return fn(abs, rel, info, root)
	})
}
