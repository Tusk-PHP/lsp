package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/phparray"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// ResolvedType represents a type with optional generic parameters.
// For example, Collection<int, Category> is {FQN: "Illuminate\...\Collection", Params: [{FQN: "int"}, {FQN: "App\Models\Category"}]}.
//
// A union, intersection or conditional type keeps its parts in Union,
// Intersection or Conditional. Its FQN, Params and Shape then mirror the most
// useful part (a generic or a class over null, mixed or a scalar), so code that
// only reads one class name keeps working.
type ResolvedType struct {
	FQN      string         // base type FQN, e.g., "Illuminate\Database\Eloquent\Collection"
	Params   []ResolvedType // generic type parameters
	Nullable bool           // true if prefixed with ? or in a union with null
	Shape    string         // for array and object shapes: "array{id: int, name: string}" — takes priority over Params in String()

	Union        []ResolvedType // every part of a union but null, when there are two or more
	Intersection []ResolvedType // every part of an intersection
	Callable     *CallableType  // the signature of callable(…): T and Closure(…): T
	Fields       []ShapeEntry   // the fields of an array, list or object shape
	Literal      string         // the value of a literal type: true, 'foo', 42
	Conditional  *ConditionalType
	// Qualified is set when the name was written fully qualified, with a
	// leading backslash: it isn't relative to any namespace.
	Qualified bool
}

// CallableType is the signature of a callable or Closure type.
type CallableType struct {
	Params []ResolvedType
	Return *ResolvedType
}

// ShapeEntry is one field of an array or object shape.
type ShapeEntry struct {
	Key      string
	Type     ResolvedType
	Optional bool
}

// ConditionalType is a PHPDoc conditional type: ($subject is Target ? IfTrue : IfFalse).
// Subject is a parameter ($as_float) or a template name (T).
type ConditionalType struct {
	Subject string
	Target  ResolvedType
	Negated bool // `is not`
	IfTrue  ResolvedType
	IfFalse ResolvedType
}

// BaseFQN returns just the FQN without generic parameters.
func (rt ResolvedType) BaseFQN() string {
	return rt.FQN
}

// MemberOwner is what to look members up on: the class, or for a union or
// intersection of classes every one of them, as "A|B" or "A&B" (which the
// symbol index reads as the members of each).
func (rt ResolvedType) MemberOwner() string {
	sep := ""
	switch {
	case len(rt.Intersection) > 0:
		sep = "&"
	case len(rt.Union) > 0:
		sep = "|"
	default:
		return rt.FQN
	}
	names := rt.ClassNames()
	switch len(names) {
	case 0:
		return rt.FQN
	case 1:
		return names[0]
	}
	return strings.Join(names, sep)
}

// IsGeneric returns true if the type has generic parameters.
func (rt ResolvedType) IsGeneric() bool {
	return len(rt.Params) > 0
}

// IsEmpty returns true if the type has no FQN.
func (rt ResolvedType) IsEmpty() bool {
	return rt.FQN == ""
}

// String serializes the type back to a string like "Collection<int, Category>".
// For array shapes, outputs "array{key: type, ...}" instead of generic syntax.
// A union or intersection is shown as its most useful part; FullString shows
// all of it.
func (rt ResolvedType) String() string {
	single := rt
	single.Union, single.Intersection = nil, nil
	return single.FullString()
}

// writeUnionPart writes one part of a union or intersection. A callable with
// a return type is put in parentheses, as `(callable(T): float)|int`, since
// `callable(T): float|int` would read as returning float|int.
func writeUnionPart(sb *strings.Builder, part ResolvedType) {
	if part.Callable != nil && part.Callable.Return != nil && len(part.Union) == 0 && len(part.Intersection) == 0 {
		sb.WriteByte('(')
		sb.WriteString(part.FullString())
		sb.WriteByte(')')
		return
	}
	sb.WriteString(part.FullString())
}

// FullString is like String but shows every part of a union or intersection,
// as in "Foo|Bar|null".
func (rt ResolvedType) FullString() string {
	if len(rt.Union) > 0 {
		var sb strings.Builder
		for i, part := range rt.Union {
			if i > 0 {
				sb.WriteByte('|')
			}
			writeUnionPart(&sb, part)
		}
		if rt.Nullable {
			sb.WriteString("|null")
		}
		return sb.String()
	}
	if len(rt.Intersection) > 0 {
		var sb strings.Builder
		if rt.Nullable {
			sb.WriteString("?(")
		}
		for i, part := range rt.Intersection {
			if i > 0 {
				sb.WriteByte('&')
			}
			writeUnionPart(&sb, part)
		}
		if rt.Nullable {
			sb.WriteByte(')')
		}
		return sb.String()
	}
	if rt.FQN == "" {
		return ""
	}
	var sb strings.Builder
	if rt.Nullable {
		sb.WriteByte('?')
	}
	// Array shapes take priority: array{id: int, name: string}
	if rt.Shape != "" && (rt.FQN == "array" || rt.FQN == "object") {
		sb.WriteString(rt.Shape)
		return sb.String()
	}
	if rt.FQN == "offset-access" && len(rt.Params) == 2 {
		sb.WriteString(rt.Params[0].FullString())
		sb.WriteByte('[')
		sb.WriteString(rt.Params[1].FullString())
		sb.WriteByte(']')
		return sb.String()
	}
	sb.WriteString(rt.FQN)
	if rt.Callable != nil {
		sb.WriteByte('(')
		for i, p := range rt.Callable.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(p.FullString())
		}
		sb.WriteByte(')')
		if rt.Callable.Return != nil {
			sb.WriteString(": ")
			sb.WriteString(rt.Callable.Return.FullString())
		}
		return sb.String()
	}
	if len(rt.Params) > 0 {
		sb.WriteByte('<')
		for i, p := range rt.Params {
			if i > 0 {
				sb.WriteString(", ")
			}
			sb.WriteString(p.String())
		}
		sb.WriteByte('>')
	}
	return sb.String()
}

// ParseGenericType parses a type string like "Collection<int, Category>" into
// a ResolvedType. Handles nested generics and nullable prefix.
func ParseGenericType(raw string) ResolvedType {
	return resolvedTypeFromDocType(types.ParseTypeExpr(raw))
}

// InferArrayLiteralType analyzes a PHP array literal string and returns a
// ResolvedType with generic params or a shape string.
// Examples:
//
//	"['name' => 'John', 'age' => 30]" → array{name: string, age: int}
//	"[1, 2, 3]" → array<int, int>
//	"[['id' => 1], ['id' => 2]]" → array<int, array{id: int}>
//	"[]" → array (bare)
func InferArrayLiteralType(literal string) ResolvedType {
	return attachShapeFields(inferArrayLiteralType(literal))
}

// attachShapeFields fills in the structured fields of shapes that were built
// as strings, here and in the type's arguments.
func attachShapeFields(rt ResolvedType) ResolvedType {
	if rt.Shape != "" && len(rt.Fields) == 0 {
		if parsed := ParseGenericType(rt.Shape); len(parsed.Fields) > 0 {
			rt.Fields = parsed.Fields
		}
	}
	for i := range rt.Params {
		rt.Params[i] = attachShapeFields(rt.Params[i])
	}
	return rt
}

func inferArrayLiteralType(literal string) ResolvedType {
	literal = strings.TrimSpace(literal)
	if !strings.HasPrefix(literal, "[") || literal == "[]" {
		return ResolvedType{FQN: "array"}
	}

	// Parse the array structure
	fields := phparray.ParseLiteralToShape(literal)

	// If we got associative fields (key => value), it's a shape
	if len(fields) > 0 && fields[0].Key != "" {
		return ResolvedType{FQN: "array", Shape: buildShapeString(fields)}
	}

	// No associative fields — try as indexed array
	entries := phparray.ParseLiteralEntries(literal[1 : len(literal)-1])
	if len(entries) == 0 {
		// Non-associative: parse individual values
		return inferIndexedArrayType(literal)
	}

	// Associative but ParseLiteralToShape returned results
	return ResolvedType{FQN: "array", Shape: buildShapeString(fields)}
}

// inferIndexedArrayType infers the type of an indexed array like [1, 2, 3] or
// [['id' => 1], ['id' => 2]].
func inferIndexedArrayType(literal string) ResolvedType {
	if len(literal) < 2 {
		return ResolvedType{FQN: "array"}
	}
	content := strings.TrimSpace(literal[1 : len(literal)-1])
	if content == "" {
		return ResolvedType{FQN: "array"}
	}

	// Split top-level values (respecting nested brackets/strings)
	values := splitArrayValues(content)
	if len(values) == 0 {
		return ResolvedType{FQN: "array"}
	}

	// Infer the type of each value and find the common type
	var valueTypes []string
	var valueShape string
	allSameShape := true

	for _, v := range values {
		v = strings.TrimSpace(v)
		v = strings.TrimSuffix(v, ",")
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if strings.HasPrefix(v, "[") {
			// Nested array — try to infer shape
			inner := phparray.ParseLiteralToShape(v)
			if len(inner) > 0 && inner[0].Key != "" {
				shape := buildShapeString(inner)
				if valueShape == "" {
					valueShape = shape
				} else if valueShape != shape {
					allSameShape = false
				}
				valueTypes = append(valueTypes, shape)
				continue
			}
		}
		allSameShape = false
		valueTypes = append(valueTypes, phparray.InferValueType(v))
	}

	if len(valueTypes) == 0 {
		return ResolvedType{FQN: "array"}
	}

	// All nested arrays with the same shape
	if allSameShape && valueShape != "" {
		return ResolvedType{
			FQN:    "array",
			Params: []ResolvedType{{FQN: "int"}, {FQN: "array", Shape: valueShape}},
		}
	}

	// All same scalar type
	commonType := valueTypes[0]
	allSame := true
	for _, vt := range valueTypes[1:] {
		if vt != commonType {
			allSame = false
			break
		}
	}

	if allSame {
		return ResolvedType{
			FQN:    "array",
			Params: []ResolvedType{{FQN: "int"}, {FQN: commonType}},
		}
	}

	// Mixed types — return bare array
	return ResolvedType{FQN: "array"}
}

// buildShapeString builds "array{key: type, ...}" from shape fields.
func buildShapeString(fields []types.ShapeField) string {
	var parts []string
	for _, f := range fields {
		if f.Key != "" {
			parts = append(parts, f.Key+": "+f.Type)
		}
	}
	if len(parts) == 0 {
		return ""
	}
	return "array{" + strings.Join(parts, ", ") + "}"
}

// splitArrayValues splits top-level array values respecting nesting.
func splitArrayValues(content string) []string {
	var values []string
	depth := 0
	inString := byte(0)
	start := 0
	for i := 0; i < len(content); i++ {
		ch := content[i]
		if inString != 0 {
			if ch == inString && (i == 0 || content[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '[', '(':
			depth++
		case ']', ')':
			depth--
		case ',':
			if depth == 0 {
				values = append(values, strings.TrimSpace(content[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(content[start:]); tail != "" {
		values = append(values, tail)
	}
	return values
}

func resolvedTypeFromDocType(t types.TypeExpr) ResolvedType {
	if t.String() == "" {
		return ResolvedType{}
	}

	switch t.Kind {
	case types.TypeKindUnion:
		parts := make([]ResolvedType, 0, len(t.Types))
		for _, part := range t.Types {
			parts = append(parts, resolvedTypeFromDocType(part))
		}
		return MakeUnion(parts, t.Nullable)
	case types.TypeKindIntersection:
		parts := make([]ResolvedType, 0, len(t.Types))
		for _, part := range t.Types {
			if rt := resolvedTypeFromDocType(part); !rt.IsEmpty() {
				parts = append(parts, rt)
			}
		}
		return makeIntersection(parts, t.Nullable)
	case types.TypeKindGeneric:
		rt := ResolvedType{FQN: strings.TrimPrefix(t.Name, "\\"), Nullable: t.Nullable, Qualified: strings.HasPrefix(t.Name, "\\")}
		for _, param := range t.Params {
			rt.Params = append(rt.Params, resolvedTypeFromDocType(param))
		}
		return rt
	case types.TypeKindShape:
		fields := make([]ShapeEntry, 0, len(t.ShapeFields))
		for _, f := range t.ShapeFields {
			fields = append(fields, ShapeEntry{Key: f.Key, Type: resolvedTypeFromDocType(f.Type), Optional: f.Optional})
		}
		if t.Name == "array" || t.Name == "list" {
			return ResolvedType{FQN: "array", Shape: t.String(), Nullable: t.Nullable, Fields: fields}
		}
		shape := t
		shape.Nullable = false
		return ResolvedType{FQN: "object", Shape: shape.String(), Nullable: t.Nullable, Fields: fields}
	case types.TypeKindArray:
		elem := ResolvedType{FQN: "mixed"}
		if t.Element != nil {
			elem = resolvedTypeFromDocType(*t.Element)
		}
		return ResolvedType{
			FQN:      "array",
			Nullable: t.Nullable,
			Params:   []ResolvedType{{FQN: "int"}, elem},
		}
	case types.TypeKindCallable:
		name := strings.TrimPrefix(t.Name, "\\")
		if name == "" {
			name = "callable"
		}
		rt := ResolvedType{FQN: name, Nullable: t.Nullable}
		if t.ParamTypes != nil || t.ReturnType != nil {
			sig := &CallableType{}
			for _, p := range t.ParamTypes {
				sig.Params = append(sig.Params, resolvedTypeFromDocType(p))
			}
			if t.ReturnType != nil {
				ret := resolvedTypeFromDocType(*t.ReturnType)
				sig.Return = &ret
			}
			rt.Callable = sig
		}
		return rt
	case types.TypeKindLiteral:
		switch strings.ToLower(t.Literal) {
		case "true", "false":
			return ResolvedType{FQN: "bool", Nullable: t.Nullable, Literal: strings.ToLower(t.Literal)}
		case "null":
			return ResolvedType{Nullable: true}
		}
		if strings.HasPrefix(t.Literal, "'") || strings.HasPrefix(t.Literal, "\"") {
			return ResolvedType{FQN: "string", Nullable: t.Nullable, Literal: t.Literal}
		}
		if strings.Contains(t.Literal, ".") {
			return ResolvedType{FQN: "float", Nullable: t.Nullable, Literal: t.Literal}
		}
		return ResolvedType{FQN: "int", Nullable: t.Nullable, Literal: t.Literal}
	case types.TypeKindOffset:
		if t.Element == nil || len(t.Params) != 1 {
			return ResolvedType{}
		}
		return ResolvedType{FQN: "offset-access", Nullable: t.Nullable, Params: []ResolvedType{
			resolvedTypeFromDocType(*t.Element), resolvedTypeFromDocType(t.Params[0]),
		}}
	case types.TypeKindConditional:
		if t.Condition == nil {
			return ResolvedType{}
		}
		cond := &ConditionalType{Subject: t.Condition.Subject}
		target := t.Condition.Target
		if target.Kind == types.TypeKindNamed && strings.HasPrefix(target.Name, "not ") {
			cond.Negated = true
			target = types.ParseTypeExpr(strings.TrimSpace(target.Name[4:]))
		}
		cond.Target = resolvedTypeFromDocType(target)
		cond.IfTrue = resolvedTypeFromDocType(t.Condition.IfTrue)
		cond.IfFalse = resolvedTypeFromDocType(t.Condition.IfFalse)
		rt := MakeUnion([]ResolvedType{cond.IfTrue, cond.IfFalse}, t.Nullable)
		rt.Conditional = cond
		return rt
	default:
		name := t.Name
		switch strings.ToLower(name) {
		case "true", "false":
			return ResolvedType{FQN: "bool", Nullable: t.Nullable, Literal: strings.ToLower(name)}
		case "null":
			return ResolvedType{Nullable: true}
		case "*":
			// A wildcard type argument: Collection<*>.
			return ResolvedType{FQN: "mixed"}
		}
		// Call-site variance: Collection<covariant Animal> reads as Animal.
		for _, prefix := range []string{"covariant ", "contravariant "} {
			if strings.HasPrefix(name, prefix) {
				rt := resolvedTypeFromDocType(types.ParseTypeExpr(strings.TrimSpace(name[len(prefix):])))
				rt.Nullable = rt.Nullable || t.Nullable
				return rt
			}
		}
		if strings.HasPrefix(name, "'") || strings.HasPrefix(name, "\"") {
			return ResolvedType{FQN: "string", Nullable: t.Nullable, Literal: name}
		}
		return ResolvedType{FQN: strings.TrimPrefix(name, "\\"), Nullable: t.Nullable, Qualified: strings.HasPrefix(name, "\\")}
	}
}

// MakeUnion builds the union of parts. Nested unions are flattened, null parts
// make it nullable, and repeated parts are dropped. With one part left, that
// part is returned; with several, the result mirrors the most useful one.
func MakeUnion(parts []ResolvedType, nullable bool) ResolvedType {
	flat := make([]ResolvedType, 0, len(parts))
	seen := make(map[string]bool, len(parts))
	var add func(rt ResolvedType)
	add = func(rt ResolvedType) {
		if len(rt.Union) > 0 && rt.Conditional == nil {
			if rt.Nullable {
				nullable = true
			}
			for _, part := range rt.Union {
				add(part)
			}
			return
		}
		if rt.IsEmpty() && len(rt.Intersection) == 0 {
			if rt.Nullable {
				nullable = true
			}
			return
		}
		if rt.FQN == "null" {
			nullable = true
			return
		}
		if rt.Nullable {
			nullable = true
			rt.Nullable = false
		}
		key := rt.FullString()
		if seen[key] {
			return
		}
		seen[key] = true
		flat = append(flat, rt)
	}
	for _, part := range parts {
		add(part)
	}
	switch len(flat) {
	case 0:
		if nullable {
			return ResolvedType{Nullable: true}
		}
		return ResolvedType{}
	case 1:
		out := flat[0]
		out.Nullable = out.Nullable || nullable
		return out
	}
	// mixed absorbs everything else.
	for _, part := range flat {
		if part.FQN == "mixed" && len(part.Union) == 0 {
			return ResolvedType{FQN: "mixed"}
		}
	}
	best := pickBestResolved(flat)
	best.Union = flat
	best.Nullable = nullable
	best.Conditional = nil
	return best
}

func makeIntersection(parts []ResolvedType, nullable bool) ResolvedType {
	switch len(parts) {
	case 0:
		return ResolvedType{}
	case 1:
		out := parts[0]
		out.Nullable = out.Nullable || nullable
		return out
	}
	best := pickBestResolved(parts)
	best.Intersection = parts
	best.Nullable = nullable
	return best
}

// pickBestResolved picks the part a single-class reader should see: the first
// generic or shape, else the first that isn't mixed or void.
func pickBestResolved(parts []ResolvedType) ResolvedType {
	var fallback ResolvedType
	for _, rt := range parts {
		if rt.IsEmpty() {
			continue
		}
		if rt.Shape != "" || len(rt.Params) > 0 {
			return rt
		}
		if fallback.IsEmpty() && rt.FQN != "mixed" && rt.FQN != "void" {
			fallback = rt
		}
	}
	if fallback.IsEmpty() && len(parts) > 0 {
		return parts[0]
	}
	return fallback
}

// Parts returns the members of a union, or the type itself.
func (rt ResolvedType) Parts() []ResolvedType {
	if len(rt.Union) > 0 {
		return rt.Union
	}
	if rt.IsEmpty() {
		return nil
	}
	single := rt
	single.Nullable = false
	return []ResolvedType{single}
}

// ClassNames returns the class names a value of this type can be: every part
// of a union or intersection that isn't a builtin type.
func (rt ResolvedType) ClassNames() []string {
	var out []string
	var walk func(t ResolvedType)
	walk = func(t ResolvedType) {
		switch {
		case len(t.Union) > 0:
			for _, p := range t.Union {
				walk(p)
			}
		case len(t.Intersection) > 0:
			for _, p := range t.Intersection {
				walk(p)
			}
		case t.FQN != "" && !IsBuiltinTypeName(t.FQN):
			out = append(out, t.FQN)
		}
	}
	walk(rt)
	return out
}
