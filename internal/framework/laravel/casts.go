package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const (
	castsAttributesInterface = `Illuminate\Contracts\Database\Eloquent\CastsAttributes`
	supportCarbon            = `Illuminate\Support\Carbon`
	carbonImmutable          = `Carbon\CarbonImmutable`
	supportCollection        = `Illuminate\Support\Collection`
	maxCastDepth             = 4
)

var builtinCastTypes = map[string]string{
	"int":                       "int",
	"integer":                   "int",
	"real":                      "float",
	"float":                     "float",
	"double":                    "float",
	"decimal":                   "string",
	"string":                    "string",
	"bool":                      "bool",
	"boolean":                   "bool",
	"object":                    "stdClass",
	"array":                     "array",
	"json":                      "array",
	"json:unicode":              "array",
	"collection":                supportCollection,
	"date":                      supportCarbon,
	"datetime":                  supportCarbon,
	"custom_datetime":           supportCarbon,
	"immutable_date":            carbonImmutable,
	"immutable_datetime":        carbonImmutable,
	"immutable_custom_datetime": carbonImmutable,
	"timestamp":                 "int",
	"hashed":                    "string",
	"encrypted":                 "string",
	"encrypted:array":           "array",
	"encrypted:collection":      supportCollection,
	"encrypted:json":            "array",
	"encrypted:object":          "stdClass",
}

var castTypePrefixes = []struct{ prefix, cast string }{
	{"date:", "custom_datetime"},
	{"datetime:", "custom_datetime"},
	{"immutable_date:", "immutable_custom_datetime"},
	{"immutable_datetime:", "immutable_custom_datetime"},
	{"decimal:", "decimal"},
}

// BuiltinCastType returns the type HasAttributes::castAttribute() gives an
// attribute read through one of the casts Eloquent itself implements, named
// as a model's $casts or casts() writes it: "datetime", "decimal:2",
// "encrypted:array". ok is false for any other string, such as a class name.
func BuiltinCastType(cast string) (phpType string, ok bool) {
	for _, p := range castTypePrefixes {
		if strings.HasPrefix(cast, p.prefix) {
			return builtinCastTypes[p.cast], true
		}
	}
	phpType, ok = builtinCastTypes[strings.ToLower(strings.TrimSpace(cast))]
	return phpType, ok
}

// ClassCastType returns the type an attribute cast by class has when it is
// read: the enum itself, what a CastsAttributes class's get() returns, or
// for a Castable class what the caster its castUsing() gives returns, with
// args, the class names the cast passes as its arguments (AsEnumCollection's
// enum, AsCollection::using()'s collection), binding castUsing()'s templates.
// It is "mixed" when the class or what it returns is unknown.
func ClassCastType(r *resolve.Resolver, class string, args []string) string {
	if rt := classCastType(r, class, args, 0); !rt.IsEmpty() {
		return rt.FullString()
	}
	return "mixed"
}

func classCastType(r *resolve.Resolver, class string, args []string, depth int) resolve.ResolvedType {
	sym := r.Index.Lookup(class)
	if sym == nil || depth > maxCastDepth {
		return resolve.ResolvedType{}
	}
	if sym.Kind == symbols.KindEnum {
		return resolve.ResolvedType{FQN: class}
	}
	if castUsing := method(r.Index, class, "castUsing"); castUsing != nil && castUsing.IsStatic {
		return casterType(r, castUsing, args, depth)
	}
	if !r.Index.IsSubtypeOf(class, castsAttributesInterface) {
		return resolve.ResolvedType{}
	}
	get := method(r.Index, class, "get")
	if get == nil {
		return resolve.ResolvedType{}
	}
	return known(r.MemberTypeForCall(get, nil, class, nil, nil))
}

func casterType(r *resolve.Resolver, castUsing *symbols.Symbol, args []string, depth int) resolve.ResolvedType {
	caster := r.MemberTypeForCall(castUsing, nil, castUsing.ParentFQN, nil, castArguments(args))
	caster = resolve.ClassStringTarget(caster)
	if strings.EqualFold(caster.FQN, castsAttributesInterface) {
		if len(caster.Params) == 0 {
			return resolve.ResolvedType{}
		}
		get := known(caster.Params[0])
		if len(args) > 0 && !get.IsEmpty() && r.Index.IsSubtypeOf(args[0], get.FQN) {
			return resolve.ResolvedType{FQN: args[0], Nullable: get.Nullable}
		}
		return get
	}
	if caster.IsEmpty() || len(caster.Union) > 0 {
		return resolve.ResolvedType{}
	}
	return classCastType(r, caster.FQN, nil, depth+1)
}

func castArguments(args []string) *resolve.CallSite {
	if len(args) == 0 {
		return nil
	}
	fields := make([]resolve.ShapeEntry, len(args))
	for i, arg := range args {
		if arg != "" {
			fields[i].Type = resolve.ResolvedType{FQN: "class-string", Params: []resolve.ResolvedType{{FQN: arg}}}
		}
	}
	arguments := resolve.ResolvedType{FQN: "array", Fields: fields}
	return &resolve.CallSite{
		Args:    []resolve.CallArg{{Expr: "$arguments"}},
		ArgType: func(string, *resolve.ResolvedType) resolve.ResolvedType { return arguments },
	}
}

func known(rt resolve.ResolvedType) resolve.ResolvedType {
	if rt.FQN == "mixed" && len(rt.Union) == 0 {
		return resolve.ResolvedType{}
	}
	return rt
}

func method(index *symbols.Index, class, name string) *symbols.Symbol {
	for _, m := range index.ClassMembersNamed(class, name) {
		if m.Kind == symbols.KindMethod {
			return m
		}
	}
	return nil
}
