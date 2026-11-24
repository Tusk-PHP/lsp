package composer

import (
	"path/filepath"
	"sort"
	"strings"
)

type ClassFile struct {
	Path     string
	IsVendor bool
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
		relative := strings.ReplaceAll(fqn[len(m.entry.Namespace)+1:], "\\", string(filepath.Separator))
		files = append(files, ClassFile{Path: filepath.Join(m.entry.Path, relative+".php"), IsVendor: m.entry.IsVendor})
	}
	return files
}
