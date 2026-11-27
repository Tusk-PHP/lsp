package checks

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type receiver struct {
	node   ast.Expr
	static bool
	text   string
	line   int
	col    int
}

func (f *fileSyntax) receiverOf(ref memberReference) (receiver, bool) {
	span := ref.owner.Bounds()
	line, col := f.lines.Position(span.Start)
	if endLine, _ := f.lines.Position(span.End); endLine != line || f.line(ref.name) != line {
		return receiver{}, false
	}
	return receiver{node: ref.owner, static: ref.static, text: span.Text(f.src), line: line, col: col}, true
}

func (r receiver) className(file *parser.FileNode, index *symbols.Index) (string, bool) {
	if c, ok := r.node.(*ast.ConstFetch); !r.static || !ok || c.Name == nil {
		return "", false
	}
	return classNamed(r.text, r.line, file, index)
}

func (r receiver) classLikes(source string, file *parser.FileNode, index *symbols.Index, typeResolver func(string, string, int, int, *parser.FileNode) string) []string {
	if name, ok := r.className(file, index); ok {
		return []string{name}
	}
	if typeResolver == nil {
		return nil
	}
	return resolvedClassLikes(typeResolver(r.text, source, r.line, r.col, file), file, index)
}

func (f *fileSyntax) onThis(e ast.Expr) bool {
	for {
		switch n := e.(type) {
		case *ast.PropertyFetch:
			e = n.Object
		case *ast.MethodCall:
			e = n.Object
		case *ast.ArrayDimFetch:
			e = n.Array
		case *ast.Variable:
			return f.text(n) == "$this"
		default:
			return false
		}
	}
}

func (f *fileSyntax) inClosure(n ast.Node) bool {
	at := n.Bounds().Start
	for _, c := range f.references().closures {
		if c.Start > at {
			return false
		}
		if at < c.End {
			return true
		}
	}
	return false
}

func classNamed(name string, line int, file *parser.FileNode, index *symbols.Index) (string, bool) {
	fqn, ok := resolveClassLikeName(name, line, file)
	if !ok || fqn == "" {
		return "", false
	}
	return canonicalClassLike(fqn, file, index)
}
