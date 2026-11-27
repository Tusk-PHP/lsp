package conformance

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The analysis engine is meant to move to Tusk-PHP/diagnostics (see
// plans/analysis-engine-extraction.md there). These packages are the language
// server's side of that boundary: the transport, the editor features, and what
// only the editor, MCP or CLI uses. Every other package under internal/ is
// engine.
var lspSidePackages = []string{
	"protocol", "lsp", "document", "mcpserver",
	"completion", "hover", "analyzer", "inlayhint", "source",
	"coverage", "introspect", "symquery",
	"graph", "deps", "unuseddeps",
	"composer/cardhover",
	"conformance",
}

// An engine package must not import these.
var engineForbiddenImports = []string{
	"protocol", "lsp", "document", "mcpserver",
	"completion", "hover", "analyzer", "inlayhint", "source",
}

// Today's violations, as "importer -> imported". Each item of the plan's Stage 1
// removes its entries; the test fails when an entry is no longer needed, so the
// list only shrinks.
var engineBoundaryAllowlist = map[string]bool{
	"config -> protocol":            true,
	"diagnostics -> protocol":       true,
	"framework/laravel -> protocol": true,
	"framework/symfony -> protocol": true,
}

const internalImportPrefix = "github.com/Tusk-PHP/lsp/internal/"

// inPackageList reports whether pkg is one of list or a subpackage of one.
func inPackageList(pkg string, list []string) bool {
	for _, p := range list {
		if pkg == p || strings.HasPrefix(pkg, p+"/") {
			return true
		}
	}
	return false
}

// TestEngineBoundary fails when an engine package imports the language server's
// side, in non-test files of any platform.
func TestEngineBoundary(t *testing.T) {
	internalDir := filepath.Join("..", "..", "internal")
	found := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(internalDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "testdata" {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		rel, err := filepath.Rel(internalDir, filepath.Dir(path))
		if err != nil {
			return err
		}
		pkg := filepath.ToSlash(rel)
		if inPackageList(pkg, lspSidePackages) {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			ipath, err := strconv.Unquote(imp.Path.Value)
			if err != nil || !strings.HasPrefix(ipath, internalImportPrefix) {
				continue
			}
			target := strings.TrimPrefix(ipath, internalImportPrefix)
			if inPackageList(target, engineForbiddenImports) {
				found[pkg+" -> "+target] = true
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	var added, stale []string
	for edge := range found {
		if !engineBoundaryAllowlist[edge] {
			added = append(added, edge)
		}
	}
	for edge := range engineBoundaryAllowlist {
		if !found[edge] {
			stale = append(stale, edge)
		}
	}
	sort.Strings(added)
	sort.Strings(stale)
	for _, edge := range added {
		t.Errorf("engine package imports the language server's side: %s (move the shared piece into the engine instead)", edge)
	}
	for _, edge := range stale {
		t.Errorf("%s no longer happens: remove it from engineBoundaryAllowlist", edge)
	}
}
