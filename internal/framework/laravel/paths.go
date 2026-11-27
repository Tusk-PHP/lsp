package laravel

import (
	"path/filepath"
	"strings"
)

var pathHelperDirs = map[string]string{
	"base_path":     "",
	"app_path":      "app",
	"config_path":   "config",
	"database_path": "database",
	"lang_path":     "lang",
	"public_path":   "public",
	"resource_path": "resources",
	"storage_path":  "storage",
}

func PathHelperDir(helper string) (string, bool) {
	dir, ok := pathHelperDirs[helper]
	return dir, ok
}

func touchesAny(path string, targets []string) bool {
	path = filepath.Clean(path)
	for _, target := range targets {
		if within(path, target) || within(target, path) {
			return true
		}
	}
	return false
}

func within(path, dir string) bool {
	return path == dir || strings.HasPrefix(path, strings.TrimSuffix(dir, string(filepath.Separator))+string(filepath.Separator))
}
