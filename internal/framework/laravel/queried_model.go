package laravel

import (
	"slices"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const (
	EloquentModel      = `Illuminate\Database\Eloquent\Model`
	EloquentBuilder    = `Illuminate\Database\Eloquent\Builder`
	EloquentCollection = `Illuminate\Database\Eloquent\Collection`
	EloquentRelations  = `Illuminate\Database\Eloquent\Relations\`
	EloquentRelation   = EloquentRelations + `Relation`
)

var tableJoiningRelations = []string{
	`Illuminate\Database\Eloquent\Relations\BelongsToMany`,
	`Illuminate\Database\Eloquent\Relations\MorphToMany`,
	`Illuminate\Database\Eloquent\Relations\HasOneOrManyThrough`,
	`Illuminate\Database\Eloquent\Relations\HasManyThrough`,
	`Illuminate\Database\Eloquent\Relations\HasOneThrough`,
}

type QueriedModel struct {
	FQN         string
	OtherTables bool
}

func IsModel(index *symbols.Index, fqn string) bool {
	return slices.Contains(index.GetInheritanceChain(fqn), EloquentModel)
}

func ModelQueriedBy(index *symbols.Index, rt resolve.ResolvedType) QueriedModel {
	if !singleClass(rt) {
		return QueriedModel{}
	}
	base := rt.BaseFQN()
	if IsModel(index, base) {
		return QueriedModel{FQN: base}
	}
	if len(rt.Params) == 0 || !singleClass(rt.Params[0]) || !isOrExtends(index, base, EloquentBuilder, EloquentRelation) {
		return QueriedModel{}
	}
	model := rt.Params[0].BaseFQN()
	if !IsModel(index, model) {
		return QueriedModel{}
	}
	return QueriedModel{FQN: model, OtherTables: isOrExtends(index, base, tableJoiningRelations...)}
}

func singleClass(rt resolve.ResolvedType) bool {
	return len(rt.Union) == 0 && len(rt.Intersection) == 0
}

func isOrExtends(index *symbols.Index, fqn string, classes ...string) bool {
	isOne := func(c string) bool { return slices.Contains(classes, c) }
	return isOne(fqn) || slices.ContainsFunc(index.GetInheritanceChain(fqn), isOne)
}
