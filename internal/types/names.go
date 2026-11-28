package types

import (
	"slices"
	"strings"
)

// MapClassNames returns expr with the name of each named and generic type in
// it replaced by rename's result, and whether any name changed. A callable's
// own template names are left as they are inside it. expr is not modified.
func MapClassNames(expr TypeExpr, rename func(string) string) (TypeExpr, bool) {
	return mapClassNames(expr, rename, nil)
}

func mapClassNames(expr TypeExpr, rename func(string) string, templates []CallableTemplate) (TypeExpr, bool) {
	changed := false
	switch expr.Kind {
	case TypeKindNamed, TypeKindGeneric:
		if expr.Name != "" && !declaresTemplate(templates, expr.Name) {
			if name := rename(expr.Name); name != expr.Name {
				expr.Name = name
				changed = true
			}
		}
	case TypeKindCallable:
		if len(expr.Templates) > 0 {
			templates = append(templates[:len(templates):len(templates)], expr.Templates...)
		}
	}
	var ok bool
	if expr.Params, ok = mapClassNameList(expr.Params, rename, templates); ok {
		changed = true
	}
	if expr.Types, ok = mapClassNameList(expr.Types, rename, templates); ok {
		changed = true
	}
	if expr.ParamTypes, ok = mapClassNameList(expr.ParamTypes, rename, templates); ok {
		changed = true
	}
	if expr.Element, ok = mapClassNamePtr(expr.Element, rename, templates); ok {
		changed = true
	}
	if expr.ReturnType, ok = mapClassNamePtr(expr.ReturnType, rename, templates); ok {
		changed = true
	}
	if expr.ShapeFields, ok = mapShapeFieldNames(expr.ShapeFields, rename, templates); ok {
		changed = true
	}
	if expr.Templates, ok = mapTemplateBounds(expr.Templates, rename, templates); ok {
		changed = true
	}
	if expr.Condition, ok = mapConditionNames(expr.Condition, rename, templates); ok {
		changed = true
	}
	return expr, changed
}

func declaresTemplate(templates []CallableTemplate, name string) bool {
	for _, tmpl := range templates {
		if tmpl.Name == name {
			return true
		}
	}
	return false
}

func mapClassNameList(list []TypeExpr, rename func(string) string, templates []CallableTemplate) ([]TypeExpr, bool) {
	var out []TypeExpr
	for i := range list {
		mapped, ok := mapClassNames(list[i], rename, templates)
		if !ok {
			continue
		}
		if out == nil {
			out = slices.Clone(list)
		}
		out[i] = mapped
	}
	if out == nil {
		return list, false
	}
	return out, true
}

func mapClassNamePtr(expr *TypeExpr, rename func(string) string, templates []CallableTemplate) (*TypeExpr, bool) {
	if expr == nil {
		return nil, false
	}
	mapped, ok := mapClassNames(*expr, rename, templates)
	if !ok {
		return expr, false
	}
	return &mapped, true
}

func mapShapeFieldNames(fields []ShapeExprField, rename func(string) string, templates []CallableTemplate) ([]ShapeExprField, bool) {
	var out []ShapeExprField
	for i := range fields {
		mapped, ok := mapClassNames(fields[i].Type, rename, templates)
		if !ok {
			continue
		}
		if out == nil {
			out = slices.Clone(fields)
		}
		out[i].Type = mapped
	}
	if out == nil {
		return fields, false
	}
	return out, true
}

func mapTemplateBounds(list []CallableTemplate, rename func(string) string, templates []CallableTemplate) ([]CallableTemplate, bool) {
	var out []CallableTemplate
	for i := range list {
		bound, ok := mapClassNamePtr(list[i].Bound, rename, templates)
		if !ok {
			continue
		}
		if out == nil {
			out = slices.Clone(list)
		}
		out[i].Bound = bound
	}
	if out == nil {
		return list, false
	}
	return out, true
}

func mapConditionNames(cond *ConditionalType, rename func(string) string, templates []CallableTemplate) (*ConditionalType, bool) {
	if cond == nil {
		return nil, false
	}
	out := *cond
	target, t := mapClassNames(cond.Target, rename, templates)
	ifTrue, a := mapClassNames(cond.IfTrue, rename, templates)
	ifFalse, b := mapClassNames(cond.IfFalse, rename, templates)
	if !t && !a && !b {
		return cond, false
	}
	out.Target, out.IfTrue, out.IfFalse = target, ifTrue, ifFalse
	return &out, true
}

// CompactString renders expr like String, but writes the members of a union
// or intersection without spaces around `|` and `&`, as PHP's own types are
// written.
func (expr TypeExpr) CompactString() string {
	var sep string
	switch expr.Kind {
	case TypeKindUnion:
		sep = "|"
	case TypeKindIntersection:
		sep = "&"
	default:
		return expr.String()
	}
	var b strings.Builder
	if expr.Nullable {
		b.WriteByte('?')
	}
	if expr.Wrapped {
		b.WriteByte('(')
	}
	for i, part := range expr.Types {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(part.CompactString())
	}
	if expr.Wrapped {
		b.WriteByte(')')
	}
	return b.String()
}
