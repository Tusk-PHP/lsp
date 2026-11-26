package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type BuilderNames int

const (
	BuilderColumns BuilderNames = iota + 1
	BuilderDBColumns
	BuilderRelations
)

type BuilderArray int

const (
	NoNamesInArray BuilderArray = iota
	NameList
	NamesOrNameKeys
	NameKeys
)

type BuilderSecondName int

const (
	NoSecondName BuilderSecondName = iota
	RelatedModelColumn
	KeyColumn
	ComparedColumn
)

type BuilderMethod struct {
	Names        BuilderNames
	Array        BuilderArray
	MayNameAlias bool
	Second       BuilderSecondName
}

func (m BuilderMethod) SecondNameAt(argCount int) (int, bool) {
	switch m.Second {
	case RelatedModelColumn, KeyColumn:
		return 1, true
	case ComparedColumn:
		if argCount > 2 {
			return 2, true
		}
		return 1, true
	}
	return 0, false
}

func (m BuilderMethod) NamesValues() bool {
	return m.Array == NameList || m.Array == NamesOrNameKeys
}

func (m BuilderMethod) NamesKeys() bool {
	return m.Array == NameKeys || m.Array == NamesOrNameKeys
}

var builderMethods = map[string]BuilderMethod{
	"where":           {Names: BuilderColumns, Array: NameKeys},
	"whereIn":         {Names: BuilderColumns},
	"whereNotIn":      {Names: BuilderColumns},
	"whereNull":       {Names: BuilderColumns, Array: NameList},
	"whereNotNull":    {Names: BuilderColumns, Array: NameList},
	"whereBetween":    {Names: BuilderColumns},
	"whereNotBetween": {Names: BuilderColumns},
	"whereDate":       {Names: BuilderColumns},
	"whereMonth":      {Names: BuilderColumns},
	"whereDay":        {Names: BuilderColumns},
	"whereYear":       {Names: BuilderColumns},
	"whereTime":       {Names: BuilderColumns},
	"whereColumn":     {Names: BuilderColumns, Second: ComparedColumn},
	"orWhereColumn":   {Names: BuilderColumns, Second: ComparedColumn},
	"orderBy":         {Names: BuilderColumns, MayNameAlias: true},
	"orderByDesc":     {Names: BuilderColumns, MayNameAlias: true},
	"latest":          {Names: BuilderColumns, MayNameAlias: true},
	"oldest":          {Names: BuilderColumns, MayNameAlias: true},
	"groupBy":         {Names: BuilderColumns, Array: NameList, MayNameAlias: true},
	"having":          {Names: BuilderColumns, MayNameAlias: true},
	"select":          {Names: BuilderColumns, Array: NameList},
	"addSelect":       {Names: BuilderColumns, Array: NameList},
	"pluck":           {Names: BuilderColumns, Second: KeyColumn},
	"value":           {Names: BuilderColumns},
	"increment":       {Names: BuilderColumns},
	"decrement":       {Names: BuilderColumns},
	"get":             {Names: BuilderDBColumns, Array: NameList},
	"with":            {Names: BuilderRelations, Array: NamesOrNameKeys},
	"without":         {Names: BuilderRelations, Array: NameList},
	"has":             {Names: BuilderRelations},
	"doesntHave":      {Names: BuilderRelations},
	"whereHas":        {Names: BuilderRelations},
	"whereDoesntHave": {Names: BuilderRelations},
	"withCount":       {Names: BuilderRelations, Array: NamesOrNameKeys},
	"withSum":         {Names: BuilderRelations, Array: NamesOrNameKeys, Second: RelatedModelColumn},
	"withAvg":         {Names: BuilderRelations, Array: NamesOrNameKeys, Second: RelatedModelColumn},
	"withMin":         {Names: BuilderRelations, Array: NamesOrNameKeys, Second: RelatedModelColumn},
	"withMax":         {Names: BuilderRelations, Array: NamesOrNameKeys, Second: RelatedModelColumn},
	"withExists":      {Names: BuilderRelations, Array: NamesOrNameKeys},
	"load":            {Names: BuilderRelations, Array: NamesOrNameKeys},
	"loadMissing":     {Names: BuilderRelations, Array: NamesOrNameKeys},
	"loadCount":       {Names: BuilderRelations, Array: NamesOrNameKeys},
}

func BuilderMethodNamed(name string) (BuilderMethod, bool) {
	m, ok := builderMethods[name]
	return m, ok
}

const (
	RelationDocSuffix = " relation"
	AccessorDocPrefix = "Accessor from "
	DatabaseColumnDoc = "(database column)"
)

func WithoutAlias(name string) string {
	name, _, _ = strings.Cut(name, " as ")
	return name
}

func IsRelationProperty(m *symbols.Symbol) bool {
	return m.Kind == symbols.KindProperty && m.IsVirtual && strings.HasSuffix(m.DocComment, RelationDocSuffix)
}

func IsColumnProperty(m *symbols.Symbol) bool {
	if m.Kind != symbols.KindProperty {
		return false
	}
	return !m.IsVirtual || !strings.HasSuffix(m.DocComment, RelationDocSuffix) && !strings.HasPrefix(m.DocComment, AccessorDocPrefix)
}

func IsDBColumnProperty(m *symbols.Symbol) bool {
	return m.Kind == symbols.KindProperty && m.IsVirtual && IsColumnProperty(m)
}

func IsLiveDBColumnProperty(m *symbols.Symbol) bool {
	return m.Kind == symbols.KindProperty && m.IsVirtual && strings.HasPrefix(m.DocComment, DatabaseColumnDoc)
}

func RelatedModel(index *symbols.Index, model, relation string) string {
	for _, m := range index.GetClassMembers(model) {
		if IsRelationProperty(m) && strings.TrimPrefix(m.Name, "$") == relation {
			related, singular := strings.CutPrefix(m.Type, "?")
			if !singular {
				return ""
			}
			return related
		}
	}
	return ""
}

type relationCardinality int

const (
	singularRelation relationCardinality = iota + 1
	pluralRelation
)

var relationTypes = map[string]relationCardinality{
	"HasOne": singularRelation, "BelongsTo": singularRelation, "MorphOne": singularRelation,
	"MorphTo": singularRelation, "HasOneThrough": singularRelation,
	"HasMany": pluralRelation, "BelongsToMany": pluralRelation, "MorphMany": pluralRelation,
	"MorphToMany": pluralRelation, "MorphedByMany": pluralRelation, "HasManyThrough": pluralRelation,
}

func IsRelationType(short string) bool {
	_, ok := relationTypes[short]
	return ok
}

func IsSingularRelationType(short string) bool {
	return relationTypes[short] == singularRelation
}

func IsPluralRelationType(short string) bool {
	return relationTypes[short] == pluralRelation
}
