package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BuilderModel finds the Eloquent model a builder call is made on, from
// prefix, the line up to and including the call to method: Category::where(
// and Category::query()->where( both query Category, and $query->where( the
// model of $query's builder. Completion uses it for column and relation
// names, and the unknown-column and unknown-relation diagnostics to check
// them.
func BuilderModel(r *resolve.Resolver, index *symbols.Index, prefix, method, source string, pos protocol.Position, file *parser.FileNode) QueriedModel {
	trimmed := strings.TrimSpace(prefix)

	// The last ->method( or ::method( in the prefix.
	patterns := []string{"->" + method + "(", "::" + method + "("}
	bestIdx := -1
	bestOp := ""
	for _, pat := range patterns {
		idx := strings.LastIndex(trimmed, pat)
		if idx > bestIdx {
			bestIdx = idx
			if strings.HasPrefix(pat, "::") {
				bestOp = "::"
			} else {
				bestOp = "->"
			}
		}
	}
	if bestIdx < 0 {
		return QueriedModel{}
	}

	beforeMethod := trimmed[:bestIdx]

	if bestOp == "->" {
		return ModelQueriedBy(index, r.ChainType(beforeMethod, source, pos, file))
	}

	end := len(beforeMethod)
	i := end - 1
	for i >= 0 && (resolve.IsWordChar(beforeMethod[i]) || beforeMethod[i] == '\\') {
		i--
	}
	className := beforeMethod[i+1 : end]
	if className == "" {
		return QueriedModel{}
	}
	if fqn := r.ResolveClassNameLoose(className, file); IsModel(index, fqn) {
		return QueriedModel{FQN: fqn}
	}
	return QueriedModel{}
}
