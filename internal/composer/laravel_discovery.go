package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
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

// LaravelPackageAliases returns the facade aliases installed packages register
// through Laravel's package discovery (extra.laravel.aliases in each package's
// composer.json), alias name to class FQN. Packages the project lists under
// extra.laravel.dont-discover are left out, and "*" leaves out all of them.
// A missing or unreadable vendor/composer/installed.json gives nil.
func LaravelPackageAliases(rootPath string) map[string]string {
	var aliases map[string]string
	for _, p := range discoveredLaravelPackages(rootPath) {
		for alias, fqn := range p.Aliases {
			if alias == "" || fqn == "" {
				continue
			}
			if aliases == nil {
				aliases = make(map[string]string)
			}
			aliases[alias] = fqn
		}
	}
	return aliases
}

// LaravelPackageProviders returns the service providers installed packages
// register through Laravel's package discovery (extra.laravel.providers), as
// class FQNs, leaving out the packages extra.laravel.dont-discover names.
func LaravelPackageProviders(rootPath string) []string {
	var providers []string
	for _, p := range discoveredLaravelPackages(rootPath) {
		for _, fqn := range p.Providers {
			if fqn != "" {
				providers = append(providers, strings.TrimPrefix(fqn, `\`))
			}
		}
	}
	return providers
}

// laravelExtra is a package's extra.laravel block.
type laravelExtra struct {
	Providers []string          `json:"providers"`
	Aliases   map[string]string `json:"aliases"`
}

// discoveredLaravelPackages reads the extra.laravel block of each installed
// package that Laravel's package discovery reads.
func discoveredLaravelPackages(rootPath string) []laravelExtra {
	var dontDiscover []string
	if data, err := os.ReadFile(filepath.Join(rootPath, "composer.json")); err == nil {
		var root struct {
			Extra struct {
				Laravel struct {
					DontDiscover []string `json:"dont-discover"`
				} `json:"laravel"`
			} `json:"extra"`
		}
		if json.Unmarshal(data, &root) == nil {
			dontDiscover = root.Extra.Laravel.DontDiscover
		}
	}
	skip := make(map[string]bool, len(dontDiscover))
	for _, name := range dontDiscover {
		if name == "*" {
			return nil
		}
		skip[name] = true
	}

	data, err := os.ReadFile(filepath.Join(rootPath, "vendor", "composer", "installed.json"))
	if err != nil {
		return nil
	}
	// extra is decoded per package, so one package's odd extra block
	// doesn't lose the others.
	type pkg struct {
		Name  string          `json:"name"`
		Extra json.RawMessage `json:"extra"`
	}
	var packages []pkg
	var v2 struct {
		Packages []pkg `json:"packages"`
	}
	if json.Unmarshal(data, &v2) == nil && v2.Packages != nil {
		packages = v2.Packages
	} else if json.Unmarshal(data, &packages) != nil {
		return nil
	}

	var out []laravelExtra
	for _, p := range packages {
		if skip[p.Name] || len(p.Extra) == 0 {
			continue
		}
		var extra struct {
			Laravel laravelExtra `json:"laravel"`
		}
		if json.Unmarshal(p.Extra, &extra) != nil {
			continue
		}
		out = append(out, extra.Laravel)
	}
	return out
}
