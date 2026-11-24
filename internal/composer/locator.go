package composer

import (
	"path/filepath"
	"sort"
	"strings"
)

type ClassFile struct {
	Root     string
	Parts    []string
	IsVendor bool
}

func (f ClassFile) Path() string {
	return filepath.Join(append([]string{f.Root}, f.Parts...)...)
}

func ClassFiles(fqn string, entries []AutoloadEntry) []ClassFile {
	fqn = strings.TrimPrefix(fqn, "\\")
	type match struct {
		entry  AutoloadEntry
		length int
	}
	var matches []match
	for _, entry := range entries {
		if entry.IsFile || entry.Namespace == "" || !strings.HasPrefix(fqn, entry.Namespace+"\\") {
			continue
		}
		matches = append(matches, match{entry, len(entry.Namespace)})
	}
	sort.SliceStable(matches, func(i, j int) bool { return matches[i].length > matches[j].length })
	files := make([]ClassFile, 0, len(matches))
	for _, m := range matches {
		parts := strings.Split(fqn[len(m.entry.Namespace)+1:], "\\")
		parts[len(parts)-1] += ".php"
		files = append(files, ClassFile{Root: m.entry.Path, Parts: parts, IsVendor: m.entry.IsVendor})
	}
	return files
}
