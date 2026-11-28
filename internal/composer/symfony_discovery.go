package composer

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

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

	fqns := bundleClasses(string(data))
	if len(fqns) == 0 {
		return nil
	}

	resolve := NewPackageResolver(rootPath)

	seen := make(map[string]struct{}, len(fqns))
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

func bundleClasses(src string) []string {
	bundles := phpparser.Parse(src, token.Latest).ReturnedArray()
	if bundles == nil {
		return nil
	}
	fqns := make([]string, 0, len(bundles.Items))
	for _, item := range bundles.Items {
		if item == nil {
			continue
		}
		fetch, ok := item.Key.(*ast.ClassConstFetch)
		if !ok {
			continue
		}
		class, ok := fetch.Class.(*ast.ConstFetch)
		if !ok || !strings.EqualFold(fetch.Name.Bounds().Text(src), "class") {
			continue
		}
		fqns = append(fqns, strings.TrimPrefix(class.Name.Text(src), `\`))
	}
	return fqns
}
