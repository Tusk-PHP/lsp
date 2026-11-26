package laravel

import (
	"slices"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const (
	EloquentModel    = `Illuminate\Database\Eloquent\Model`
	EloquentBuilder  = `Illuminate\Database\Eloquent\Builder`
	EloquentRelation = `Illuminate\Database\Eloquent\Relations\Relation`
)

type QueriedModel struct {
	FQN string
}

func IsModel(index *symbols.Index, fqn string) bool {
	return slices.Contains(index.GetInheritanceChain(fqn), EloquentModel)
}

func ModelQueriedBy(index *symbols.Index, rt resolve.ResolvedType) QueriedModel {
	base := rt.BaseFQN()
	if IsModel(index, base) {
		return QueriedModel{FQN: base}
	}
	if len(rt.Params) == 0 || !isOrExtends(index, base, EloquentBuilder, EloquentRelation) {
		return QueriedModel{}
	}
	model := rt.Params[0].BaseFQN()
	if !IsModel(index, model) {
		return QueriedModel{}
	}
	return QueriedModel{FQN: model}
}

func isOrExtends(index *symbols.Index, fqn string, classes ...string) bool {
	isOne := func(c string) bool { return slices.Contains(classes, c) }
	return isOne(fqn) || slices.ContainsFunc(index.GetInheritanceChain(fqn), isOne)
}
