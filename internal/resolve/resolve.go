// Package resolve provides shared PHP symbol resolution utilities used by
// completion, hover, and analyzer packages. Consolidates duplicated logic
// for class name resolution, enclosing scope detection, member lookup,
// and text helpers.
package resolve

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// maxResolveDepth is the maximum re-entrancy depth for ResolveVariableType and
// ResolveVariableTypeTyped. Each call increments the counter; if the counter
// exceeds this bound the call returns an empty result immediately to break the
// cycle. A bound of 32 is far above any real method-chain depth (typical chains
// are 2–8 hops) but safely below the goroutine stack limit.
const maxResolveDepth = 32

// Engine holds what resolution keeps between requests: the index, the hooks
// that wire it to other providers, and the caches. A provider builds one
// Engine and starts each request with Engine.Resolver. It is safe for
// concurrent use.
type Engine struct {
	Index *symbols.Index
	// ContainerCall, when set, types a call the project's container answers,
	// such as app('request') or $container->get(Foo::class); src is the
	// source call's offsets are in. ok is false when it isn't such a call; an
	// empty type with ok set is one that gives a value rather than an object.
	ContainerCall func(r *Resolver, src string, file *parser.FileNode, call ast.Expr) (rt ResolvedType, ok bool)

	// The last source's scopes, since one request asks about many variables.
	scopeMu     sync.Mutex
	scopeSource string
	cachedLines []string
	scopeDoc    *scope.Document
	scopeTree   *syntaxTree
	// closureMemos remembers what each closure of scopeDoc is passed to.
	closureMemos   map[*scope.Scope]closureMemo
	closureMemoDoc *scope.Document
	// varMemos remembers variable types per write while scopeDoc and the
	// index generation stay the same.
	varMemos   map[varMemoKey]ResolvedType
	varMemoDoc *scope.Document
	varMemoGen uint64

	// parsed caches docblocks and PHPDoc types by their text.
	parsed parseCache
}

// Resolver resolves types for one request. It carries that request's
// recursion guard (depth, the keys in progress and the count of cuts) without
// locks, so it belongs to the goroutine that runs the request and is never
// shared or kept after it.
type Resolver struct {
	*Engine
	depth  int
	active map[string]bool
	cuts   int
}

// NewEngine returns an Engine over index with no hooks set.
func NewEngine(index *symbols.Index) *Engine {
	return &Engine{Index: index}
}

// Resolver starts a request: a Resolver sharing e's index, hooks and caches,
// with a guard of its own.
func (e *Engine) Resolver() *Resolver {
	return &Resolver{Engine: e}
}

// NewResolver returns a Resolver for one request over a new Engine on index,
// for callers that resolve once and keep no caches.
func NewResolver(index *symbols.Index) *Resolver {
	return NewEngine(index).Resolver()
}

func (r *Resolver) enterResolution(key string) (func(), bool) {
	return r.enterGuard(key, true)
}

// enterGuard is enterResolution; a cut counts as one (cuts) only when
// counted is set, for a caller that expects the cycle and handles it.
func (r *Resolver) enterGuard(key string, counted bool) (func(), bool) {
	if r.depth >= maxResolveDepth || r.active[key] {
		if counted || !r.active[key] {
			r.cuts++
		}
		return nil, false
	}
	if r.active == nil {
		r.active = make(map[string]bool)
	}
	r.depth++
	r.active[key] = true
	return func() {
		delete(r.active, key)
		r.depth--
	}, true
}

// ResolveClassName resolves a short or partially-qualified class name to a FQN
// using use statements and the file's namespace.
func (r *Resolver) ResolveClassName(name string, file *parser.FileNode) string {
	if name == "" {
		return ""
	}
	if file == nil {
		return name
	}
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
	if strings.HasPrefix(name, "?") {
		name = name[1:]
	}
	if strings.ContainsAny(name, "|&(") {
		return typeNamePartRe.ReplaceAllStringFunc(name, func(part string) string {
			return r.ResolveClassName(part, file)
		})
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range file.Uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	if file.Namespace != "" {
		fqn := file.Namespace + "\\" + name
		if r.Index.Lookup(fqn) != nil {
			return fqn
		}
	}
	return name
}

// typeNamePartRe matches each name in a union, intersection or DNF type.
var typeNamePartRe = regexp.MustCompile(`[^|&()\s]+`)

// classLike is the part of a class, trait, interface or enum that positions are
// matched against.
type classLike struct {
	fqn       string
	startLine int
	methods   []parser.MethodNode
}

// classLikes returns the file's class-likes, latest declared first.
func classLikes(file *parser.FileNode) []classLike {
	fqn := func(fullName, name string) string {
		if fullName != "" {
			return fullName
		}
		return BuildFQN(file.Namespace, name)
	}
	var all []classLike
	for _, c := range file.Classes {
		all = append(all, classLike{fqn(c.FullName, c.Name), c.StartLine, c.Methods})
	}
	for _, c := range file.Traits {
		all = append(all, classLike{fqn(c.FullName, c.Name), c.StartLine, c.Methods})
	}
	for _, c := range file.Interfaces {
		all = append(all, classLike{fqn(c.FullName, c.Name), c.StartLine, c.Methods})
	}
	for _, c := range file.Enums {
		all = append(all, classLike{fqn(c.FullName, c.Name), c.StartLine, c.Methods})
	}
	sort.SliceStable(all, func(i, j int) bool { return all[i].startLine > all[j].startLine })
	return all
}

// FindEnclosingClass returns the FQN of the class, trait, interface or enum that
// contains the given position.
func FindEnclosingClass(file *parser.FileNode, pos textpos.Position) string {
	if file == nil {
		return ""
	}
	if anon := file.AnonymousClassAt(pos.Line, pos.Character); anon != nil {
		return anon.FullName
	}
	for _, c := range classLikes(file) {
		if pos.Line >= c.startLine {
			return c.fqn
		}
	}
	return ""
}

// FindEnclosingMethod returns the method node that contains the given position.
func FindEnclosingMethod(file *parser.FileNode, pos textpos.Position) *parser.MethodNode {
	if file == nil {
		return nil
	}
	if anon := file.AnonymousClassAt(pos.Line, pos.Character); anon != nil {
		var best *parser.MethodNode
		for mi := range anon.Methods {
			m := &anon.Methods[mi]
			if pos.Line >= m.StartLine && (best == nil || m.StartLine > best.StartLine) {
				best = m
			}
		}
		return best
	}
	for _, c := range classLikes(file) {
		if pos.Line < c.startLine {
			continue
		}
		var best *parser.MethodNode
		for mi := range c.methods {
			m := &c.methods[mi]
			if pos.Line >= m.StartLine {
				if best == nil || m.StartLine > best.StartLine {
					best = m
				}
			}
		}
		if best != nil {
			return best
		}
	}
	return nil
}

// FindMember looks up a member (method, property, constant) on a class,
// traversing the inheritance chain and traits via GetClassMembers.
func (r *Resolver) FindMember(classFQN, memberName string) *symbols.Symbol {
	if named := r.Index.ClassMembersNamed(classFQN, memberName); len(named) > 0 {
		return named[0]
	}
	return nil
}

type MemberAccess uint8

const (
	PropertyAccess MemberAccess = iota
	ConstantAccess
	CallAccess
)

func MemberAccessOf(called, static bool, name string) MemberAccess {
	switch {
	case called:
		return CallAccess
	case static && !strings.HasPrefix(name, "$"):
		return ConstantAccess
	}
	return PropertyAccess
}

func (a MemberAccess) names(m *symbols.Symbol) bool {
	switch a {
	case CallAccess:
		return m.Kind == symbols.KindMethod
	case ConstantAccess:
		return m.Kind == symbols.KindConstant || m.Kind == symbols.KindEnumCase
	}
	return m.Kind == symbols.KindProperty
}

func (a MemberAccess) reachesMagic() bool {
	return a != ConstantAccess
}

func (r *Resolver) FindAccessedMember(classFQN, memberName string, access MemberAccess) *symbols.Symbol {
	var fallback *symbols.Symbol
	for _, m := range r.Index.ClassMembersNamed(classFQN, memberName) {
		if access.names(m) {
			return m
		}
		if fallback == nil && access.reachesMagic() {
			fallback = m
		}
	}
	return fallback
}

// MemberTypeResolved returns the fully resolved type including generic params.
// callerFQN is the class the method was called on (for late static binding:
// Category::query() where query is on Model — callerFQN is Category).
// typeContext carries generic params from the caller (e.g., Builder<Category>).
// It's MemberTypeForCall without the call's arguments.
func (r *Resolver) MemberTypeResolved(member *symbols.Symbol, file *parser.FileNode, callerFQN string, typeContext []ResolvedType) ResolvedType {
	return r.MemberTypeForCall(member, file, callerFQN, typeContext, nil)
}

// pickBestUnionPart selects the most informative part from a union type string.
// Prefers parts with generic syntax (Builder<static>) over bare types (Category).
// Skips null/void/mixed. For "Builder<static>|Category", returns "Builder<static>".
func PickBestUnionPart(union string) string {
	// Split union respecting < > nesting
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(union); i++ {
		switch union[i] {
		case '<':
			depth++
		case '>':
			depth--
		case '|':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(union[start:i]))
				start = i + 1
			}
		}
	}
	parts = append(parts, strings.TrimSpace(union[start:]))

	// Prefer the first part with generics; otherwise first non-null part
	var bestGeneric, bestPlain string
	for _, p := range parts {
		p = strings.TrimPrefix(p, "\\")
		if p == "" || p == "null" || p == "void" || p == "mixed" {
			continue
		}
		if strings.Contains(p, "<") && bestGeneric == "" {
			bestGeneric = p
		}
		if bestPlain == "" {
			bestPlain = p
		}
	}
	if bestGeneric != "" {
		return bestGeneric
	}
	return bestPlain
}

// hasNullInUnion checks if a union type string contains "null" as one of its parts.
func HasNullInUnion(union string) bool {
	depth := 0
	start := 0
	for i := 0; i < len(union); i++ {
		switch union[i] {
		case '<':
			depth++
		case '>':
			depth--
		case '|':
			if depth == 0 {
				if strings.TrimSpace(union[start:i]) == "null" {
					return true
				}
				start = i + 1
			}
		}
	}
	return strings.TrimSpace(union[start:]) == "null"
}

func (r *Resolver) resolveDocType(raw string, file *parser.FileNode) ResolvedType {
	return r.applyEnv(ParseGenericType(raw), typeEnv{file: file, fileFirst: true})
}

// resolveDocTypeIn resolves a PHPDoc type written in the file being edited,
// inside class owner: names go through the file's imports, and self and
// static stand for owner.
func (r *Resolver) resolveDocTypeIn(raw, owner string, file *parser.FileNode) ResolvedType {
	env := typeEnv{owner: owner, file: file}
	if owner != "" {
		env.static = ResolvedType{FQN: owner}
	}
	return r.applyEnv(ParseGenericType(raw), envWithFileFirst(env))
}

func (r *Resolver) resolveResolvedType(rt ResolvedType, file *parser.FileNode) ResolvedType {
	if rt.IsEmpty() || (rt.Shape != "" && rt.FQN == "array") {
		return rt
	}

	switch rt.FQN {
	case "self", "static", "$this":
		return rt
	default:
		rt.FQN = r.ResolveClassName(rt.FQN, file)
	}

	for i := range rt.Params {
		rt.Params[i] = r.resolveResolvedType(rt.Params[i], file)
	}

	return rt
}

// paramDocType is a parameter's PHPDoc type: a `@var` in its own doc comment
// (PHP 8.6), else the function's `@param` for it.
func paramDocType(param parser.ParamNode, fnDoc *parser.DocBlock, varName string) string {
	if param.DocComment != "" {
		if doc := parser.ParseDocBlock(param.DocComment); doc != nil {
			for _, v := range doc.Vars {
				if v.Type != "" && (v.Name == "" || v.Name == varName) {
					return v.Type
				}
			}
		}
	}
	return resolveDocParam(fnDoc, varName)
}

func resolveDocParam(doc *parser.DocBlock, varName string) string {
	if doc == nil {
		return ""
	}
	for _, param := range doc.Params {
		if param.Name == varName {
			return param.Type
		}
	}
	return ""
}

func (r *Resolver) resolveMethodParamTypeTyped(method *parser.MethodNode, varName string, file *parser.FileNode) ResolvedType {
	for _, param := range method.Params {
		if param.Name != varName {
			continue
		}
		if docType := paramDocType(param, parser.ParseDocBlock(method.DocComment), varName); docType != "" {
			docType = r.expandTypeAliases(docType, FindEnclosingClass(file, textpos.Position{Line: method.StartLine}), file)
			if rt := r.resolveDocType(docType, file); !rt.IsEmpty() {
				return rt
			}
		}
		return ResolvedType{FQN: r.ResolveClassName(paramTypeName(param), file)}
	}
	return ResolvedType{}
}

func (r *Resolver) resolveFunctionParamTypeTyped(fn *parser.FunctionNode, varName string, file *parser.FileNode) ResolvedType {
	for _, param := range fn.Params {
		if param.Name != varName {
			continue
		}
		if docType := paramDocType(param, parser.ParseDocBlock(fn.DocComment), varName); docType != "" {
			if rt := r.resolveDocType(docType, file); !rt.IsEmpty() {
				return rt
			}
		}
		return ResolvedType{FQN: r.ResolveClassName(paramTypeName(param), file)}
	}
	return ResolvedType{}
}

func (r *Resolver) resolveMethodParamType(method *parser.MethodNode, varName string, file *parser.FileNode) string {
	if rt := r.resolveMethodParamTypeTyped(method, varName, file); !rt.IsEmpty() {
		return rt.BaseFQN()
	}
	return ""
}

func (r *Resolver) resolveFunctionParamType(fn *parser.FunctionNode, varName string, file *parser.FileNode) string {
	if rt := r.resolveFunctionParamTypeTyped(fn, varName, file); !rt.IsEmpty() {
		return rt.BaseFQN()
	}
	return ""
}

// inferConstructorGenerics binds a class's @template params from the constructor
// argument type. E.g., new Collection(array<int, Shape>) → Collection<int, Shape>.
func (r *Resolver) inferConstructorGenerics(classFQN string, argType ResolvedType) ResolvedType {
	sym := r.Index.Lookup(classFQN)
	if sym == nil || len(sym.Templates) == 0 {
		return ResolvedType{}
	}

	// The argument type is array<K, V> or array{shape} — extract K and V
	if argType.FQN != "array" {
		return ResolvedType{}
	}

	var params []ResolvedType
	if len(argType.Params) >= 2 {
		// array<K, V> → bind templates in order
		for i := range sym.Templates {
			if i < len(argType.Params) {
				params = append(params, argType.Params[i])
			}
		}
	} else if argType.Shape != "" {
		// array{shape} with no generic params — treat as array<string, mixed>
		// This case is for associative arrays passed directly
		params = append(params, ResolvedType{FQN: "string"})
		params = append(params, ResolvedType{FQN: "mixed"})
	}

	if len(params) == 0 {
		return ResolvedType{}
	}
	return ResolvedType{FQN: classFQN, Params: params}
}

// MemberType returns the resolved type of a member symbol (property type or method return type).
// Falls back to @return/@var docblock annotations when no type hint is present.
func (r *Resolver) MemberType(member *symbols.Symbol, file *parser.FileNode) string {
	var typeName string
	ownerFQN := ownerScopeFQN(member, textpos.Position{}, file)
	switch member.Kind {
	case symbols.KindProperty:
		typeName = member.Type
		if typeName == "" && member.DocComment != "" {
			if doc := parser.ParseDocBlock(member.DocComment); doc != nil {
				if vars, ok := doc.Tags["var"]; ok && len(vars) > 0 {
					typeName = strings.Fields(vars[0])[0]
				}
			}
		}
	case symbols.KindConstant:
		typeName = member.Type
	case symbols.KindEnumCase:
		typeName = member.ParentFQN
	case symbols.KindMethod:
		typeName = member.ReturnType
		if typeName == "" && member.DocComment != "" {
			if doc := parser.ParseDocBlock(member.DocComment); doc != nil && doc.Return.Type != "" {
				typeName = doc.Return.Type
			}
		}
	default:
		return ""
	}
	typeName = r.expandTypeAliases(typeName, ownerFQN, file)
	if typeName == "" || typeName == "void" || typeName == "mixed" {
		return ""
	}
	if fields := types.ParseArrayShape(typeName); len(fields) > 0 {
		return "array"
	}
	if typeName == "self" || typeName == "static" || typeName == "$this" {
		return member.ParentFQN
	}
	// Strip leading backslash for FQN types from docblocks
	typeName = strings.TrimPrefix(typeName, "\\")
	// Strip generic type parameters: Builder<static> → Builder
	if idx := strings.IndexByte(typeName, '<'); idx > 0 {
		typeName = typeName[:idx]
	}
	// Handle union types: take the first non-null type
	if strings.Contains(typeName, "|") {
		for _, part := range strings.Split(typeName, "|") {
			part = strings.TrimSpace(part)
			if part != "" && part != "null" && part != "void" && part != "mixed" {
				typeName = part
				break
			}
		}
	}
	return r.ResolveClassName(typeName, file)
}

// ResolveVariableTypeTyped infers the type of a variable preserving generic context.
// Used by the typed chain resolver so that $categories = Category::query()->get()
// results in $categories having type Collection<int, Category>.
func (r *Resolver) ResolveVariableTypeTyped(varName string, lines []string, pos textpos.Position, file *parser.FileNode) ResolvedType {
	if varName == "$this" || file == nil {
		return r.resolveVariableTypeTypedBase(varName, lines, pos, file, variableScope{})
	}
	vs := r.variableScopeAt(varName, lines, pos)
	if rt, ok := r.reachingType(varName, lines, pos, file, vs); ok {
		return rt
	}
	key, memo := r.varMemoKeyAt(vs, varName, lines, pos, file)
	// An answer a guard against cycles cut short isn't kept. Guards tripped
	// by other requests at the same time only cost a missed cache entry.
	cutsBefore := r.cuts
	if memo {
		if rt, ok := r.varMemoGet(key); ok {
			return rt
		}
	}
	rt := r.resolveVariableTypeTypedBase(varName, lines, pos, file, vs)
	// What asserting calls, @param-out and self-out since its last
	// assignment say about it.
	rt = r.narrowVariable(varName, rt, lines, pos, file)
	if memo && r.cuts == cutsBefore {
		r.varMemoPut(key, rt)
	}
	return rt
}

func (r *Resolver) resolveVariableTypeTypedBase(varName string, lines []string, pos textpos.Position, file *parser.FileNode, vs variableScope) ResolvedType {
	exit, ok := r.enterResolution("typed|" + varName + "|" + strconv.Itoa(pos.Line) + ":" + strconv.Itoa(pos.Character))
	if !ok {
		return ResolvedType{}
	}
	defer exit()

	if file == nil {
		return ResolvedType{}
	}

	// Special case: $this, unless a closure is bound to another object.
	if varName == "$this" {
		if bound := r.closureThisAt(lines, pos, file); !bound.IsEmpty() {
			return bound
		}
		return ResolvedType{FQN: FindEnclosingClass(file, pos)}
	}

	if vs.severalValues() {
		return r.reachingValuesType(varName, lines, pos, file, vs)
	}
	if vs.param != nil {
		return r.closureParamType(vs.param, lines, file)
	}
	if vs.capturedAt != nil {
		return r.ResolveVariableTypeTyped(varName, lines, *vs.capturedAt, file)
	}

	// Check parameter types
	if !vs.inClosure && !vs.reassigned {
		enclosingMethod := FindEnclosingMethod(file, pos)
		if enclosingMethod != nil {
			if rt := r.resolveMethodParamTypeTyped(enclosingMethod, varName, file); !rt.IsEmpty() {
				return rt
			}
		}
		for _, fn := range file.Functions {
			if pos.Line >= fn.StartLine && (fn.EndLine == 0 || pos.Line <= fn.EndLine) {
				if rt := r.resolveFunctionParamTypeTyped(&fn, varName, file); !rt.IsEmpty() {
					return rt
				}
			}
		}
	}

	bare := strings.TrimPrefix(varName, "$")
	varPrefix := "$" + bare

	// Check foreach loop value bindings: foreach ($iterable as $v) or foreach ($iterable as $k => $v)
	if rt := r.resolveForeachValueType(varPrefix, lines, pos, file, vs.firstLine); !rt.IsEmpty() {
		return rt
	}

	tree := r.syntaxTree(lines, file)
	for i := pos.Line; i >= vs.firstLine && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		value, ok := vs.assignedValue(tree, i, varPrefix)
		if !ok {
			continue
		}
		if rt, ok := r.assignedType(varName, tree, tree.reduce(value), lines, pos, file); ok {
			return rt
		}
		break
	}

	// Fall back to standard resolution
	fqn := r.resolveVariableTypePlain(varName, lines, pos, file, vs)
	return ResolvedType{FQN: fqn}
}

// arrayElementType extracts the value element type from an array ResolvedType.
// For array<K, V> returns V; for array<V> (single param) returns V.
func arrayElementType(rt ResolvedType) ResolvedType {
	if rt.FQN != "array" || len(rt.Params) == 0 {
		return ResolvedType{}
	}
	// array<K, V> — value is last param
	return rt.Params[len(rt.Params)-1]
}

// ResolveVariableType infers the type of a variable from context:
// method/function parameters, class properties, $var = new ClassName(...),
// @var annotations, and literal type inference.
func (r *Resolver) ResolveVariableType(varName string, lines []string, pos textpos.Position, file *parser.FileNode) string {
	if varName == "$this" || file == nil {
		return r.resolveVariableTypePlain(varName, lines, pos, file, variableScope{})
	}
	vs := r.variableScopeAt(varName, lines, pos)
	if rt, ok := r.reachingType(varName, lines, pos, file, vs); ok {
		return PlainTypeName(rt)
	}
	return r.resolveVariableTypePlain(varName, lines, pos, file, vs)
}

func (r *Resolver) resolveVariableTypePlain(varName string, lines []string, pos textpos.Position, file *parser.FileNode, vs variableScope) string {
	exit, ok := r.enterResolution("plain|" + varName + "|" + strconv.Itoa(pos.Line) + ":" + strconv.Itoa(pos.Character))
	if !ok {
		return ""
	}
	defer exit()

	if file == nil {
		return ""
	}

	// Special case: $this
	if varName == "$this" {
		return FindEnclosingClass(file, pos)
	}

	if vs.severalValues() {
		return PlainTypeName(r.ResolveVariableTypeTyped(varName, lines, pos, file))
	}
	if vs.param != nil {
		return r.closureParamType(vs.param, lines, file).String()
	}
	if vs.capturedAt != nil {
		return r.ResolveVariableType(varName, lines, *vs.capturedAt, file)
	}

	// 1. Check enclosing method/function parameter type hints
	if !vs.inClosure && !vs.reassigned {
		enclosingMethod := FindEnclosingMethod(file, pos)
		if enclosingMethod != nil {
			if typ := r.resolveMethodParamType(enclosingMethod, varName, file); typ != "" {
				return typ
			}
		}
		// Also check standalone function parameters
		for _, fn := range file.Functions {
			if pos.Line >= fn.StartLine && (fn.EndLine == 0 || pos.Line <= fn.EndLine) {
				if typ := r.resolveFunctionParamType(&fn, varName, file); typ != "" {
					return typ
				}
			}
		}
	}

	// 2. Check class properties, unless a local variable has this name
	if !vs.isLocal && !vs.inClosure {
		for _, cls := range file.Classes {
			for _, prop := range cls.Properties {
				if "$"+prop.Name == varName && prop.Type.Name != "" {
					return r.ResolveClassName(prop.Type.Name, file)
				}
			}
		}
	}

	bare := strings.TrimPrefix(varName, "$")
	varPrefix := "$" + bare

	// 3. Look for $var = new ClassName(...) and literal assignments
	tree := r.syntaxTree(lines, file)
	for i := pos.Line; i >= vs.firstLine && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		value, ok := vs.assignedValue(tree, i, varPrefix)
		if !ok {
			continue
		}
		typ, stop := r.assignedPlainType(tree, tree.reduce(value))
		if typ != "" {
			return typ
		}
		if stop {
			break
		}
	}

	// 4. An inline `@var Type $var` on a statement before the read.
	if rt, ok := r.varAssertion(varName, pos, file, tree, tree.offset(textpos.Position{Line: vs.firstLine}), tree.offset(pos)); ok {
		return rt.String()
	}
	return ""
}

// InferLiteralType returns the PHP type for a literal expression value.
var classConstantRe = regexp.MustCompile(`^\\?[A-Za-z_][A-Za-z0-9_\\]*::class$`)

// ClassKeywordFQN resolves self, static and parent (any letter case) to the
// class they stand for at pos. ok is false when name isn't one of them; fqn is
// empty when there is no enclosing class or, for parent, no parent class.
func (r *Resolver) ClassKeywordFQN(name string, pos textpos.Position, file *parser.FileNode) (fqn string, ok bool) {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "self", "static":
		return FindEnclosingClass(file, pos), true
	case "parent":
		if enclosing := FindEnclosingClass(file, pos); enclosing != "" && r.Index != nil {
			if chain := r.Index.GetInheritanceChain(enclosing); len(chain) > 0 {
				return chain[0], true
			}
		}
		return "", true
	}
	return "", false
}

func InferLiteralType(expr string) string {
	if expr == "" {
		return ""
	}
	if expr[0] == '\'' || expr[0] == '"' {
		return "string"
	}
	if t := castType(expr); t != "" {
		return t
	}
	lower := strings.ToLower(expr)
	if lower == "true" || lower == "false" {
		return "bool"
	}
	if lower == "null" {
		return "null"
	}
	if expr[0] == '[' || strings.HasPrefix(lower, "array(") {
		return "array"
	}
	// Foo::class, self::class, static::class are class-name strings.
	if strings.HasSuffix(lower, "::class") && classConstantRe.MatchString(expr) {
		return "string"
	}
	if expr[0] >= '0' && expr[0] <= '9' || expr[0] == '-' {
		if strings.ContainsAny(expr, ".eE") {
			return "float"
		}
		return "int"
	}
	return ""
}

// castType returns the type an expression like `(int) $x` is cast to.
func castType(expr string) string {
	if expr[0] != '(' {
		return ""
	}
	end := strings.IndexByte(expr, ')')
	if end < 0 {
		return ""
	}
	switch strings.ToLower(strings.TrimSpace(expr[1:end])) {
	case "int", "integer":
		return "int"
	case "float", "double":
		return "float"
	case "string":
		return "string"
	case "bool", "boolean":
		return "bool"
	case "array":
		return "array"
	case "object":
		return "object"
	}
	return ""
}

// BuildFQN combines a namespace and name into a fully qualified name.
func BuildFQN(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

// --- Text helpers ---

// IsWordChar returns true if the byte is a valid PHP identifier character
// (letters, digits, underscore, bytes 0x80-0xff, backslash).
func IsWordChar(ch byte) bool {
	return parser.IsIdentByte(ch) || ch == '\\'
}

// SplitLines splits source into lines. Use this once per request,
// then pass the result to LineAt/WordAt to avoid repeated splitting.
func SplitLines(source string) []string {
	return strings.Split(source, "\n")
}

// LineAt returns the line at the given 0-based index from pre-split lines.
func LineAt(lines []string, line int) string {
	if line >= 0 && line < len(lines) {
		return lines[line]
	}
	return ""
}

// GetLineAt returns the line at the given 0-based line number.
func GetLineAt(source string, line int) string {
	return LineAt(SplitLines(source), line)
}

// WordAt extracts the word at the given position from pre-split lines,
// including $ prefix for variables.
func WordAt(lines []string, pos textpos.Position) string {
	if pos.Line < 0 || pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character > len(line) {
		return ""
	}
	ch := pos.Character
	if ch < len(line) && line[ch] == '$' {
		start := ch
		end := ch + 1
		for end < len(line) && IsWordChar(line[end]) {
			end++
		}
		if end > start+1 {
			return line[start:end]
		}
		return ""
	}
	start := pos.Character
	for start > 0 && IsWordChar(line[start-1]) {
		start--
	}
	if start > 0 && line[start-1] == '$' {
		start--
	}
	end := pos.Character
	for end < len(line) && IsWordChar(line[end]) {
		end++
	}
	if start >= end {
		return ""
	}
	return line[start:end]
}

// GetWordAt extracts the word at the given position, including $ prefix for variables.
func GetWordAt(source string, pos textpos.Position) string {
	return WordAt(SplitLines(source), pos)
}

// ParamDocSummary returns the summary of the doc comment written on the
// parameter varName (PHP 8.6) of the method or function enclosing pos, or ""
// when it has none.
func ParamDocSummary(file *parser.FileNode, pos textpos.Position, varName string) string {
	if file == nil {
		return ""
	}
	var params []parser.ParamNode
	if m := FindEnclosingMethod(file, pos); m != nil {
		params = m.Params
	}
	for _, fn := range file.Functions {
		if pos.Line >= fn.StartLine && (fn.EndLine == 0 || pos.Line <= fn.EndLine) {
			params = append(params, fn.Params...)
		}
	}
	for _, p := range params {
		if p.Name != varName || p.DocComment == "" {
			continue
		}
		if doc := parser.ParseDocBlock(p.DocComment); doc != nil && doc.Summary != "" {
			return doc.Summary
		}
	}
	return ""
}

// paramTypeName returns a parameter's declared type, or else the class its
// PHP 8.1 `= new Foo()` default creates.
func paramTypeName(param parser.ParamNode) string {
	if param.Type.Name != "" {
		return param.Type.Name
	}
	rest, ok := cutKeyword(param.DefaultValue, "new")
	if !ok {
		return ""
	}
	if open := strings.IndexByte(rest, '('); open >= 0 {
		rest = rest[:open]
	}
	return strings.TrimSpace(rest)
}
