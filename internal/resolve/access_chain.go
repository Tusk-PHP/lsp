package resolve

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// NewChainEngine returns an Engine over index that resolves method chains in
// variable assignments ($user = User::first()) with ExpressionClass.
// Set ContainerCall on it to type container calls too.
func NewChainEngine(index *symbols.Index) *Engine {
	e := NewEngine(index)
	e.ChainResolver = (*Resolver).ExpressionClass
	return e
}

// ExpressionClass gives the class an expression such as "Category::first()"
// or "$foo->bar()->baz()" evaluates to, without type arguments: the class
// names of ChainType's result, the way ResolveVariableType gives them.
func (r *Resolver) ExpressionClass(expr string, source string, pos textpos.Position, file *parser.FileNode) string {
	return PlainTypeName(r.ChainType(expr, source, pos, file))
}

// DiagnosticType returns the type diagnostics check an expression node of
// file against: the generic-aware type, written out in full ("?Foo",
// "Foo|Bar", "Collection<int, User>"), the same one hover and completion
// see. Each lookup is a request of its own.
func (e *Engine) DiagnosticType(file *parser.FileNode, node ast.Expr) string {
	return e.Resolver().NodeType(file, node).FullString()
}
