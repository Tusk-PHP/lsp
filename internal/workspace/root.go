package workspace

import (
	"fmt"
	"os"
	"path/filepath"
)

// ResolveRoot returns the absolute path of the workspace root a user or
// client names, with every symbolic link on the way resolved, so that every
// path derived from it names files the way the workspace's file walk does.
// It fails when the root isn't a directory.
func ResolveRoot(given string) (string, error) {
	abs, err := filepath.Abs(given)
	if err != nil {
		return "", fmt.Errorf("resolving workspace root %q: %w", given, err)
	}
	root, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return "", fmt.Errorf("resolving workspace root %q: %w", given, err)
	}
	info, err := os.Stat(root)
	if err == nil && !info.IsDir() {
		err = fmt.Errorf("%s is not a directory", root)
	}
	if err != nil {
		return "", fmt.Errorf("resolving workspace root %q: %w", given, err)
	}
	return root, nil
}

// ResolvePath returns the absolute path of a file or directory a user or
// client names inside the workspace, spelled as the workspace's file walk
// spells it: the directories on the way have their symbolic links resolved,
// and so does a directory named itself, while a file keeps its own name.
// isDir reports whether it names a directory.
func ResolvePath(path string) (resolved string, isDir bool, err error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", false, fmt.Errorf("resolving %q: %w", path, err)
	}
	info, err := os.Stat(abs)
	if err != nil {
		return "", false, fmt.Errorf("resolving %q: %w", path, err)
	}
	isDir = info.IsDir()
	target := abs
	if !isDir {
		target = filepath.Dir(abs)
	}
	if resolved, err = filepath.EvalSymlinks(target); err != nil {
		return "", false, fmt.Errorf("resolving %q: %w", path, err)
	}
	if !isDir {
		resolved = filepath.Join(resolved, filepath.Base(abs))
	}
	return resolved, isDir, nil
}
