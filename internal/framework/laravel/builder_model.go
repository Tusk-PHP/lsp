package laravel

import (
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BuilderModel finds the Eloquent model a builder call with the receiver
// node of file is made on: the class of Category::where(, and the model of
// the builder or model Category::query()->where( or $query->where( is called
// on. Completion uses it for column and relation names, and the
// unknown-column and unknown-relation diagnostics to check them. Its FQN is
// "" when the model can't be worked out.
func BuilderModel(r *resolve.Resolver, index *symbols.Index, file *parser.FileNode, receiver ast.Expr) QueriedModel {
	if file == nil || receiver == nil {
		return QueriedModel{}
	}
	class, ok := receiver.(*ast.ConstFetch)
	if !ok {
		return ModelQueriedBy(index, r.NodeType(file, receiver))
	}
	if fqn := r.ResolveClassNameLoose(class.Name.Bounds().Text(file.Source), file); IsModel(index, fqn) {
		return QueriedModel{FQN: fqn}
	}
	return QueriedModel{}
}
