package resolve

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/phparray"
	"github.com/open-southeners/tusk-php/internal/types"
)

// ResolvedType represents a type with optional generic parameters.
// For example, Collection<int, Category> is {FQN: "Illuminate\...\Collection", Params: [{FQN: "int"}, {FQN: "App\Models\Category"}]}.
type ResolvedType struct {
	FQN      string         // base type FQN, e.g., "Illuminate\Database\Eloquent\Collection"
	Params   []ResolvedType // generic type parameters
	Nullable bool           // true if prefixed with ?
	Shape    string         // for array shapes: "array{id: int, name: string}" — takes priority over Params in String()
}

// BaseFQN returns just the FQN without generic parameters.
func (rt ResolvedType) BaseFQN() string {
	return rt.FQN
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
func (rt ResolvedType) String() string {
	if rt.FQN == "" {
		return ""
	}
	var sb strings.Builder
	if rt.Nullable {
		sb.WriteByte('?')
	}
	// Array shapes take priority: array{id: int, name: string}
	if rt.Shape != "" && rt.FQN == "array" {
		sb.WriteString(rt.Shape)
		return sb.String()
	}
	sb.WriteString(rt.FQN)
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
		nullable := false
		for _, part := range t.Types {
			if strings.EqualFold(part.Name, "null") {
				nullable = true
			}
		}
		best := pickBestResolvedBranch(t.Types)
		best.Nullable = best.Nullable || nullable
		return best
	case types.TypeKindIntersection:
		return pickBestResolvedBranch(t.Types)
	case types.TypeKindGeneric:
		rt := ResolvedType{FQN: strings.TrimPrefix(t.Name, "\\"), Nullable: t.Nullable}
		for _, param := range t.Params {
			rt.Params = append(rt.Params, resolvedTypeFromDocType(param))
		}
		return rt
	case types.TypeKindShape:
		if t.Name == "array" || t.Name == "list" {
			return ResolvedType{FQN: "array", Shape: t.String(), Nullable: t.Nullable}
		}
		return ResolvedType{FQN: "object", Nullable: t.Nullable}
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
		return ResolvedType{FQN: "callable", Nullable: t.Nullable}
	case types.TypeKindLiteral:
		switch strings.ToLower(t.Literal) {
		case "true", "false":
			return ResolvedType{FQN: "bool", Nullable: t.Nullable}
		case "null":
			return ResolvedType{Nullable: true}
		}
		if strings.HasPrefix(t.Literal, "'") || strings.HasPrefix(t.Literal, "\"") {
			return ResolvedType{FQN: "string", Nullable: t.Nullable}
		}
		if strings.Contains(t.Literal, ".") {
			return ResolvedType{FQN: "float", Nullable: t.Nullable}
		}
		return ResolvedType{FQN: "int", Nullable: t.Nullable}
	case types.TypeKindConditional:
		if t.Condition == nil {
			return ResolvedType{}
		}
		return resolvedTypeFromDocType(types.TypeExpr{
			Kind:  types.TypeKindUnion,
			Types: []types.TypeExpr{t.Condition.IfTrue, t.Condition.IfFalse},
		})
	default:
		switch strings.ToLower(t.Name) {
		case "true", "false":
			return ResolvedType{FQN: "bool", Nullable: t.Nullable}
		case "null":
			return ResolvedType{Nullable: true}
		}
		if strings.HasPrefix(t.Name, "'") || strings.HasPrefix(t.Name, "\"") {
			return ResolvedType{FQN: "string", Nullable: t.Nullable}
		}
		return ResolvedType{FQN: strings.TrimPrefix(t.Name, "\\"), Nullable: t.Nullable}
	}
}

func pickBestResolvedBranch(parts []types.TypeExpr) ResolvedType {
	var fallback ResolvedType
	for _, part := range parts {
		rt := resolvedTypeFromDocType(part)
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
	return fallback
}
