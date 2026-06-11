package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Package describes a single installed Composer package together with the
// PSR-4 namespace prefixes it exports.
type Package struct {
	Name         string
	Version      string
	PSR4Prefixes []string
}

// InstalledPackages reads vendor/composer/installed.json under rootPath and
// returns one Package per installed entry. Both the Composer v2 object form
// {"packages":[...]} and the legacy v1 top-level array form are supported.
// If the file is missing or cannot be parsed, nil is returned (no panic).
func InstalledPackages(rootPath string) []Package {
	installedPath := filepath.Join(rootPath, "vendor", "composer", "installed.json")
	data, err := os.ReadFile(installedPath)
	if err != nil {
		return nil
	}

	var raw []installedPackage

	// Try Composer v2 object form first.
	var v2 installedJSON
	if json.Unmarshal(data, &v2) == nil && v2.Packages != nil {
		raw = v2.Packages
	} else {
		// Fall back to legacy top-level array form.
		if json.Unmarshal(data, &raw) != nil {
			return nil
		}
	}

	packages := make([]Package, 0, len(raw))
	for _, p := range raw {
		pkg := Package{
			Name:    p.Name,
			Version: p.Version,
		}
		for ns := range p.Autoload.PSR4 {
			pkg.PSR4Prefixes = append(pkg.PSR4Prefixes, ns)
		}
		sort.Strings(pkg.PSR4Prefixes)
		packages = append(packages, pkg)
	}
	return packages
}

// NewPackageResolver builds a longest-PSR4-prefix matcher from the packages
// installed under rootPath and returns a resolver function. The resolver maps
// a vendor PHP FQN (e.g. "Monolog\Logger") to the Composer package name and
// version that provides it. When no package prefix matches, ok is false.
//
// The returned function is safe to call from multiple goroutines.
func NewPackageResolver(rootPath string) func(fqn string) (pkg, version string, ok bool) {
	type entry struct {
		prefix  string // PSR-4 prefix, normalised (no leading backslash, trailing backslash preserved)
		name    string
		version string
	}

	packages := InstalledPackages(rootPath)

	var entries []entry
	for _, p := range packages {
		for _, prefix := range p.PSR4Prefixes {
			// Normalise: strip any leading backslash and ensure a trailing one
			// so prefix matching is unambiguous (e.g. "Foo\" matches "Foo\Bar"
			// but not "FooBar").
			norm := strings.TrimLeft(prefix, "\\")
			if norm != "" && !strings.HasSuffix(norm, "\\") {
				norm += "\\"
			}
			entries = append(entries, entry{
				prefix:  norm,
				name:    p.Name,
				version: p.Version,
			})
		}
	}

	// Sort by descending prefix length so a linear scan finds the longest
	// match first.
	sort.Slice(entries, func(i, j int) bool {
		return len(entries[i].prefix) > len(entries[j].prefix)
	})

	return func(fqn string) (string, string, bool) {
		// Normalise: strip leading backslash.
		norm := strings.TrimLeft(fqn, "\\")
		for _, e := range entries {
			if strings.HasPrefix(norm, e.prefix) {
				return e.name, e.version, true
			}
		}
		return "", "", false
	}
}
