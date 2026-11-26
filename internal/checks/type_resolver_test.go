package checks

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

func typesByText(types func(expr string) string) TypeResolver {
	return func(file *parser.FileNode, e ast.Expr) string {
		return types(e.Bounds().Text(file.Source))
	}
}
