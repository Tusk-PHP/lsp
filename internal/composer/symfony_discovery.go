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
	file := phpparser.Parse(src, token.Latest)
	bundles := file.ReturnedArray()
	if bundles == nil {
		return nil
	}
	imports := classImports(file, src)
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
		fqns = append(fqns, resolveClassName(class.Name, imports, src))
	}
	return fqns
}

func classImports(file *ast.File, src string) map[string]string {
	imports := make(map[string]string)
	for _, stmt := range file.Stmts {
		use, ok := stmt.(*ast.Use)
		if !ok {
			continue
		}
		prefix := ""
		if use.Prefix != nil {
			prefix = strings.TrimPrefix(use.Prefix.Text(src), `\`) + `\`
		}
		for _, item := range use.Items {
			if item.Kind != token.Use || item.Name == nil {
				continue
			}
			fqn := prefix + strings.TrimPrefix(item.Name.Text(src), `\`)
			alias := fqn[strings.LastIndexByte(fqn, '\\')+1:]
			if item.Alias != nil {
				alias = item.Alias.Text(src)
			}
			imports[strings.ToLower(alias)] = fqn
		}
	}
	return imports
}

func resolveClassName(name *ast.Name, imports map[string]string, src string) string {
	text := name.Text(src)
	switch name.Kind {
	case token.NameFullyQualified:
		return text[1:]
	case token.NameRelative:
		return text[len(`namespace\`):]
	}
	first, rest, qualified := strings.Cut(text, `\`)
	fqn, ok := imports[strings.ToLower(first)]
	if !ok {
		return text
	}
	if !qualified {
		return fqn
	}
	return fqn + `\` + rest
}
