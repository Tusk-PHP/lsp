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

// The analysis engine stays apart from the editor inside this module (see
// plans/analysis-engine-extraction.md in Tusk-PHP/diagnostics). These packages
// are the language server's side of that boundary: the transports, the editor
// features, session (which wires both for every transport), and what only the
// editor, MCP or CLI uses. Every other package under internal/ is engine.
var lspSidePackages = []string{
	"protocol", "lsp", "document", "mcpserver", "session",
	"completion", "hover", "analyzer", "inlayhint", "source",
	"coverage", "introspect", "symquery",
	"graph", "deps", "unuseddeps",
	"composer/cardhover",
	"conformance",
}

// An engine package must not import these.
var engineForbiddenImports = []string{
	"protocol", "lsp", "document", "mcpserver", "session",
	"completion", "hover", "analyzer", "inlayhint", "source",
}

// Engine imports of the language server's side that are still allowed, as
// "importer -> imported". It has been empty since Stage 1 of the extraction
// plan drew the boundary, and it stays empty: move what the engine needs into
// the engine instead. The test fails when an entry here is no longer needed.
var engineBoundaryAllowlist = map[string]bool{}

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
