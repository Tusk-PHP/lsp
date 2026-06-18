package composer

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
)

// bundleClassRe matches PHP class constant expressions used as array keys in
// config/bundles.php, e.g. "Symfony\Bundle\FrameworkBundle\FrameworkBundle::class".
// Both fully-qualified forms (with or without a leading backslash) are accepted.
var bundleClassRe = regexp.MustCompile(`\\?[A-Za-z0-9_\\]+::class`)

// SymfonyBundlePackages reads config/bundles.php at rootPath and returns the
// Composer package names that own the registered Symfony bundle classes.
//
// config/bundles.php is the canonical bundle registry produced by Symfony Flex
// and Symfony 4+. It returns an array whose keys are fully-qualified bundle
// class names expressed as ::class constants, for example:
//
//	return [
//	    Symfony\Bundle\FrameworkBundle\FrameworkBundle::class => ['all' => true],
//	    App\MyBundle::class => ['dev' => true],
//	];
//
// Each bundle class FQN is resolved to its owning Composer package via
// NewPackageResolver. Bundle classes whose FQN cannot be resolved (e.g.
// first-party bundles in App\) are silently skipped — only vendor packages are
// relevant for the unused-dependency analysis.
//
// The result is a deduplicated, sorted slice of package names. Nil is returned
// when config/bundles.php is absent, unreadable, or contains no resolvable
// bundle classes. This is safe to call unconditionally on non-Symfony projects.
func SymfonyBundlePackages(rootPath string) []string {
	bundlesPath := filepath.Join(rootPath, "config", "bundles.php")
	data, err := os.ReadFile(bundlesPath)
	if err != nil {
		return nil
	}

	// Extract every ::class key expression from the file.  A regex/string scan
	// is intentional here (mirrors laravel_discovery.go) — we deliberately avoid
	// a full PHP parse dependency for these discovery helpers.
	matches := bundleClassRe.FindAllString(string(data), -1)
	if len(matches) == 0 {
		return nil
	}

	// Strip the trailing ::class suffix to obtain the bare FQN.
	fqns := make([]string, 0, len(matches))
	for _, m := range matches {
		fqn := m[:len(m)-len("::class")]
		if fqn != "" {
			fqns = append(fqns, fqn)
		}
	}

	if len(fqns) == 0 {
		return nil
	}

	resolve := NewPackageResolver(rootPath)

	seen := make(map[string]struct{})
	for _, fqn := range fqns {
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
