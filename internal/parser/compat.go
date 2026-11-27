package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// Compatibility types kept for older callers that still expect the previous AST shape.
type TypeNode struct {
	Name string
}

type UseNode struct {
	FullName  string
	Alias     string
	Kind      string
	StartLine int
}

type AttributeDef struct {
	Name string
	Args AttributeArgs
	Line int
}

type AttributeArgs []AttributeArg

type AttributeArg struct {
	Name  string
	Value string
}

func (a AttributeArg) String() string {
	if a.Name == "" {
		return a.Value
	}
	return a.Name + ":" + a.Value
}

func (args AttributeArgs) String() string {
	parts := make([]string, len(args))
	for i, arg := range args {
		parts[i] = arg.String()
	}
	return strings.Join(parts, ", ")
}

func (args AttributeArgs) Get(name string, position int) (string, bool) {
	for _, arg := range args {
		if arg.Name == name {
			return arg.Value, true
		}
	}
	if position >= 0 && position < len(args) && args[position].Name == "" {
		return args[position].Value, true
	}
	return "", false
}

type ParamNode struct {
	Name         string
	Type         TypeNode
	DefaultValue string
	IsVariadic   bool
	IsReference  bool
	// DocComment is the parameter's own doc comment (PHP 8.6), written
	// before it or after it before the comma.
	DocComment string
}

type PropertyNode struct {
	Name          string
	Type          TypeNode
	DefaultValue  string
	Visibility    string
	SetVisibility string // PHP 8.4 asymmetric visibility (e.g. "private" in public private(set))
	IsStatic      bool
	IsReadonly    bool
	Attributes    []AttributeDef
	DocComment    string
	StartLine     int
	StartCol      int
	Hooks         []PropertyHook // PHP 8.4 property hooks (get/set accessors)
}

type MethodNode struct {
	Attributes []AttributeDef
	Name       string
	Params     []ParamNode
	ReturnType TypeNode
	Visibility string
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	DocComment string
	StartLine  int
	StartCol   int
	EndLine    int
}

type ConstantNode struct {
	Name       string
	Value      string
	Type       TypeNode
	Visibility string
	Attributes []AttributeDef
	DocComment string
	StartLine  int
	StartCol   int
}

type ClassNode struct {
	Attributes []AttributeDef
	Name       string
	FullName   string
	Extends    string
	Implements []string
	Traits     []string
	Properties []PropertyNode
	Methods    []MethodNode
	Constants  []ConstantNode
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	// Conditional is set for a class declared inside a block (see ClassDef).
	Conditional bool
	StartLine   int
	EndLine     int
	StartCol    int
	DocComment  string
	// TraitUseDoc holds the docblocks above the class's trait `use`
	// statements, where @use gives a trait its type arguments.
	TraitUseDoc string
	TraitRules  TraitAdaptations
	// BodyLine and BodyCol are where an anonymous class's `{` is, set only
	// for AnonymousClasses.
	BodyLine, BodyCol int
}

type InterfaceNode struct {
	Attributes []AttributeDef
	Name       string
	FullName   string
	Extends    []string
	Methods    []MethodNode
	Properties []PropertyNode
	Constants  []ConstantNode
	StartLine  int
	EndLine    int
	StartCol   int
	DocComment string
}

type TraitNode struct {
	Attributes []AttributeDef
	Constants  []ConstantNode
	Name       string
	FullName   string
	Properties []PropertyNode
	Methods    []MethodNode
	Traits     []string
	StartLine  int
	EndLine    int
	StartCol   int
	DocComment string
	// TraitUseDoc holds the docblocks above the trait's own trait `use`
	// statements.
	TraitUseDoc string
	TraitRules  TraitAdaptations
}

type EnumCaseNode struct {
	Name       string
	Value      string
	Attributes []AttributeDef
	DocComment string
	StartLine  int
	StartCol   int
}

type EnumNode struct {
	Name       string
	FullName   string
	BackedType string
	Cases      []EnumCaseNode
	Methods    []MethodNode
	Properties []PropertyNode
	Constants  []ConstantNode
	Traits     []string
	TraitRules TraitAdaptations
	Implements []string
	Attributes []AttributeDef
	StartLine  int
	EndLine    int
	StartCol   int
	DocComment string
}

type FunctionNode struct {
	Attributes []AttributeDef
	EndLine    int
	Name       string
	FullName   string
	Params     []ParamNode
	ReturnType TypeNode
	StartLine  int
	StartCol   int
	DocComment string
}

type FileNode struct {
	Grammar   token.Version
	Namespace string
	Uses      []UseNode
	Classes   []ClassNode
	// AnonymousClasses are the `new class { ... }` expressions in the file.
	// Their Name is AnonymousClassName and their FullName is unique to the file.
	AnonymousClasses []ClassNode
	Interfaces       []InterfaceNode
	Traits           []TraitNode
	Enums            []EnumNode
	Functions        []FunctionNode
	Constants        []ConstantNode
	Errors           []ParseError
}

type DocParam struct {
	Type        string
	ParsedType  *types.TypeExpr
	Name        string
	Description string
}

type DocReturn struct {
	Type        string
	ParsedType  *types.TypeExpr
	Description string
}

type DocThrow struct {
	Type        string
	Description string
}

type DocProperty struct {
	Type        string
	ParsedType  *types.TypeExpr
	Name        string
	Description string
	ReadOnly    bool
	WriteOnly   bool
}

type DocMethod struct {
	ReturnType  string
	ParsedType  *types.TypeExpr
	Name        string
	Params      string
	Description string
	IsStatic    bool
}

// DocTemplate represents a @template tag: @template T of SomeClass
type DocTemplate struct {
	Name        string // e.g., "T", "TModel"
	Bound       string // e.g., "SomeClass" (from "of SomeClass"), or "" if unbounded
	ParsedBound *types.TypeExpr
	Default     string // e.g., "string" (from "= string"), or "" without a default
}

type DocTypeAlias struct {
	Name       string
	Type       string
	ParsedType *types.TypeExpr
}

type DocImportedTypeAlias struct {
	Name  string
	From  string
	Alias string
}

type DocBlock struct {
	Summary       string
	Tags          map[string][]string
	Params        []DocParam
	Return        DocReturn
	Throws        []DocThrow
	Properties    []DocProperty
	Methods       []DocMethod
	Templates     []DocTemplate
	TypeAliases   []DocTypeAlias
	ImportedTypes []DocImportedTypeAlias
	Deprecated    bool
	DeprecatedMsg string

	// Generic and narrowing tags. Prefixed forms (@phpstan-, @psalm-) win
	// over the plain tag for the same parameter or class, as in PHPStan.
	Extends     []string   // @extends / @template-extends: "Base<A, B>"
	Implements  []string   // @implements / @template-implements
	Uses        []string   // @use / @template-use
	Mixins      []string   // @mixin
	Vars        []DocParam // @var Type [$name]
	ParamOuts   []DocParam // @param-out Type $name
	ClosureThis []DocParam // @param-closure-this Type $name
	Asserts     []DocAssert
	SelfOut     string // @phpstan-self-out / @phpstan-this-out
}

// AssertKind says when a @phpstan-assert tag holds.
type AssertKind int

const (
	AssertAlways  AssertKind = iota // @phpstan-assert: after the call returns
	AssertIfTrue                    // @phpstan-assert-if-true: when the call returned true
	AssertIfFalse                   // @phpstan-assert-if-false: when the call returned false
)

// DocAssert is a @phpstan-assert tag: `@phpstan-assert !Foo $value`.
type DocAssert struct {
	Kind AssertKind
	Type string
	// Target is the parameter it narrows ($value), or $this, or an access on
	// either ($this->prop, $value->method()).
	Target string
	// Negated is set for !Type: the value is not of that type.
	Negated bool
	// Equality is set for =Type: the value is exactly that type, no narrowing
	// by intersection.
	Equality bool
}

func ParseDocBlock(raw string) *DocBlock {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	trimmed = strings.TrimPrefix(trimmed, "/**")
	trimmed = strings.TrimSuffix(trimmed, "*/")
	lines := strings.Split(trimmed, "\n")
	doc := &DocBlock{Tags: make(map[string][]string)}
	var tagged []docTag

	for _, line := range lines {
		line = strings.TrimSpace(line)
		line = strings.TrimPrefix(line, "*")
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "@") {
			name, value := splitDocTag(line[1:])
			doc.Tags[name] = append(doc.Tags[name], value)
			tagged = append(tagged, docTag{name: name, value: value})
			continue
		}
		if doc.Summary == "" {
			doc.Summary = line
		}
	}
	applyDocTags(doc, tagged)

	if doc.Summary == "" && len(doc.Tags) == 0 {
		return nil
	}
	return doc
}

// parseDocParam parses "@param Type $name Description" into structured DocParam.
// Uses ExtractDocTypeString to correctly handle complex types like array{name: string, age: int}.
func parseDocParam(value string) DocParam {
	p := DocParam{}
	value = strings.TrimSpace(value)
	if value == "" {
		return p
	}
	// If value starts with $, there's no type
	if strings.HasPrefix(value, "$") {
		parts := strings.Fields(value)
		p.Name = parts[0]
		if len(parts) > 1 {
			p.Description = strings.Join(parts[1:], " ")
		}
		return p
	}
	// Extract type (handles nested braces/angles)
	p.Type, value = types.ExtractDocTypeString(value)
	p.ParsedType = parseDocTypeExprPtr(p.Type)
	parts := strings.Fields(value)
	idx := 0
	if idx < len(parts) && strings.HasPrefix(parts[idx], "$") {
		p.Name = parts[idx]
		idx++
	}
	if idx < len(parts) {
		p.Description = strings.Join(parts[idx:], " ")
	}
	return p
}

// parseDocReturn parses "@return Type Description" into structured DocReturn.
// Uses ExtractDocTypeString to correctly handle complex types like array{key: string}.
func parseDocReturn(value string) DocReturn {
	r := DocReturn{}
	r.Type, r.Description = types.ExtractDocTypeString(value)
	r.ParsedType = parseDocTypeExprPtr(r.Type)
	return r
}

// parseDocThrow parses "@throws Type Description" into structured DocThrow.
func parseDocThrow(value string) DocThrow {
	th := DocThrow{}
	parts := strings.SplitN(value, " ", 2)
	if len(parts) >= 1 {
		th.Type = parts[0]
	}
	if len(parts) >= 2 {
		th.Description = strings.TrimSpace(parts[1])
	}
	return th
}

// parseDocProperty parses "@property Type $name Description" into structured DocProperty.
func parseDocProperty(value string, readOnly, writeOnly bool) DocProperty {
	p := DocProperty{ReadOnly: readOnly, WriteOnly: writeOnly}
	value = strings.TrimSpace(value)
	if value == "" {
		return p
	}
	// If value starts with $, there's no type
	if strings.HasPrefix(value, "$") {
		parts := strings.Fields(value)
		p.Name = strings.TrimPrefix(parts[0], "$")
		if len(parts) > 1 {
			p.Description = strings.Join(parts[1:], " ")
		}
		return p
	}
	// Extract type (handles nested braces/angles)
	p.Type, value = types.ExtractDocTypeString(value)
	p.ParsedType = parseDocTypeExprPtr(p.Type)
	parts := strings.Fields(value)
	idx := 0
	if idx < len(parts) && strings.HasPrefix(parts[idx], "$") {
		p.Name = strings.TrimPrefix(parts[idx], "$")
		idx++
	}
	if idx < len(parts) {
		p.Description = strings.Join(parts[idx:], " ")
	}
	return p
}

// parseDocMethod parses "@method ReturnType name(params) Description" or
// "@method name(params) Description" into structured DocMethod.
func parseDocMethod(value string) DocMethod {
	m := DocMethod{}
	value = strings.TrimSpace(value)
	if value == "" {
		return m
	}

	// Check if there's a static keyword prefix
	if strings.HasPrefix(value, "static ") {
		m.IsStatic = true
		value = strings.TrimSpace(value[7:])
	}

	// Try to detect if first token is a return type or a method name.
	// A method name will be followed by '(' while a type won't.
	firstParen := strings.Index(value, "(")
	if firstParen < 0 {
		// No parens at all — malformed, just store as name
		m.Name = value
		return m
	}

	beforeParen := value[:firstParen]
	parts := strings.Fields(beforeParen)

	switch len(parts) {
	case 0:
		return m
	case 1:
		// Either "name(" or might be a type if there's no space before (
		m.Name = parts[0]
	default:
		// "ReturnType name(" — last part is name, everything before is return type
		m.Name = parts[len(parts)-1]
		m.ReturnType = strings.Join(parts[:len(parts)-1], " ")
		m.ParsedType = parseDocTypeExprPtr(m.ReturnType)
	}

	// Extract params between ( and matching )
	rest := value[firstParen:]
	depth := 0
	closeIdx := -1
	for i, ch := range rest {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth == 0 {
				closeIdx = i
				break
			}
		}
	}
	if closeIdx >= 0 {
		m.Params = rest[1:closeIdx]
		remaining := strings.TrimSpace(rest[closeIdx+1:])
		if remaining != "" {
			m.Description = remaining
		}
	}

	return m
}

func parseDocTypeAlias(value string) DocTypeAlias {
	value = strings.TrimSpace(value)
	if value == "" {
		return DocTypeAlias{}
	}

	parts := strings.Fields(value)
	if len(parts) < 2 {
		return DocTypeAlias{}
	}

	alias := DocTypeAlias{Name: parts[0]}
	alias.Type = strings.TrimSpace(value[len(parts[0]):])
	alias.ParsedType = parseDocTypeExprPtr(alias.Type)
	return alias
}

func parseDocTypeExprPtr(raw string) *types.TypeExpr {
	expr := types.ParseTypeExpr(raw)
	if expr.String() == "" {
		return nil
	}
	return &expr
}

func parseDocImportedTypeAlias(value string) DocImportedTypeAlias {
	parts := strings.Fields(strings.TrimSpace(value))
	if len(parts) < 3 || parts[1] != "from" {
		return DocImportedTypeAlias{}
	}

	alias := DocImportedTypeAlias{
		Name: parts[0],
		From: parts[2],
	}
	if len(parts) >= 5 && parts[3] == "as" {
		alias.Alias = parts[4]
	}
	return alias
}
