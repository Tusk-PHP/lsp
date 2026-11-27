package symbols

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

var wholeGrammarRangeAllowed = map[string]string{
	"internal/symbols/symboltest/grammar.go:PinToPHP74":            "checks that the whole range reads the pinned text with another grammar",
	"internal/conformance/invariants.go:checkParserNeverNil":       "corpus files are parsed without an index, under their project's range",
	"internal/conformance/invariants.go:checkParserNoPanic":        "corpus files are parsed without an index, under their project's range",
	"internal/conformance/invariants.go:checkParserDeterministic":  "corpus files are parsed without an index, under their project's range",
	"internal/conformance/invariants.go:checkParserErrorPositions": "corpus files are parsed without an index, under their project's range",
	"internal/conformance/invariants.go:checkUseImportSpotCheck":   "corpus files are parsed without an index, under their project's range",
}

func TestProjectFilesAreParsedWithTheIndexsGrammarForThem(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	fset := token.NewFileSet()
	seen := make(map[string]bool)
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if name := d.Name(); path != root && (name == "testdata" || name == "vendor" || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, use := range wholeGrammarRangeUses(file) {
			site := filepath.ToSlash(rel) + ":" + use.function
			if _, ok := wholeGrammarRangeAllowed[site]; ok {
				seen[site] = true
				continue
			}
			t.Errorf("%s: %s passes the whole grammar range; parse a project file with Index.GrammarOf(uri)", fset.Position(use.pos), use.function)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	var stale []string
	for site := range wholeGrammarRangeAllowed {
		if !seen[site] {
			stale = append(stale, site)
		}
	}
	sort.Strings(stale)
	for _, site := range stale {
		t.Errorf("allowed site %s no longer passes the whole grammar range; remove it from the list", site)
	}
}

func TestWholeGrammarRangeUsesAreFound(t *testing.T) {
	const src = `package p

func direct(idx *Index, s string)      { parser.ParseFileIn(s, idx.Grammars()) }
func any(s string)                     { parser.ParseFileIn(s, phpparser.AnyGrammar()) }
func receiver(idx *Index, s string)    { idx.Grammars().Parse(s) }
func code(s string)                    { phpparser.AnyGrammar().ParseCode(s) }
func threaded(idx *Index, p string)    { readRoutes(p, idx.Grammars()) }
func profile(e entry, s string)        { parser.ParseFileIn(s, e.grammars()) }
func assigned(idx *Index, s string)    { g := idx.Grammars(); parser.ParseFileIn(s, g) }
func declared(idx *Index, s string)    { var g = idx.Grammars(); parser.ParseFileIn(s, g) }
func (r *Reader) method(s string)      { parser.ParseFileIn(s, r.index.Grammars()) }
func perFile(idx *Index, u, s string)  { parser.ParseFileIn(s, idx.GrammarOf(u)) }
func narrowed(idx *Index, u, s string) { g := idx.GrammarOf(u); parser.ParseFileIn(s, g) }
func compared(idx *Index) bool         { return idx.Grammars() == phpparser.AnyGrammar() }
func other(s string)                   { parser.New().Parse(s) }
func fresh(e entry) *Index             { return symbols.NewIndexFor(e.grammars()) }
func empty() *Index                    { return NewIndexFor(phpparser.AnyGrammar()) }
`
	file, err := parser.ParseFile(token.NewFileSet(), "p.go", src, 0)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, use := range wholeGrammarRangeUses(file) {
		got = append(got, use.function)
	}
	want := []string{"direct", "any", "receiver", "code", "threaded", "profile", "assigned", "declared", "Reader.method"}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("uses = %v, want %v", got, want)
	}
}

type wholeGrammarRangeUse struct {
	function string
	pos      token.Pos
}

func wholeGrammarRangeUses(file *ast.File) []wholeGrammarRangeUse {
	var uses []wholeGrammarRangeUse
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		name := functionName(fn)
		ranges := make(map[string]bool)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.AssignStmt:
				if len(n.Lhs) == len(n.Rhs) {
					for i, lhs := range n.Lhs {
						if id, ok := lhs.(*ast.Ident); ok && isWholeGrammarRange(n.Rhs[i], ranges) {
							ranges[id.Name] = true
						}
					}
				}
			case *ast.ValueSpec:
				if len(n.Names) == len(n.Values) {
					for i, id := range n.Names {
						if isWholeGrammarRange(n.Values[i], ranges) {
							ranges[id.Name] = true
						}
					}
				}
			case *ast.CallExpr:
				if parsesWithWholeGrammarRange(n, ranges) {
					uses = append(uses, wholeGrammarRangeUse{function: name, pos: n.Pos()})
				}
			}
			return true
		})
	}
	return uses
}

func functionName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return fn.Name.Name
	}
	recv := fn.Recv.List[0].Type
	if star, ok := recv.(*ast.StarExpr); ok {
		recv = star.X
	}
	if id, ok := recv.(*ast.Ident); ok {
		return id.Name + "." + fn.Name.Name
	}
	return fn.Name.Name
}

func parsesWithWholeGrammarRange(call *ast.CallExpr, ranges map[string]bool) bool {
	if calleeName(call.Fun) == "NewIndexFor" {
		return false
	}
	if sel, ok := call.Fun.(*ast.SelectorExpr); ok && (sel.Sel.Name == "Parse" || sel.Sel.Name == "ParseCode") && isWholeGrammarRange(sel.X, ranges) {
		return true
	}
	for _, arg := range call.Args {
		if isWholeGrammarRange(arg, ranges) {
			return true
		}
	}
	return false
}

func isWholeGrammarRange(e ast.Expr, ranges map[string]bool) bool {
	switch e := e.(type) {
	case *ast.Ident:
		return ranges[e.Name]
	case *ast.CallExpr:
		if len(e.Args) != 0 {
			return false
		}
		name := calleeName(e.Fun)
		return name == "Grammars" || name == "grammars" || name == "AnyGrammar"
	}
	return false
}

func calleeName(fun ast.Expr) string {
	switch fun := fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name
	case *ast.Ident:
		return fun.Name
	}
	return ""
}
