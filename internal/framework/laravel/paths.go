package laravel

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
