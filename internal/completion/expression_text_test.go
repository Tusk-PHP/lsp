package completion

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
)

// exprTypeAt types expr as if it were written as a statement at pos, which
// sees what the statements starting on pos's line or before assign: it goes
// before the first statement that starts after that line, in the innermost
// block around it, or at the start of the body a declaration opens.
func (p *Provider) exprTypeAt(expr, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
	expr = strings.TrimSuffix(strings.TrimSpace(expr), ";")
	grammar := token.Latest
	if file != nil {
		grammar = file.Grammar
	}
	lines := ast.NewLines(source)
	off, ok := lines.Offset(pos.Line+1, 0, len(source))
	if !ok {
		off = uint32(len(source))
	}
	at := insertionPoint(parser.ParseTree(source, grammar), off)
	stmt := "\n" + expr + ";\n"
	f := parser.ParseFile(source[:at]+stmt+source[at:], grammar)
	start := at + 1
	e := ast.ExprSpanning(f.Tree, ast.Span{Start: start, End: start + uint32(len(expr))})
	if e == nil {
		return resolve.ResolvedType{}
	}
	return p.engine.Resolver().NodeType(f, e)
}

func (p *Provider) plainExprTypeAt(expr, source string, pos protocol.Position, file *parser.FileNode) string {
	return resolve.PlainTypeName(p.exprTypeAt(expr, source, pos, file))
}

func insertionPoint(tree *ast.File, off uint32) uint32 {
	stmts, end := tree.Stmts, tree.End
	var body *ast.Block
	ast.Walk(tree, func(n ast.Node) bool {
		span := n.Bounds()
		if body == nil && span.Start >= off {
			if b, ok := n.(*ast.Block); ok {
				body = b
			}
		}
		if span.Start > off || span.End < off {
			return body == nil
		}
		switch n := n.(type) {
		case *ast.Block:
			if n.Start < off {
				stmts, end = n.Stmts, n.End-1
			}
		case *ast.Namespace:
			stmts, end = n.Stmts, n.End
		}
		return true
	})
	for _, s := range stmts {
		span := s.Bounds()
		if span.Start >= off {
			return span.Start
		}
		if off < span.End {
			if _, decl := s.(*ast.ClassDecl); decl && body != nil {
				return body.Start + 1
			}
			if _, decl := s.(*ast.FunctionDecl); decl && body != nil {
				return body.Start + 1
			}
			return span.End
		}
	}
	return end
}
