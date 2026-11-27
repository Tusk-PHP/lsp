package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (r *Resolver) fallbackTemplates(classFQN string) (symbols.TemplateFallback, bool) {
	if r.Index == nil {
		return symbols.TemplateFallback{}, false
	}
	if sym := r.Index.Lookup(classFQN); sym != nil && len(sym.Templates) > 0 {
		return symbols.TemplateFallback{}, false
	}
	return r.Index.TemplateFallbackOf(classFQN)
}

func (r *Resolver) fallbackTemplateReturn(classFQN, methodName string, typeParams []ResolvedType) ResolvedType {
	tmpl, ok := r.fallbackTemplates(classFQN)
	if !ok {
		return ResolvedType{}
	}
	retTemplate, ok := tmpl.Methods[methodName]
	if !ok {
		return ResolvedType{}
	}
	if retTemplate == "static" {
		return ResolvedType{FQN: classFQN, Params: typeParams}
	}
	subst := make(map[string]ResolvedType, len(tmpl.Params))
	for i, paramName := range tmpl.Params {
		if i < len(typeParams) {
			subst[paramName] = typeParams[i]
		}
	}
	return substituteTemplate(retTemplate, subst)
}

// substituteTemplate parses a return type template and replaces template
// parameter names with their concrete types.
func substituteTemplate(template string, subst map[string]ResolvedType) ResolvedType {
	template = strings.TrimSpace(template)

	nullable := false
	if strings.HasPrefix(template, "?") {
		nullable = true
		template = template[1:]
	}

	// Check if the entire template is a single param name (e.g., "TModel")
	if rt, ok := subst[template]; ok {
		result := rt
		result.Nullable = result.Nullable || nullable
		return result
	}

	// Parse as generic type and substitute params recursively
	parsed := ParseGenericType(template)
	parsed.Nullable = parsed.Nullable || nullable

	// Substitute the base FQN if it's a template param
	if rt, ok := subst[parsed.FQN]; ok {
		parsed.FQN = rt.FQN
	}

	// Substitute params
	for i, p := range parsed.Params {
		if rt, ok := subst[p.FQN]; ok {
			parsed.Params[i] = rt
		}
	}

	return parsed
}

// ResolveSymbolTemplateReturn resolves a method return type using the symbol's
// @template declarations and @return docblock. Falls back if the method's
// return type references a template parameter.
func (r *Resolver) ResolveSymbolTemplateReturn(sym *symbols.Symbol, methodName string, typeParams []ResolvedType) ResolvedType {
	if sym == nil || len(sym.Templates) == 0 || len(typeParams) == 0 {
		return ResolvedType{}
	}

	// Build substitution map from symbol's template params
	subst := make(map[string]ResolvedType)
	for i, tmpl := range sym.Templates {
		if i < len(typeParams) {
			subst[tmpl.Name] = typeParams[i]
		}
	}

	// Find the method and check if its return type references a template param
	member := r.FindMember(sym.FQN, methodName)
	if member == nil {
		return ResolvedType{}
	}

	retType := member.ReturnType
	if retType == "" && member.DocComment != "" {
		if doc := parser.ParseDocBlock(member.DocComment); doc != nil && doc.Return.Type != "" {
			retType = doc.Return.Type
		}
	}
	if retType == "" {
		return ResolvedType{}
	}
	retType = r.expandTypeAliases(retType, sym.FQN, nil)

	return substituteTemplate(retType, subst)
}
