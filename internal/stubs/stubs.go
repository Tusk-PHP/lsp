package stubs

import (
	"embed"
	"io/fs"
	"sort"
)

//go:embed php/*.php
var phpFS embed.FS

// Entry is an embedded builtin stub file ready for indexing by the parent.
type Entry struct {
	Path    string
	Content string
}

// BuiltinPHP returns embedded PHP builtin stubs in deterministic path order.
func BuiltinPHP() ([]Entry, error) {
	matches, err := fs.Glob(phpFS, "php/*.php")
	if err != nil {
		return nil, err
	}

	sort.Strings(matches)

	entries := make([]Entry, 0, len(matches))
	for _, path := range matches {
		content, err := fs.ReadFile(phpFS, path)
		if err != nil {
			return nil, err
		}

		entries = append(entries, Entry{
			Path:    path,
			Content: string(content),
		})
	}

	return entries, nil
}
