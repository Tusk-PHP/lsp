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
		cleaned := strings.TrimPrefix(path.Clean(filepath.ToSlash(entry)), "/")
		switch {
		case cleaned == "" || cleaned == ".":
		case strings.Contains(cleaned, "/"):
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

func (r excludeRules) reaches(rel string) bool {
	for dir := path.Dir(rel); dir != "."; dir = path.Dir(dir) {
		if r.excludes(dir) {
			return false
		}
	}
	return true
}
