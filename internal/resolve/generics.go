package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// This file holds the generics engine: substituting template parameters,
// binding them from a call's arguments, following @extends/@implements/@use
// type arguments up the hierarchy, and falling back to a template's bound or
// default when nothing binds it. It follows PHPStan's reading of PHPDoc
// generics: https://phpstan.org/blog/generics-in-php-using-phpdocs

// pseudoTypes are the PHPDoc type keywords that are never class names.
var pseudoTypes = map[string]bool{
	"int": true, "integer": true, "string": true, "float": true, "double": true,
	"bool": true, "boolean": true, "array": true, "object": true, "callable": true,
	"iterable": true, "void": true, "never": true, "null": true, "false": true,
	"true": true, "mixed": true, "self": true, "static": true, "parent": true,
	"resource": true, "$this": true, "list": true, "scalar": true, "numeric": true,
	"number": true, "empty": true, "noreturn": true, "class-string": true,
}

// IsBuiltinTypeName reports whether name is a PHP or PHPDoc type keyword
// (int, list, non-empty-string, class-string…) rather than a class name.
func IsBuiltinTypeName(name string) bool {
	if pseudoTypes[name] {
		return true
	}
	lower := strings.ToLower(name)
	if pseudoTypes[lower] {
		return true
	}
	// non-empty-list, positive-int, array-key, key-of, value-of and the like.
	return strings.IndexByte(name, '-') > 0 && strings.IndexByte(name, '\\') < 0
}

// transform rebuilds rt bottom-up, calling fn on every type inside it after
// its own parts are rebuilt. fn returns the replacement for the node. Unions
// and intersections are rebuilt from their parts, so their mirrored best part
// stays in step. rt is never modified.
func (rt ResolvedType) transform(fn func(ResolvedType) ResolvedType) ResolvedType {
	if rt.Conditional != nil {
		c := *rt.Conditional
		c.Target = c.Target.transform(fn)
		c.IfTrue = c.IfTrue.transform(fn)
		c.IfFalse = c.IfFalse.transform(fn)
		out := MakeUnion([]ResolvedType{c.IfTrue, c.IfFalse}, rt.Nullable)
		out.Conditional = &c
		return out
	}
	if len(rt.Union) > 0 {
		parts := make([]ResolvedType, len(rt.Union))
		for i, p := range rt.Union {
			parts[i] = p.transform(fn)
		}
		return MakeUnion(parts, rt.Nullable)
	}
	if len(rt.Intersection) > 0 {
		parts := make([]ResolvedType, len(rt.Intersection))
		for i, p := range rt.Intersection {
			parts[i] = p.transform(fn)
		}
		return makeIntersection(parts, rt.Nullable)
	}
	out := rt
	if len(rt.Params) > 0 {
		out.Params = make([]ResolvedType, len(rt.Params))
		for i, p := range rt.Params {
			out.Params[i] = p.transform(fn)
		}
	}
	if rt.Callable != nil {
		sig := &CallableType{}
		if len(rt.Callable.Params) > 0 {
			sig.Params = make([]ResolvedType, len(rt.Callable.Params))
			for i, p := range rt.Callable.Params {
				sig.Params[i] = p.transform(fn)
			}
		}
		if rt.Callable.Return != nil {
			ret := rt.Callable.Return.transform(fn)
			sig.Return = &ret
		}
		out.Callable = sig
	}
	if len(rt.Fields) > 0 {
		out.Fields = make([]ShapeEntry, len(rt.Fields))
		for i, f := range rt.Fields {
			out.Fields[i] = ShapeEntry{Key: f.Key, Type: f.Type.transform(fn), Optional: f.Optional}
		}
		out.Shape = shapeString(out.FQN, rt.Shape, out.Fields)
	}
	return fn(out)
}

// shapeString renders shape fields back into array{…}, list{…} or object{…}.
func shapeString(fqn, previous string, fields []ShapeEntry) string {
	prefix := "array"
	if fqn == "object" {
		prefix = "object"
	} else if strings.HasPrefix(previous, "list{") {
		prefix = "list"
	}
	var sb strings.Builder
	sb.WriteString(prefix)
	sb.WriteByte('{')
	for i, f := range fields {
		if i > 0 {
			sb.WriteString(", ")
		}
		if f.Key != "" {
			sb.WriteString(f.Key)
			if f.Optional {
				sb.WriteByte('?')
			}
			sb.WriteString(": ")
		}
		sb.WriteString(f.Type.String())
	}
	sb.WriteByte('}')
	return sb.String()
}

// typeEnv is what the names in a PHPDoc type stand for where it's used.
type typeEnv struct {
	// subst maps template names to their types.
	subst map[string]ResolvedType
	// static is what static, self and $this stand for: the class a method was
	// called on, with its type arguments. Empty leaves them as written.
	static ResolvedType
	// owner is the class whose PHPDoc the type was written in. Names relative
	// to its namespace are resolved there.
	owner string
	// file is the file being edited, the last resort for resolving a name.
	file *parser.FileNode
	// fileFirst resolves names through file's imports first: the type was
	// written in the file being edited.
	fileFirst bool
}

// envWithFileFirst marks env's types as written in env.file.
func envWithFileFirst(env typeEnv) typeEnv {
	env.fileFirst = true
	return env
}

// applyEnv substitutes templates and static in rt and resolves its class names.
func (r *Resolver) applyEnv(rt ResolvedType, env typeEnv) ResolvedType {
	return rt.transform(func(t ResolvedType) ResolvedType {
		if t.FQN == "" {
			return t
		}
		if len(t.Params) == 0 && t.Callable == nil && len(t.Fields) == 0 {
			if sub, ok := env.subst[t.FQN]; ok {
				if t.Nullable && !sub.Nullable {
					sub = MakeUnion([]ResolvedType{sub}, true)
				}
				return sub
			}
		}
		switch t.FQN {
		case "static", "self", "$this":
			if env.static.IsEmpty() {
				return t
			}
			out := env.static
			out.Nullable = out.Nullable || t.Nullable
			if len(t.Params) > 0 {
				out.Params = t.Params
			}
			return out
		}
		if t.Qualified {
			t.Qualified = false
			return t
		}
		if env.fileFirst && env.file != nil && !IsBuiltinTypeName(t.FQN) && !strings.HasPrefix(t.FQN, "\\") {
			if res := r.ResolveClassName(t.FQN, env.file); res != t.FQN || r.Index != nil && r.Index.Lookup(res) != nil {
				t.FQN = res
				return t
			}
		}
		switch t.FQN {
		case "new":
			// new<T>: an object of the class T or class-string<T> names.
			if len(t.Params) == 1 {
				if target := ClassStringTarget(t.Params[0]); !IsBuiltinTypeName(target.FQN) && !target.IsEmpty() {
					return target
				}
			}
			return t
		case "key-of", "value-of", "offset-access":
			if out, ok := r.evalTypeOperator(t); ok {
				return out
			}
			return t
		}
		if i := strings.Index(t.FQN, "::"); i > 0 && len(t.Params) == 0 {
			// A class constant used as a type: Foo::TYPES.
			class := r.qualifyDocName(t.FQN[:i], env.owner, env.file)
			if out, ok := r.classConstantType(class, t.FQN[i+2:], env.static); ok {
				return out
			}
			return t
		}
		t.FQN = r.qualifyDocName(t.FQN, env.owner, env.file)
		return t
	})
}

// evalTypeOperator works out key-of<T>, value-of<T> and T[K] once T and K
// are known. ok is false while they aren't.
func (r *Resolver) evalTypeOperator(t ResolvedType) (ResolvedType, bool) {
	switch t.FQN {
	case "offset-access":
		if len(t.Params) != 2 {
			return ResolvedType{}, false
		}
		key := t.Params[1].Literal
		if key == "" {
			key = t.Params[1].FQN
		}
		out := r.OffsetType(t.Params[0], key)
		return out, !out.IsEmpty()
	case "key-of", "value-of":
		if len(t.Params) != 1 {
			return ResolvedType{}, false
		}
		target := t.Params[0]
		// value-of<BackedEnum> is the enum's backing type.
		if t.FQN == "value-of" && target.FQN != "" && !IsBuiltinTypeName(target.FQN) && r.Index != nil {
			if sym := r.Index.Lookup(target.FQN); sym != nil && sym.Kind == symbols.KindEnum && sym.BackedType != "" {
				return ResolvedType{FQN: sym.BackedType}, true
			}
		}
		if len(target.Fields) > 0 {
			parts := make([]ResolvedType, 0, len(target.Fields))
			for i, f := range target.Fields {
				if t.FQN == "value-of" {
					parts = append(parts, f.Type)
					continue
				}
				switch {
				case f.Key == "":
					parts = append(parts, ResolvedType{FQN: "int", Literal: itoa(i)})
				case isIntKey(f.Key):
					parts = append(parts, ResolvedType{FQN: "int", Literal: f.Key})
				default:
					parts = append(parts, ResolvedType{FQN: "string", Literal: "'" + f.Key + "'"})
				}
			}
			return MakeUnion(parts, false), true
		}
		if k, v, ok := r.IterableTypes(target); ok {
			if t.FQN == "key-of" {
				return k, true
			}
			return v, true
		}
	}
	return ResolvedType{}, false
}

// classConstantType is the type of a class constant's value, for types such
// as key-of<Foo::TYPES>.
func (r *Resolver) classConstantType(class, name string, static ResolvedType) (ResolvedType, bool) {
	switch strings.ToLower(class) {
	case "self", "static", "$this":
		class = static.FQN
	}
	if class == "" || r.Index == nil || strings.ContainsAny(name, "*") {
		return ResolvedType{}, false
	}
	sym := r.Index.Lookup(class + "::" + name)
	if sym == nil {
		for _, m := range r.Index.GetClassMembers(class) {
			if m.Name == name && m.Kind == symbols.KindConstant {
				sym = m
				break
			}
		}
	}
	if sym == nil || sym.Value == "" {
		return ResolvedType{}, false
	}
	v := strings.TrimSpace(sym.Value)
	if strings.HasPrefix(v, "[") || strings.HasPrefix(strings.ToLower(v), "array(") {
		rt := InferArrayLiteralType(v)
		return rt, !rt.IsEmpty()
	}
	if lit := literalExprType(v); !lit.IsEmpty() {
		return lit, true
	}
	return ResolvedType{}, false
}

// qualifyDocName resolves a class name written in the PHPDoc of owner. The
// declaring file's imports were qualified when it was indexed; what's left is
// relative to owner's namespace, a global class, or a template that nothing
// bound.
func (r *Resolver) qualifyDocName(name, owner string, file *parser.FileNode) string {
	if name == "" || IsBuiltinTypeName(name) {
		return name
	}
	if strings.HasPrefix(name, "\\") {
		return name[1:]
	}
	if r.Index == nil {
		return name
	}
	if owner != "" {
		if i := strings.LastIndexByte(owner, '\\'); i > 0 {
			if cand := owner[:i] + "\\" + name; r.Index.Lookup(cand) != nil {
				return cand
			}
		}
	}
	if r.Index.Lookup(name) != nil {
		return name
	}
	if file != nil {
		return r.ResolveClassName(name, file)
	}
	return name
}

// templatesOf returns a class's template parameters, from its PHPDoc or, for
// the classes it knows, the built-in registry.
func (r *Resolver) templatesOf(classFQN string) []symbols.TemplateParam {
	if r.Index != nil {
		if sym := r.Index.Lookup(classFQN); sym != nil && len(sym.Templates) > 0 {
			return sym.Templates
		}
	}
	if tmpl, ok := knownTemplates[classFQN]; ok {
		out := make([]symbols.TemplateParam, len(tmpl.Params))
		for i, name := range tmpl.Params {
			out[i] = symbols.TemplateParam{Name: name}
		}
		return out
	}
	return nil
}

// maxAncestorDepth bounds how far up a hierarchy type arguments are followed.
const maxAncestorDepth = 16

// AncestorTypeArgs returns the type arguments that classFQN, given args,
// passes to ancestor through @extends, @implements and @use, at any depth:
// for `@implements Collection<Dog>` on DogCollection, the args of Collection
// are [Dog]. ok is false when ancestor isn't above classFQN.
func (r *Resolver) AncestorTypeArgs(classFQN string, args []ResolvedType, ancestor string) ([]ResolvedType, bool) {
	if strings.EqualFold(classFQN, ancestor) {
		return args, true
	}
	return r.ancestorArgs(classFQN, args, ancestor, map[string]bool{classFQN: true}, 0)
}

func (r *Resolver) ancestorArgs(classFQN string, args []ResolvedType, ancestor string, visited map[string]bool, depth int) ([]ResolvedType, bool) {
	if r.Index == nil || depth > maxAncestorDepth {
		return nil, false
	}
	sym := r.Index.Lookup(classFQN)
	if sym == nil {
		return nil, false
	}
	parents := make([]string, 0, 1+len(sym.Implements))
	if sym.Extends != "" {
		parents = append(parents, sym.Extends)
	}
	parents = append(parents, sym.Implements...)
	parents = append(parents, r.Index.TraitsOf(classFQN)...)

	var own map[string]ResolvedType
	for _, parent := range parents {
		if visited[parent] {
			continue
		}
		visited[parent] = true
		var parentArgs []ResolvedType
		for _, st := range sym.Supertypes {
			if !strings.EqualFold(st.FQN, parent) {
				continue
			}
			if own == nil {
				own = r.zipTemplates(sym.Templates, args)
			}
			parentArgs = make([]ResolvedType, len(st.Args))
			env := typeEnv{subst: own, owner: classFQN}
			for i, raw := range st.Args {
				parentArgs[i] = r.applyEnv(r.parseType(raw), env)
			}
			break
		}
		if strings.EqualFold(parent, ancestor) {
			return parentArgs, true
		}
		if found, ok := r.ancestorArgs(parent, parentArgs, ancestor, visited, depth+1); ok {
			return found, true
		}
	}
	return nil, false
}

// zipTemplates maps template names to the arguments given for them, by
// position. Arguments missing or empty leave their template unbound.
func (r *Resolver) zipTemplates(tmpls []symbols.TemplateParam, args []ResolvedType) map[string]ResolvedType {
	if len(tmpls) == 0 {
		return nil
	}
	subst := make(map[string]ResolvedType, len(tmpls))
	for i, t := range tmpls {
		if i < len(args) && !args[i].IsEmpty() {
			subst[t.Name] = args[i]
		}
	}
	return subst
}

// classSubst binds the templates of declFQN, the class that declares a
// member, for a call on staticFQN with type arguments args. A member
// declared on a parent, interface or trait gets the arguments the hierarchy
// passes up to it.
func (r *Resolver) classSubst(declFQN, staticFQN string, args []ResolvedType) map[string]ResolvedType {
	tmpls := r.templatesOf(declFQN)
	if len(tmpls) == 0 {
		return nil
	}
	if staticFQN == "" || strings.EqualFold(staticFQN, declFQN) {
		return r.zipTemplates(tmpls, args)
	}
	if up, ok := r.AncestorTypeArgs(staticFQN, args, declFQN); ok && up != nil {
		return r.zipTemplates(tmpls, up)
	}
	// A subclass that doesn't say what it passes up (no @extends) is read
	// as passing its own arguments on in order, which is what such
	// subclasses almost always mean.
	return r.zipTemplates(tmpls, args)
}

// fillUnbound gives every template in tmpls that subst doesn't bind its
// default, then its bound, then mixed, as PHPStan does for a template nothing
// in the call or the receiver pins down.
func (r *Resolver) fillUnbound(subst map[string]ResolvedType, tmpls []symbols.TemplateParam, owner string, file *parser.FileNode) map[string]ResolvedType {
	for _, t := range tmpls {
		if _, ok := subst[t.Name]; ok {
			continue
		}
		if subst == nil {
			subst = make(map[string]ResolvedType, len(tmpls))
		}
		fallback := ResolvedType{FQN: "mixed"}
		switch {
		case t.Default != "":
			fallback = r.applyEnv(r.parseType(t.Default), typeEnv{subst: subst, owner: owner, file: file})
		case t.Bound != "":
			fallback = r.applyEnv(r.parseType(t.Bound), typeEnv{subst: subst, owner: owner, file: file})
		}
		subst[t.Name] = fallback
	}
	return subst
}

// bindTemplates matches the type a parameter is declared with (pattern)
// against the type of the argument passed for it (actual) and records what
// each template in names stands for. A template bound twice gets the union of
// both. It never fails: what doesn't match just binds nothing.
func (r *Resolver) bindTemplates(pattern, actual ResolvedType, names map[string]bool, out map[string]ResolvedType, depth int) {
	if depth > 12 || actual.IsEmpty() && !actual.Nullable {
		return
	}
	// A union pattern: concrete parts take what they match, and a single
	// template part takes the rest. `T|null` given ?User binds T to User.
	if len(pattern.Union) > 0 || (pattern.Nullable && names[pattern.FQN] && len(pattern.Params) == 0) {
		parts := pattern.Parts()
		var tmpl string
		for _, p := range parts {
			if names[p.FQN] && len(p.Params) == 0 {
				if tmpl != "" {
					tmpl = "" // two templates: can't tell which takes what
					break
				}
				tmpl = p.FQN
			}
		}
		for _, p := range parts {
			if p.FQN != tmpl {
				r.bindTemplates(p, actual, names, out, depth+1)
			}
		}
		if tmpl != "" {
			var rest []ResolvedType
			for _, a := range actual.Parts() {
				a = r.classStringFromLiteral(a)
				if !r.matchesConcretePart(a, parts, names) {
					rest = append(rest, a)
				}
			}
			bindOne(out, tmpl, MakeUnion(rest, actual.Nullable && !pattern.Nullable))
		}
		return
	}
	if names[pattern.FQN] && len(pattern.Params) == 0 {
		bindOne(out, pattern.FQN, actual)
		return
	}
	// class-string<T> given Foo::class, or 'App\Foo'.
	if pattern.FQN == "class-string" && len(pattern.Params) == 1 {
		for _, a := range actual.Parts() {
			a = r.classStringFromLiteral(a)
			if a.FQN == "class-string" && len(a.Params) == 1 {
				r.bindTemplates(pattern.Params[0], a.Params[0], names, out, depth+1)
			}
		}
		return
	}
	// Callables: parameter types and the return type, each way they're known.
	if pattern.Callable != nil {
		for _, a := range actual.Parts() {
			if a.Callable == nil {
				continue
			}
			if pattern.Callable.Return != nil && a.Callable.Return != nil {
				r.bindTemplates(*pattern.Callable.Return, *a.Callable.Return, names, out, depth+1)
			}
			for i, p := range pattern.Callable.Params {
				if i < len(a.Callable.Params) && !a.Callable.Params[i].IsEmpty() && a.Callable.Params[i].FQN != "mixed" {
					r.bindTemplates(p, a.Callable.Params[i], names, out, depth+1)
				}
			}
		}
		return
	}
	// Shapes, by key.
	if len(pattern.Fields) > 0 {
		for _, a := range actual.Parts() {
			for i, pf := range pattern.Fields {
				for j, af := range a.Fields {
					if af.Key == pf.Key && (pf.Key != "" || i == j) {
						r.bindTemplates(pf.Type, af.Type, names, out, depth+1)
					}
				}
			}
		}
		return
	}
	if len(pattern.Params) == 0 {
		return
	}
	// array<K, V>, list<V>, iterable<K, V> and the like take the key and value
	// types of whatever is iterated.
	if isIterablePattern(pattern.FQN) {
		pk, pv := patternKeyValue(pattern)
		for _, a := range actual.Parts() {
			k, v, ok := r.IterableTypes(a)
			if !ok {
				continue
			}
			if !pk.IsEmpty() {
				r.bindTemplates(pk, k, names, out, depth+1)
			}
			r.bindTemplates(pv, v, names, out, depth+1)
		}
		return
	}
	// Generic classes: the same class pairwise, or a subclass through the
	// type arguments it passes up.
	for _, a := range actual.Parts() {
		args, ok := r.AncestorTypeArgs(a.FQN, a.Params, pattern.FQN)
		if !ok {
			continue
		}
		for i, p := range pattern.Params {
			if i < len(args) {
				r.bindTemplates(p, args[i], names, out, depth+1)
			}
		}
	}
}

// matchesConcretePart reports whether a part of an argument's type is taken
// by one of a union pattern's concrete (non-template) parts.
func (r *Resolver) matchesConcretePart(a ResolvedType, parts []ResolvedType, names map[string]bool) bool {
	for _, p := range parts {
		if names[p.FQN] {
			continue
		}
		if p.FQN == a.FQN {
			return true
		}
		if !IsBuiltinTypeName(p.FQN) && !IsBuiltinTypeName(a.FQN) && r.Index != nil && r.Index.IsSubtypeOf(a.FQN, p.FQN) {
			return true
		}
	}
	return false
}

// classStringFromLiteral reads a string literal naming a known class, such as
// 'App\\User', as class-string<App\User>.
func (r *Resolver) classStringFromLiteral(a ResolvedType) ResolvedType {
	if a.FQN != "string" || len(a.Literal) < 3 || r.Index == nil {
		return a
	}
	name := a.Literal[1 : len(a.Literal)-1]
	name = strings.TrimPrefix(strings.ReplaceAll(name, `\\`, `\`), `\`)
	if !isValidPHPClassName(name) {
		return a
	}
	if sym := r.Index.Lookup(name); sym != nil {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
			return ResolvedType{FQN: "class-string", Params: []ResolvedType{{FQN: name}}}
		}
	}
	return a
}

func bindOne(out map[string]ResolvedType, name string, t ResolvedType) {
	if t.IsEmpty() && !t.Nullable {
		return
	}
	if prev, ok := out[name]; ok {
		out[name] = MakeUnion([]ResolvedType{prev, t}, false)
		return
	}
	out[name] = t
}

// isIterablePattern reports whether a generic type is read as keys and values.
func isIterablePattern(fqn string) bool {
	switch strings.TrimPrefix(fqn, "\\") {
	case "array", "list", "non-empty-array", "non-empty-list", "iterable",
		"Traversable", "Iterator", "IteratorAggregate", "Generator":
		return true
	}
	return false
}

// patternKeyValue returns the key and value types a generic iterable type
// names. With one argument, there is only a value.
func patternKeyValue(rt ResolvedType) (key, value ResolvedType) {
	switch len(rt.Params) {
	case 0:
		return ResolvedType{}, ResolvedType{}
	case 1:
		switch rt.FQN {
		case "list", "non-empty-list":
			return ResolvedType{FQN: "int"}, rt.Params[0]
		}
		return ResolvedType{}, rt.Params[0]
	}
	return rt.Params[0], rt.Params[1]
}

// IterableTypes returns the key and value types of iterating rt with
// foreach: arrays and lists, shapes, iterable, generators, and objects that
// implement Traversable through PHPDoc type arguments.
func (r *Resolver) IterableTypes(rt ResolvedType) (key, value ResolvedType, ok bool) {
	if len(rt.Union) > 0 {
		var keys, values []ResolvedType
		for _, p := range rt.Union {
			if k, v, ok := r.IterableTypes(p); ok {
				keys = append(keys, k)
				values = append(values, v)
			}
		}
		if len(values) == 0 {
			return ResolvedType{}, ResolvedType{}, false
		}
		return MakeUnion(keys, false), MakeUnion(values, false), true
	}
	switch rt.FQN {
	case "array", "list", "non-empty-array", "non-empty-list", "iterable":
		if len(rt.Fields) > 0 {
			values := make([]ResolvedType, 0, len(rt.Fields))
			keyIsInt := true
			for _, f := range rt.Fields {
				values = append(values, f.Type)
				if f.Key != "" && !isIntKey(f.Key) {
					keyIsInt = false
				}
			}
			k := ResolvedType{FQN: "string"}
			if keyIsInt {
				k = ResolvedType{FQN: "int"}
			}
			return k, MakeUnion(values, false), true
		}
		if len(rt.Params) == 0 {
			return ResolvedType{}, ResolvedType{}, false
		}
		k, v := patternKeyValue(rt)
		if k.IsEmpty() {
			k = ResolvedType{FQN: "int"}
			if rt.FQN == "array" || rt.FQN == "non-empty-array" || rt.FQN == "iterable" {
				k = ResolvedType{FQN: "array-key"}
			}
		}
		return k, v, true
	case "Generator", "Traversable", "Iterator", "IteratorAggregate":
		if len(rt.Params) == 0 {
			return ResolvedType{}, ResolvedType{}, false
		}
		k, v := patternKeyValue(rt)
		if k.IsEmpty() {
			k = ResolvedType{FQN: "mixed"}
		}
		return k, v, true
	}
	if rt.FQN == "" || IsBuiltinTypeName(rt.FQN) {
		return ResolvedType{}, ResolvedType{}, false
	}
	for _, iface := range []string{"Iterator", "IteratorAggregate", "Traversable"} {
		args, found := r.AncestorTypeArgs(rt.FQN, rt.Params, iface)
		if !found || len(args) == 0 {
			continue
		}
		k, v := patternKeyValue(ResolvedType{FQN: iface, Params: args})
		if k.IsEmpty() {
			k = ResolvedType{FQN: "mixed"}
		}
		return k, v, true
	}
	// Without type arguments on the interface, what getIterator() or
	// current() and key() return.
	if r.Index != nil && r.Index.IsSubtypeOf(rt.FQN, "Traversable") {
		exit, ok := r.enterResolution("iterable|" + rt.FQN)
		if !ok {
			return ResolvedType{}, ResolvedType{}, false
		}
		defer exit()
		if m := r.FindAccessedMember(rt.FQN, "getIterator", true); m != nil && m.ParentFQN != "IteratorAggregate" {
			if it := r.MemberTypeForCall(m, nil, rt.FQN, rt.Params, nil); !it.IsEmpty() && it.FQN != rt.FQN {
				if k, v, ok := r.IterableTypes(it); ok {
					return k, v, true
				}
			}
		}
		if m := r.FindAccessedMember(rt.FQN, "current", true); m != nil && m.ParentFQN != "Iterator" {
			if v := r.MemberTypeForCall(m, nil, rt.FQN, rt.Params, nil); !v.IsEmpty() && v.FQN != "mixed" {
				k := ResolvedType{FQN: "mixed"}
				if km := r.FindAccessedMember(rt.FQN, "key", true); km != nil && km.ParentFQN != "Iterator" {
					if kt := r.MemberTypeForCall(km, nil, rt.FQN, rt.Params, nil); !kt.IsEmpty() {
						k = kt
					}
				}
				return k, v, true
			}
		}
	}
	return ResolvedType{}, ResolvedType{}, false
}

func isIntKey(k string) bool {
	if k == "" {
		return false
	}
	for i := 0; i < len(k); i++ {
		if k[i] < '0' || k[i] > '9' {
			return false
		}
	}
	return true
}
