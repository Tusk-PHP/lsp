package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// DirectRequires returns the package names listed under the "require" key of
// composer.json at rootPath, excluding platform pseudo-packages:
//   - the literal "php" (case-insensitive)
//   - anything matching "php-*" (e.g. "php-64bit")
//   - anything matching "ext-*"
//
// "require-dev" entries are never included. The slice is sorted for
// determinism. If composer.json is missing or malformed, nil is returned.
func DirectRequires(rootPath string) []string {
	composerPath := filepath.Join(rootPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil
	}

	var cj composerJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil
	}

	var pkgs []string
	for pkg := range cj.Require {
		if isPlatformPackage(pkg) {
			continue
		}
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}

// isPlatformPackage reports whether pkg is a Composer platform pseudo-package:
// "php", "php-*", or "ext-*" (all matched case-insensitively).
func isPlatformPackage(pkg string) bool {
	lower := strings.ToLower(pkg)
	return lower == "php" ||
		strings.HasPrefix(lower, "php-") ||
		strings.HasPrefix(lower, "ext-")
}
