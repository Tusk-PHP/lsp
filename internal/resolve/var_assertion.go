package resolve

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// varAssertion returns the type an inline `/** @var Type $name */` gives
// varName on a statement that doesn't assign it, as PHPStan reads one: the
// doc comment documents a statement that starts after the write at after and
// holds or comes before the read at off, in a block the read is in. The last
// of them wins.
func (r *Resolver) varAssertion(varName string, pos textpos.Position, file *parser.FileNode, t *syntaxTree, after, off uint32) (ResolvedType, bool) {
	declared := ""
	for node := ast.Node(t.root); node != nil; {
		var next ast.Node
		statements := holdsStatements(node)
		prevEnd := node.Bounds().Start
		ast.Children(node, func(c ast.Node) {
			span := c.Bounds()
			if next != nil || span.Start > off {
				return
			}
			if statements && (span.Start > after && !t.assignsTo(c, varName) || declaresGlobal(c, after)) {
				for _, doc := range t.docsBetween(prevEnd, span.Start) {
					if d := t.docVarType(doc.Comment, varName, false); d != "" {
						declared = d
					}
				}
			}
			prevEnd = span.End
			if off <= span.End && span.End > span.Start {
				next = c
			}
		})
		node = next
	}
	if declared == "" {
		return ResolvedType{}, false
	}
	rt := r.declaredVarType(declared, pos, file)
	return rt, !rt.IsEmpty()
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

func holdsStatements(n ast.Node) bool {
	switch n.(type) {
	case *ast.Block, *ast.Case, *ast.Namespace, *ast.File:
		return true
	}
	return false
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

// docVarType returns the type doc gives varName with `@var Type $name`, or,
// when bare is set, with a bare `@var Type`.
func (t *syntaxTree) docVarType(doc ast.Span, varName string, bare bool) string {
	for _, v := range parser.ParseDocBlock(t.text(doc)).Vars {
		if v.Name == varName || bare && v.Name == "" {
			return v.Type
		}
	}
	return ""
}

// docBetween returns the last doc comment between from and to: the one of a
// statement that starts at to when from is where the one before it ends,
// also across a `?>` that ends the PHP code the doc comment is in.
func (t *syntaxTree) docBetween(from, to uint32) (ast.Span, bool) {
	docs := t.root.Docs
	i := sort.Search(len(docs), func(i int) bool { return docs[i].Comment.End > to }) - 1
	if i < 0 || docs[i].Comment.Start < from {
		return ast.Span{}, false
	}
	return docs[i].Comment, true
}

// docsBetween returns the doc comments between from and to, in order: those
// of a statement that starts at to when from is where the one before it ends,
// as a Blade view's header declares each variable in a doc comment of its own.
func (t *syntaxTree) docsBetween(from, to uint32) []ast.Doc {
	docs := t.root.Docs
	end := sort.Search(len(docs), func(i int) bool { return docs[i].Comment.End > to })
	start := end
	for start > 0 && docs[start-1].Comment.Start >= from {
		start--
	}
	return docs[start:end]
}

// statementDoc returns the doc comment of the innermost statement that holds
// off: the one between it and the statement before it.
func (t *syntaxTree) statementDoc(off uint32) (ast.Span, bool) {
	var doc ast.Span
	found := false
	for node := ast.Node(t.root); node != nil; {
		var next ast.Node
		statements := holdsStatements(node)
		prevEnd := node.Bounds().Start
		ast.Children(node, func(c ast.Node) {
			span := c.Bounds()
			if next != nil || span.Start > off {
				return
			}
			if off > span.End || span.End == span.Start {
				prevEnd = span.End
				return
			}
			next = c
			if statements {
				doc, found = t.docBetween(prevEnd, span.Start)
			}
		})
		node = next
	}
	return doc, found
}

// statementVarDoc returns the type the doc comment of the statement holding
// off gives varName, with `@var Type $name` or a bare `@var Type`.
func (t *syntaxTree) statementVarDoc(off uint32, varName string) string {
	doc, ok := t.statementDoc(off)
	if !ok {
		return ""
	}
	return t.docVarType(doc, varName, true)
}

// closureDoc returns the doc comment written on a closure, or else the one of
// the statement the closure is in.
func (r *Resolver) closureDoc(param *scope.Binding, lines []string, file *parser.FileNode) *parser.DocBlock {
	t := r.syntaxTree(lines, file)
	off := t.offset(param.Scope.Range.Start)
	doc, ok := t.root.DocBefore(off)
	if !ok {
		doc, ok = t.statementDoc(off)
	}
	if !ok {
		return nil
	}
	return parser.ParseDocBlock(t.text(doc))
}

// declaredVarType resolves the type a `@var` docblock declares, with the
// enclosing class's type aliases and namespace.
func (r *Resolver) declaredVarType(declared string, pos textpos.Position, file *parser.FileNode) ResolvedType {
	owner := FindEnclosingClass(file, pos)
	return r.resolveDocTypeIn(r.expandTypeAliases(declared, owner, file), owner, file)
}
