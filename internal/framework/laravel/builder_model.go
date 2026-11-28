package laravel

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// BuilderModel finds the Eloquent model the builder call to method whose
// argument list holds pos is made on, read from file's syntax tree, so a call
// on a continuation line of a chain gets its model too. The unknown-column
// and unknown-relation diagnostics use it to check the call's arguments.
func BuilderModel(r *resolve.Resolver, index *symbols.Index, method, source string, pos textpos.Position, file *parser.FileNode) QueriedModel {
	if file == nil {
		return QueriedModel{}
	}
	off, inside := ast.NewLines(source).Offset(pos.Line, pos.Character, len(source))
	if !inside {
		return QueriedModel{}
	}
	call, _ := ast.CallAt(file.Tree, off)
	if call == nil || ast.MethodName(source, call) != method {
		return QueriedModel{}
	}
	return BuilderCallModel(r, index, call, source, file)
}

// BuilderCallModel finds the Eloquent model the builder call is made on:
// Category::where( and Category::query()->where( both query Category, and
// $query->where( the model of $query's builder. Completion uses it for column
// and relation names.
func BuilderCallModel(r *resolve.Resolver, index *symbols.Index, call ast.Expr, source string, file *parser.FileNode) QueriedModel {
	switch call := call.(type) {
	case *ast.StaticCall:
		return modelClass(r, index, call.Class, source, file)
	case *ast.MethodCall:
		return ModelQueriedBy(index, r.NodeType(file, call.Object))
	}
	return QueriedModel{}
}

func modelClass(r *resolve.Resolver, index *symbols.Index, e ast.Expr, source string, file *parser.FileNode) QueriedModel {
	name, ok := e.(*ast.ConstFetch)
	if !ok {
		return QueriedModel{}
	}
	if fqn := r.ResolveClassNameLoose(name.Name.Bounds().Text(source), file); IsModel(index, fqn) {
		return QueriedModel{FQN: fqn}
	}
	return QueriedModel{}
}
