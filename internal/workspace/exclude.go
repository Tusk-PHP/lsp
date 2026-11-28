package workspace

import (
	"path"
	"path/filepath"
	"strings"
)

type excludeRules struct {
	names map[string]struct{}
	paths map[string]struct{}
}

func newExcludeRules(entries []string) excludeRules {
	rules := excludeRules{names: map[string]struct{}{}, paths: map[string]struct{}{}}
	for _, entry := range entries {
		slashed := filepath.ToSlash(entry)
		anchored := strings.HasPrefix(slashed, "/") || strings.HasPrefix(slashed, "./")
		cleaned := strings.TrimPrefix(path.Clean(slashed), "/")
		switch {
		case cleaned == "" || cleaned == ".":
		case anchored || strings.Contains(cleaned, "/"):
			rules.paths[cleaned] = struct{}{}
		default:
			rules.names[cleaned] = struct{}{}
		}
	}
	return rules
}

func (r excludeRules) excludes(rel string) bool {
	if _, ok := r.paths[rel]; ok {
		return true
	}
	_, ok := r.names[path.Base(rel)]
	return ok
}

func (r excludeRules) excludesDir(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil || rel == "." {
		return false
	}
	return r.excludes(filepath.ToSlash(rel))
}

func (r excludeRules) reaches(rel string) bool {
	for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
		if r.excludes(dir) {
			return false
		}
	}
	return true
}
