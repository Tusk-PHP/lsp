package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
)

// LaravelAutoDiscoveredPackages reads composer.json at rootPath and returns the
// Composer package names that are registered via Laravel's package auto-discovery
// mechanism (the extra.laravel.providers and extra.laravel.aliases keys).
//
// These packages are consumed by the framework through non-static means
// (service provider registration, facade aliases) and must NOT be flagged as
// unused even when no static code reference exists.
//
// The result is a deduplicated, sorted slice of package names. Unresolvable FQNs
// and missing/malformed composer.json are skipped silently; nil is returned when
// no packages are found.
func LaravelAutoDiscoveredPackages(rootPath string) []string {
	composerPath := filepath.Join(rootPath, "composer.json")
	data, err := os.ReadFile(composerPath)
	if err != nil {
		return nil
	}

	var cj composerJSON
	if err := json.Unmarshal(data, &cj); err != nil {
		return nil
	}

	// Collect every class FQN declared under extra.laravel.
	var fqns []string
	fqns = append(fqns, cj.Extra.Laravel.Providers...)
	for _, alias := range cj.Extra.Laravel.Aliases {
		fqns = append(fqns, alias)
	}

	if len(fqns) == 0 {
		return nil
	}

	// Map each FQN to its owning Composer package via the PSR-4 resolver.
	resolve := NewPackageResolver(rootPath)

	seen := make(map[string]struct{})
	for _, fqn := range fqns {
		if fqn == "" {
			continue
		}
		pkg, _, ok := resolve(fqn)
		if !ok || pkg == "" {
			continue
		}
		seen[pkg] = struct{}{}
	}

	if len(seen) == 0 {
		return nil
	}

	pkgs := make([]string, 0, len(seen))
	for pkg := range seen {
		pkgs = append(pkgs, pkg)
	}
	sort.Strings(pkgs)
	return pkgs
}
