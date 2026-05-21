package types

import "strings"

type TypeKind int

const (
	TypeKindNamed TypeKind = iota
	TypeKindUnion
	TypeKindIntersection
	TypeKindGeneric
	TypeKindConditional
	TypeKindShape
	TypeKindArray
	TypeKindCallable
	TypeKindLiteral
)

// TypeExpr is a small structured PHPDoc type model shared by hover/completion.
// It currently focuses on round-tripping structured display output rather than
// full semantic evaluation.
type TypeExpr struct {
	Kind        TypeKind
	Name        string
	Nullable    bool
	Wrapped     bool
	Literal     string
	Element     *TypeExpr
	Params      []TypeExpr
	Types       []TypeExpr
	ShapeFields []ShapeExprField
	ParamTypes  []TypeExpr
	ReturnType  *TypeExpr
	Condition   *ConditionalType
}

type Type = TypeExpr

type ShapeExprField struct {
	Key      string
	Type     TypeExpr
	Optional bool
}

type ConditionalType struct {
	Subject string
	Target  TypeExpr
	IfTrue  TypeExpr
	IfFalse TypeExpr
}

// ShapeField represents a single field in an array shape: array{name: string, age?: int}
type ShapeField struct {
	Key      string
	Type     string
	Optional bool
}

// ParseTypeExpr parses a PHPDoc type string into a structured expression.
func ParseTypeExpr(raw string) TypeExpr {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return TypeExpr{}
	}
	return parseTypeExpr(raw)
}

func Parse(raw string) *Type {
	expr := ParseTypeExpr(raw)
	if expr.String() == "" {
		return nil
	}
	return &expr
}

// RenderTypeExpr renders a PHPDoc type expression in a normalized form.
func RenderTypeExpr(raw string) string {
	return ParseTypeExpr(raw).String()
}

func (expr TypeExpr) String() string {
	if expr.Kind == TypeKindNamed && expr.Name == "" && !expr.Nullable {
		return ""
	}

	var out string
	switch expr.Kind {
	case TypeKindUnion:
		out = joinTypeExprs(expr.Types, " | ")
	case TypeKindIntersection:
		out = joinTypeExprs(expr.Types, " & ")
	case TypeKindGeneric:
		out = trimLeadingSlash(expr.Name)
		if len(expr.Params) > 0 {
			out += "<" + joinTypeExprs(expr.Params, ", ") + ">"
		}
	case TypeKindConditional:
		if expr.Condition != nil {
			out = expr.Condition.Subject + " is " + expr.Condition.Target.String() + " ? " + expr.Condition.IfTrue.String() + " : " + expr.Condition.IfFalse.String()
		}
	case TypeKindShape:
		out = trimLeadingSlash(expr.Name)
		var parts []string
		for _, field := range expr.ShapeFields {
			key := field.Key
			if key != "" {
				if field.Optional {
					key += "?"
				}
				parts = append(parts, key+": "+field.Type.String())
				continue
			}
			parts = append(parts, field.Type.String())
		}
		out += "{" + strings.Join(parts, ", ") + "}"
	case TypeKindArray:
		if expr.Element != nil {
			out = expr.Element.String() + "[]"
		}
	case TypeKindCallable:
		out = trimLeadingSlash(expr.Name)
		if out == "" {
			out = "callable"
		}
		if expr.ParamTypes != nil || expr.ReturnType != nil {
			out += "(" + joinTypeExprs(expr.ParamTypes, ", ") + ")"
			if expr.ReturnType != nil {
				out += ": " + expr.ReturnType.String()
			}
		}
	case TypeKindLiteral:
		out = expr.Literal
	default:
		out = trimLeadingSlash(expr.Name)
	}

	if expr.Wrapped && out != "" {
		out = "(" + out + ")"
	}
	if expr.Nullable && out != "" {
		out = "?" + out
	}
	return out
}

// ParseArrayShape extracts shape fields from a PHPStan array shape type string.
// Input: "array{name: string, age: int, address?: string}"
// Returns the fields, or nil if the type is not a shape.
func ParseArrayShape(typeStr string) []ShapeField {
	typeStr = trimNullable(typeStr)

	// Handle union types — find the array shape part
	if idx := findArrayShapeInUnion(typeStr); idx >= 0 {
		typeStr = typeStr[idx:]
		// Find the end of this shape
		end := findShapeEnd(typeStr)
		if end > 0 {
			typeStr = typeStr[:end]
		}
	}

	// Must start with "array{" or "list{"
	braceStart := -1
	if len(typeStr) > 6 && typeStr[:6] == "array{" {
		braceStart = 5
	} else if len(typeStr) > 5 && typeStr[:5] == "list{" {
		braceStart = 4
	}
	if braceStart < 0 {
		return nil
	}

	// Extract content between { and }
	inner := typeStr[braceStart+1:]
	if len(inner) == 0 || inner[len(inner)-1] != '}' {
		return nil
	}
	inner = inner[:len(inner)-1]
	if inner == "" {
		return nil
	}

	return parseShapeFields(inner)
}

// parseShapeFields splits "name: string, age?: int" into fields,
// respecting nested braces, angle brackets, and parentheses.
func parseShapeFields(inner string) []ShapeField {
	var fields []ShapeField
	depth := 0 // tracks {, <, (
	start := 0

	for i := 0; i < len(inner); i++ {
		switch inner[i] {
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ',':
			if depth == 0 {
				if f := parseOneField(inner[start:i]); f != nil {
					fields = append(fields, *f)
				}
				start = i + 1
			}
		}
	}
	// Last field
	if start < len(inner) {
		if f := parseOneField(inner[start:]); f != nil {
			fields = append(fields, *f)
		}
	}
	return fields
}

// parseOneField parses "name: string" or "age?: int" into a ShapeField.
func parseOneField(s string) *ShapeField {
	s = trim(s)
	if s == "" {
		return nil
	}

	// Find the colon separator (respecting nested types)
	colonIdx := -1
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ':':
			if depth == 0 {
				colonIdx = i
				goto found
			}
		}
	}
found:
	if colonIdx < 0 {
		// Positional field (no key name), e.g. "string" in array{string, int}
		return &ShapeField{Type: trim(s)}
	}

	key := trim(s[:colonIdx])
	typ := trim(s[colonIdx+1:])

	optional := false
	if len(key) > 0 && key[len(key)-1] == '?' {
		optional = true
		key = key[:len(key)-1]
	}

	// Strip quotes from key if present: 'key-name' or "key-name"
	if len(key) >= 2 && (key[0] == '\'' || key[0] == '"') && key[len(key)-1] == key[0] {
		key = key[1 : len(key)-1]
	}

	return &ShapeField{Key: key, Type: typ, Optional: optional}
}

// ExtractDocTypeString extracts a type string from a docblock value,
// handling nested braces/angles so "array{name: string, age: int} $var desc"
// correctly returns ("array{name: string, age: int}", "$var desc").
func ExtractDocTypeString(value string) (typeStr, rest string) {
	value = trim(value)
	if value == "" {
		return "", ""
	}

	depth := 0
	inString := byte(0)
	i := 0
	for i < len(value) {
		ch := value[i]
		if inString != 0 {
			if ch == inString && (i == 0 || value[i-1] != '\\') {
				inString = 0
			}
			i++
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ' ', '\t':
			if depth == 0 && !continuesTypeExpr(value, i) {
				return value[:i], trim(value[i:])
			}
		}
		i++
	}
	return value, ""
}

func parseTypeExpr(raw string) TypeExpr {
	nullable := false
	if strings.HasPrefix(raw, "?") {
		nullable = true
		raw = strings.TrimSpace(raw[1:])
	}

	if inner, ok := unwrapOuterParens(raw); ok {
		expr := parseTypeExpr(inner)
		expr.Nullable = expr.Nullable || nullable
		expr.Wrapped = true
		return expr
	}

	if parts := splitTopLevel(raw, '|'); len(parts) > 1 {
		return TypeExpr{Kind: TypeKindUnion, Nullable: nullable, Types: parseTypeExprs(parts)}
	}
	if parts := splitTopLevel(raw, '&'); len(parts) > 1 {
		return TypeExpr{Kind: TypeKindIntersection, Nullable: nullable, Types: parseTypeExprs(parts)}
	}
	if cond, ok := parseConditionalType(raw); ok {
		return TypeExpr{Kind: TypeKindConditional, Nullable: nullable, Condition: cond}
	}
	if callable, ok := parseCallableExpr(raw, nullable); ok {
		return callable
	}
	if shape, ok := parseShapeExpr(raw, nullable); ok {
		return shape
	}
	if strings.HasSuffix(raw, "[]") && len(raw) > 2 {
		elem := parseTypeExpr(strings.TrimSpace(raw[:len(raw)-2]))
		return TypeExpr{Kind: TypeKindArray, Nullable: nullable, Element: &elem}
	}
	if isLiteralType(raw) {
		return TypeExpr{Kind: TypeKindLiteral, Nullable: nullable, Literal: raw}
	}
	if name, params, ok := parseGenericExpr(raw); ok {
		return TypeExpr{Kind: TypeKindGeneric, Nullable: nullable, Name: name, Params: params}
	}

	return TypeExpr{Kind: TypeKindNamed, Nullable: nullable, Name: raw}
}

// ExpandAliases substitutes named aliases inside a structured type expression.
func ExpandAliases(expr *Type, aliases map[string]*Type) *Type {
	if expr == nil {
		return nil
	}
	if len(aliases) == 0 {
		cloned := *expr
		return &cloned
	}
	return expandAliases(expr, aliases, map[string]bool{})
}

func parseTypeExprs(parts []string) []TypeExpr {
	out := make([]TypeExpr, 0, len(parts))
	for _, part := range parts {
		out = append(out, parseTypeExpr(part))
	}
	return out
}

func joinTypeExprs(parts []TypeExpr, sep string) string {
	rendered := make([]string, 0, len(parts))
	for _, part := range parts {
		rendered = append(rendered, part.String())
	}
	return strings.Join(rendered, sep)
}

func parseCallableExpr(raw string, nullable bool) (TypeExpr, bool) {
	name := ""
	switch {
	case raw == "callable":
		return TypeExpr{Kind: TypeKindCallable, Nullable: nullable, Name: "callable"}, true
	case strings.HasPrefix(raw, "callable("):
		name = "callable"
	case strings.HasPrefix(raw, "Closure("):
		name = "Closure"
	case strings.HasPrefix(raw, "\\Closure("):
		name = "\\Closure"
	default:
		return TypeExpr{}, false
	}

	open := strings.IndexByte(raw, '(')
	close := findMatchingParen(raw, open)
	if open < 0 || close < 0 {
		return TypeExpr{}, false
	}

	expr := TypeExpr{Kind: TypeKindCallable, Nullable: nullable, Name: name, ParamTypes: []TypeExpr{}}
	paramsStr := strings.TrimSpace(raw[open+1 : close])
	if paramsStr != "" {
		expr.ParamTypes = parseTypeExprs(splitTopLevelByComma(paramsStr))
	}

	rest := strings.TrimSpace(raw[close+1:])
	if strings.HasPrefix(rest, ":") {
		ret := parseTypeExpr(strings.TrimSpace(rest[1:]))
		expr.ReturnType = &ret
	}
	return expr, true
}

func findMatchingParen(s string, open int) int {
	if open < 0 || open >= len(s) || s[open] != '(' {
		return -1
	}
	depth := 0
	inString := byte(0)
	for i := open; i < len(s); i++ {
		ch := s[i]
		if inString != 0 {
			if ch == inString && (i == 0 || s[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func parseConditionalType(raw string) (*ConditionalType, bool) {
	qIdx := indexTopLevel(raw, '?')
	if qIdx < 0 {
		return nil, false
	}
	colonIdx := indexTopLevel(raw[qIdx+1:], ':')
	if colonIdx < 0 {
		return nil, false
	}
	colonIdx += qIdx + 1

	left := strings.TrimSpace(raw[:qIdx])
	isIdx := strings.LastIndex(left, " is ")
	if isIdx < 0 {
		return nil, false
	}

	subject := strings.TrimSpace(left[:isIdx])
	target := strings.TrimSpace(left[isIdx+4:])
	if subject == "" || target == "" {
		return nil, false
	}

	return &ConditionalType{
		Subject: subject,
		Target:  parseTypeExpr(target),
		IfTrue:  parseTypeExpr(strings.TrimSpace(raw[qIdx+1 : colonIdx])),
		IfFalse: parseTypeExpr(strings.TrimSpace(raw[colonIdx+1:])),
	}, true
}

func expandAliases(expr *Type, aliases map[string]*Type, seen map[string]bool) *Type {
	if expr == nil {
		return nil
	}

	switch expr.Kind {
	case TypeKindNamed:
		if replacement, ok := aliases[expr.Name]; ok && replacement != nil && !seen[expr.Name] {
			seen[expr.Name] = true
			expanded := expandAliases(replacement, aliases, seen)
			delete(seen, expr.Name)
			if expanded == nil {
				return cloneType(expr)
			}
			if expr.Nullable && !expanded.Nullable {
				out := *expanded
				out.Nullable = true
				return &out
			}
			if expr.Wrapped && !expanded.Wrapped {
				out := *expanded
				out.Wrapped = true
				return &out
			}
			return expanded
		}
	case TypeKindGeneric:
		if replacement, ok := aliases[expr.Name]; ok && replacement != nil && !seen[expr.Name] {
			seen[expr.Name] = true
			base := expandAliases(replacement, aliases, seen)
			delete(seen, expr.Name)
			if base != nil {
				out := *base
				out.Params = append(append([]TypeExpr(nil), out.Params...), expr.Params...)
				if expr.Nullable {
					out.Nullable = true
				}
				if expr.Wrapped {
					out.Wrapped = true
				}
				for i := range out.Params {
					out.Params[i] = derefType(expandAliases(&out.Params[i], aliases, seen))
				}
				return &out
			}
		}
	}

	out := *expr
	if len(expr.Params) > 0 {
		out.Params = make([]TypeExpr, len(expr.Params))
		for i := range expr.Params {
			out.Params[i] = derefType(expandAliases(&expr.Params[i], aliases, seen))
		}
	}
	if len(expr.Types) > 0 {
		out.Types = make([]TypeExpr, len(expr.Types))
		for i := range expr.Types {
			out.Types[i] = derefType(expandAliases(&expr.Types[i], aliases, seen))
		}
	}
	if len(expr.ShapeFields) > 0 {
		out.ShapeFields = make([]ShapeExprField, len(expr.ShapeFields))
		for i := range expr.ShapeFields {
			out.ShapeFields[i] = expr.ShapeFields[i]
			out.ShapeFields[i].Type = derefType(expandAliases(&expr.ShapeFields[i].Type, aliases, seen))
		}
	}
	if expr.Condition != nil {
		out.Condition = &ConditionalType{
			Subject: expr.Condition.Subject,
			Target:  derefType(expandAliases(&expr.Condition.Target, aliases, seen)),
			IfTrue:  derefType(expandAliases(&expr.Condition.IfTrue, aliases, seen)),
			IfFalse: derefType(expandAliases(&expr.Condition.IfFalse, aliases, seen)),
		}
	}
	if expr.Element != nil {
		out.Element = expandAliases(expr.Element, aliases, seen)
	}
	if len(expr.ParamTypes) > 0 {
		out.ParamTypes = make([]TypeExpr, len(expr.ParamTypes))
		for i := range expr.ParamTypes {
			out.ParamTypes[i] = derefType(expandAliases(&expr.ParamTypes[i], aliases, seen))
		}
	}
	if expr.ReturnType != nil {
		out.ReturnType = expandAliases(expr.ReturnType, aliases, seen)
	}
	return &out
}

func cloneType(expr *Type) *Type {
	if expr == nil {
		return nil
	}
	out := *expr
	return &out
}

func derefType(expr *Type) TypeExpr {
	if expr == nil {
		return TypeExpr{}
	}
	return *expr
}

func parseShapeExpr(raw string, nullable bool) (TypeExpr, bool) {
	for _, prefix := range []string{"array", "list", "object"} {
		if !strings.HasPrefix(raw, prefix+"{") {
			continue
		}
		if !strings.HasSuffix(raw, "}") {
			return TypeExpr{}, false
		}
		end := findShapeEnd(raw[len(prefix):])
		if end != len(raw)-len(prefix) {
			return TypeExpr{}, false
		}
		inner := raw[len(prefix)+1 : len(raw)-1]
		fields := parseShapeExprFields(inner)
		return TypeExpr{Kind: TypeKindShape, Nullable: nullable, Name: prefix, ShapeFields: fields}, true
	}
	return TypeExpr{}, false
}

func parseShapeExprFields(inner string) []ShapeExprField {
	parts := splitTopLevelByComma(inner)
	fields := make([]ShapeExprField, 0, len(parts))
	for _, part := range parts {
		part = trim(part)
		if part == "" {
			continue
		}
		key, typ, ok := splitShapeField(part)
		if !ok {
			fields = append(fields, ShapeExprField{Type: parseTypeExpr(part)})
			continue
		}
		optional := false
		key = trim(key)
		if strings.HasSuffix(key, "?") {
			optional = true
			key = trim(strings.TrimSuffix(key, "?"))
		}
		if len(key) >= 2 && (key[0] == '\'' || key[0] == '"') && key[len(key)-1] == key[0] {
			key = key[1 : len(key)-1]
		}
		fields = append(fields, ShapeExprField{
			Key:      key,
			Optional: optional,
			Type:     parseTypeExpr(trim(typ)),
		})
	}
	return fields
}

func splitShapeField(part string) (string, string, bool) {
	depth := 0
	inString := byte(0)
	for i := 0; i < len(part); i++ {
		ch := part[i]
		if inString != 0 {
			if ch == inString && (i == 0 || part[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ':':
			if depth == 0 {
				return part[:i], part[i+1:], true
			}
		}
	}
	return "", "", false
}

func parseGenericExpr(raw string) (string, []TypeExpr, bool) {
	openIdx := strings.IndexByte(raw, '<')
	if openIdx <= 0 || raw[len(raw)-1] != '>' {
		return "", nil, false
	}
	name := trimLeadingSlash(strings.TrimSpace(raw[:openIdx]))
	paramsStr := raw[openIdx+1 : len(raw)-1]
	params := splitTopLevelByComma(paramsStr)
	if len(params) == 0 {
		return name, nil, true
	}
	return name, parseTypeExprs(params), true
}

func isLiteralType(raw string) bool {
	if raw == "true" || raw == "false" || raw == "null" {
		return true
	}
	if len(raw) >= 2 && (raw[0] == '\'' || raw[0] == '"') && raw[len(raw)-1] == raw[0] {
		return true
	}
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if (ch < '0' || ch > '9') && ch != '.' && ch != '-' {
			return false
		}
	}
	return raw != "" && raw != "-"
}

func unwrapOuterParens(raw string) (string, bool) {
	if len(raw) < 2 || raw[0] != '(' || raw[len(raw)-1] != ')' {
		return "", false
	}
	depth := 0
	inString := byte(0)
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		if inString != 0 {
			if ch == inString && (i == 0 || raw[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 && i != len(raw)-1 {
				return "", false
			}
		}
	}
	return strings.TrimSpace(raw[1 : len(raw)-1]), true
}

func splitTopLevelByComma(s string) []string {
	var parts []string
	depth := 0
	inString := byte(0)
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString != 0 {
			if ch == inString && (i == 0 || s[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if tail := strings.TrimSpace(s[start:]); tail != "" {
		parts = append(parts, tail)
	}
	return parts
}

func splitTopLevel(s string, sep byte) []string {
	var parts []string
	depth := 0
	inString := byte(0)
	start := 0
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString != 0 {
			if ch == inString && (i == 0 || s[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		default:
			if ch == sep && depth == 0 {
				parts = append(parts, strings.TrimSpace(s[start:i]))
				start = i + 1
			}
		}
	}
	if start == 0 {
		return nil
	}
	parts = append(parts, strings.TrimSpace(s[start:]))
	return parts
}

func indexTopLevel(s string, target byte) int {
	depth := 0
	inString := byte(0)
	for i := 0; i < len(s); i++ {
		ch := s[i]
		if inString != 0 {
			if ch == inString && (i == 0 || s[i-1] != '\\') {
				inString = 0
			}
			continue
		}
		switch ch {
		case '\'', '"':
			inString = ch
		case '{', '<', '(':
			depth++
		case '}', '>', ')':
			depth--
		default:
			if ch == target && depth == 0 {
				return i
			}
		}
	}
	return -1
}

func continuesTypeExpr(value string, ws int) bool {
	j := ws
	for j < len(value) && (value[j] == ' ' || value[j] == '\t') {
		j++
	}
	if j >= len(value) {
		return false
	}

	switch value[j] {
	case '|', '&', '?', ':':
		return true
	}
	if j+1 < len(value) && value[j] == 'i' && value[j+1] == 's' && isTypeBoundary(value, j+2) {
		return true
	}

	k := ws - 1
	for k >= 0 && (value[k] == ' ' || value[k] == '\t') {
		k--
	}
	if k >= 0 {
		switch value[k] {
		case '|', '&', '?', ':':
			return true
		}
	}
	return false
}

func isTypeBoundary(s string, idx int) bool {
	if idx >= len(s) {
		return true
	}
	return s[idx] == ' ' || s[idx] == '\t'
}

func trimNullable(s string) string {
	s = trim(s)
	if len(s) > 0 && s[0] == '?' {
		return s[1:]
	}
	return s
}

func findArrayShapeInUnion(s string) int {
	// Look for "array{" or "list{" in a union like "string|array{key: int}"
	for i := 0; i < len(s)-5; i++ {
		if s[i:i+6] == "array{" || s[i:i+5] == "list{" {
			return i
		}
	}
	return -1
}

func findShapeEnd(s string) int {
	depth := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return i + 1
			}
		}
	}
	return len(s)
}

func trimLeadingSlash(s string) string {
	return strings.TrimPrefix(strings.TrimSpace(s), "\\")
}

func trim(s string) string {
	start := 0
	end := len(s)
	for start < end && (s[start] == ' ' || s[start] == '\t') {
		start++
	}
	for end > start && (s[end-1] == ' ' || s[end-1] == '\t') {
		end--
	}
	return s[start:end]
}
