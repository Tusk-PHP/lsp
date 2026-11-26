package resolve

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// varAssertion returns the type an inline `/** @var Type $name */` gives
// varName on a statement that doesn't assign it, as PHPStan reads one: the
// doc comment documents a statement that starts after the write at after and
// holds or comes before the read at off, in a block the read is in. The last
// of them wins.
func (r *Resolver) varAssertion(varName string, pos textpos.Position, file *parser.FileNode, t *syntaxTree, after, off uint32) (ResolvedType, bool) {
	declared := r.inlineVarDoc(varName, t, after, off, false)
	if declared == "" {
		return ResolvedType{}, false
	}
	rt := r.declaredVarType(declared, pos, file)
	return rt, !rt.IsEmpty()
}

// VarDocType returns the type a `@var Type $name` doc comment declares for
// varName at pos, as written, in the variable's own scope: on the statement
// that assigns it or on one before the read.
func (r *Resolver) VarDocType(varName string, lines []string, pos textpos.Position, file *parser.FileNode) string {
	if file == nil {
		return ""
	}
	vs := r.variableScopeAt(varName, lines, pos)
	t := r.syntaxTree(lines, file)
	return r.inlineVarDoc(varName, t, t.offset(textpos.Position{Line: vs.firstLine}), t.offset(pos), true)
}

// inlineVarDoc returns the type of the last `@var Type $name` naming varName
// on a statement that starts after after and holds or comes before off, in a
// block off is in; statements that assign varName count only when
// assignments is set.
func (r *Resolver) inlineVarDoc(varName string, t *syntaxTree, after, off uint32, assignments bool) string {
	declared := ""
	for _, node := range t.path(off) {
		stmts, ok := statementsOf(node)
		if !ok {
			continue
		}
		prevEnd := node.Bounds().Start
		for _, stmt := range stmts {
			span := stmt.Bounds()
			if span.Start > off {
				break
			}
			if span.Start > after && (assignments || !t.assignsTo(stmt, varName)) || declaresGlobal(stmt, after) {
				for _, doc := range t.docsBetween(prevEnd, span.Start) {
					if d := r.docVarType(t, doc.Doc, varName, false); d != "" {
						declared = d
					}
				}
			}
			prevEnd = span.End
		}
	}
	return declared
}

// declaresGlobal reports whether stmt is the `global` statement that holds
// the write at after: its doc comment types the variables it brings in, as
// a Blade view's header declares the variables the view receives.
func declaresGlobal(stmt ast.Node, after uint32) bool {
	g, ok := stmt.(*ast.Global)
	if !ok {
		return false
	}
	span := g.Bounds()
	return span.Start <= after && after < span.End
}

// assignsTo reports whether stmt is an assignment to varName, whose @var
// types the value assigned instead.
func (t *syntaxTree) assignsTo(stmt ast.Node, varName string) bool {
	s, ok := stmt.(*ast.ExprStmt)
	if !ok {
		return false
	}
	for e := s.Expr; ; {
		a, ok := e.(*ast.Assign)
		if !ok {
			return false
		}
		if v, ok := a.Target.(*ast.Variable); ok && t.text(v) == varName {
			return true
		}
		e = a.Value
	}
}

// docsBetween returns the doc comments between from and to, in order: those
// of a statement that starts at to when from is where the one before it ends,
// also across a `?>` that ends the PHP code they are in.
func (t *syntaxTree) docsBetween(from, to uint32) []ast.DocComment {
	docs := t.root.Docs
	end := sort.Search(len(docs), func(i int) bool { return docs[i].Next > to })
	start := end
	for start > 0 && docs[start-1].Doc.Start >= from {
		start--
	}
	return docs[start:end]
}

// declaredVarType resolves the type a `@var` docblock declares, with the
// enclosing class's type aliases and namespace.
func (r *Resolver) declaredVarType(declared string, pos textpos.Position, file *parser.FileNode) ResolvedType {
	owner := FindEnclosingClass(file, pos)
	return r.resolveDocTypeIn(r.expandTypeAliases(declared, owner, file), owner, file)
}
