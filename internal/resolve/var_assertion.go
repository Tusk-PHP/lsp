package resolve

import (
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
	declared := ""
	for node := ast.Node(t.root); node != nil; {
		var next ast.Node
		statements := holdsStatements(node)
		ast.Children(node, func(c ast.Node) {
			span := c.Bounds()
			if next != nil || span.Start > off {
				return
			}
			if statements && span.Start > after && !t.assignsTo(c, varName) {
				if d := t.docVar(span.Start, varName); d != "" {
					declared = d
				}
			}
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

// docVar returns the type the doc comment of the statement starting at
// offset gives varName with `@var Type $name`.
func (t *syntaxTree) docVar(offset uint32, varName string) string {
	span, ok := t.root.DocBefore(offset)
	if !ok {
		return ""
	}
	doc := parser.ParseDocBlock(t.text(span))
	if doc == nil {
		return ""
	}
	for _, v := range doc.Vars {
		if v.Name == varName {
			return v.Type
		}
	}
	return ""
}

// declaredVarType resolves the type a `@var` docblock declares, with the
// enclosing class's type aliases and namespace.
func (r *Resolver) declaredVarType(declared string, pos textpos.Position, file *parser.FileNode) ResolvedType {
	owner := FindEnclosingClass(file, pos)
	return r.resolveDocTypeIn(r.expandTypeAliases(declared, owner, file), owner, file)
}
