package completion

import (
	"fmt"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

type Provider struct {
	index               *symbols.Index
	container           *container.ContainerAnalyzer
	resolver            *resolve.Resolver
	framework           string
	arrayResolver       *models.FrameworkArrayResolver
	routes              *frameworklaravel.RouteIndex
	views               *frameworklaravel.Views
	translations        *frameworklaravel.TranslationResolver
	symfonyTranslations *frameworksymfony.TranslationResolver
}

func NewProvider(index *symbols.Index, ca *container.ContainerAnalyzer, framework string) *Provider {
	p := &Provider{index: index, container: ca, resolver: resolve.NewResolver(index), framework: framework}
	// Wire up chain resolution so ResolveVariableType can handle $var = Class::method()->chain()
	p.resolver.ChainResolver = p.ResolveExpressionType
	// Wire up typed chain resolution for generic-aware variable type inference
	p.resolver.TypedChainResolver = p.ResolveExpressionTypeTyped
	return p
}

// ResolveExpressionType resolves the type of an expression like "Category::first()"
// or "$foo->bar()->baz()". Used as the ChainResolver callback and exposed for
// diagnostics type resolution.
func (p *Provider) ResolveExpressionType(expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return ""
	}
	// Append a dummy "->" so resolveAccessChain can walk the chain up to the last member
	// and return the type of the final call/access.
	dummyLine := expr + "->__dummy__"
	// wordStart points to "__dummy__" which is after "->"
	wordStart := len(expr) + 2
	return p.resolveAccessChain(dummyLine, wordStart, source, pos, file)
}

// ResolveDiagnosticType returns the type diagnostics check an expression
// against: the generic-aware type, written out in full ("?Foo",
// "Foo|Bar", "Collection<int, User>"), the same one hover and completion
// see. It falls back to the plain chain type when that can't be worked out.
func (p *Provider) ResolveDiagnosticType(expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	if rt := p.ResolveExpressionTypeTyped(expr, source, pos, file); !rt.IsEmpty() {
		return rt.FullString()
	}
	return p.ResolveExpressionType(expr, source, pos, file)
}

// ResolveExpressionTypeTyped resolves the type of an expression preserving
// generic parameters (e.g., returns Collection<int, Category> instead of bare Collection).
func (p *Provider) ResolveExpressionTypeTyped(expr string, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return resolve.ResolvedType{}
	}
	dummyLine := expr + "->__dummy__"
	wordStart := len(expr) + 2
	return p.resolveAccessChainTyped(dummyLine, wordStart, source, pos, file)
}

// SetArrayResolver sets the framework array resolver for config/request/model key completion.
func (p *Provider) SetArrayResolver(resolver *models.FrameworkArrayResolver) {
	p.arrayResolver = resolver
}

func (p *Provider) SetViewResolver(resolver *frameworklaravel.Views) {
	p.views = resolver
}

func (p *Provider) SetLaravelRouteIndex(routeIndex *frameworklaravel.RouteIndex) {
	p.routes = routeIndex
}

func (p *Provider) SetTranslationResolver(resolver *frameworklaravel.TranslationResolver) {
	p.translations = resolver
}

func (p *Provider) SetSymfonyTranslationResolver(resolver *frameworksymfony.TranslationResolver) {
	p.symfonyTranslations = resolver
}

// sanitizeCompletions is the authoritative post-processing step applied to every
// GetCompletions result. It performs two operations:
//
//  1. (M1) Filters out any item whose Label is empty — these are invalid LSP
//     items that some editors surface as blank entries.
//
//  2. (M2) Sorts the slice deterministically so that two identical calls on the
//     same input always produce byte-identical results. The sort key hierarchy
//     preserves the intended priority bucketing already encoded in SortText
//     (e.g., "0"–"5" prefixes) and then breaks ties alphabetically:
//     primary: SortText, secondary: Label, tertiary: Kind (numeric), quaternary: Detail.
func sanitizeCompletions(items []protocol.CompletionItem) []protocol.CompletionItem {
	// M1 — filter empty labels
	filtered := items[:0]
	for _, item := range items {
		if item.Label != "" {
			filtered = append(filtered, item)
		}
	}
	// M2 — deterministic sort
	sort.SliceStable(filtered, func(i, j int) bool {
		a, b := filtered[i], filtered[j]
		if a.SortText != b.SortText {
			return a.SortText < b.SortText
		}
		if a.Label != b.Label {
			return a.Label < b.Label
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Detail < b.Detail
	})
	return filtered
}

func (p *Provider) GetCompletions(uri, source string, pos protocol.Position) []protocol.CompletionItem {
	lines := p.resolver.Lines(source)
	line := resolve.LineAt(lines, pos.Line)
	prefix := ""
	if pos.Character <= len(line) {
		prefix = line[:pos.Character]
	}

	// Join multi-line chains: if the current line is a continuation (starts with ->),
	// prepend previous lines so the chain resolver can see the full expression.
	joinedLine, joinedChar := resolve.JoinChainLines(lines, pos.Line, pos.Character)
	if joinedChar > pos.Character && joinedChar <= len(joinedLine) {
		prefix = joinedLine[:joinedChar]
	}

	trimmed := strings.TrimSpace(prefix)

	// Check if there's already a '(' after the cursor (skip remaining identifier chars)
	parenAfterCursor := false
	if pos.Character < len(line) {
		rest := line[pos.Character:]
		for i := 0; i < len(rest); i++ {
			if resolve.IsWordChar(rest[i]) || rest[i] == '$' {
				continue
			}
			parenAfterCursor = rest[i] == '('
			break
		}
	}
	if p.framework == "laravel" && p.views != nil {
		if viewCtx, ok := frameworklaravel.ExtractViewCompletionContext(trimmed); ok {
			return sanitizeCompletions(p.completeLaravelViews(viewCtx))
		}
	}

	ctx := sourcectx.Analyze(uri, source, pos)
	if ctx == nil {
		return nil
	}
	file := ctx.File
	rk := newNameRanker(ctx.Namespace, ctx.Uses)

	if strings.HasSuffix(trimmed, "->") || strings.HasSuffix(trimmed, "?->") {
		return sanitizeCompletions(p.completeMemberAccess(uri, source, pos, prefix, file, parenAfterCursor))
	}
	// Container argument context takes priority over :: detection
	// (e.g. app(Request::class) should not trigger static access)
	if _, _, isContainer := extractContainerArgContext(trimmed); !isContainer {
		if strings.HasSuffix(trimmed, "::") {
			return sanitizeCompletions(p.completeStaticAccess(source, prefix, pos, file, parenAfterCursor))
		}
		// Typing after -> or :: (e.g. "$foo->ba" or "Foo::cr")
		if memberCtx, ok := sourcectx.DetectMemberContext(trimmed); ok {
			if memberCtx.AccessKind == sourcectx.AccessStatic {
				items := p.completeStaticAccess(source, memberCtx.Prefix, pos, file, parenAfterCursor)
				return sanitizeCompletions(filterByPrefix(items, memberCtx.Filter))
			}
			items := p.completeMemberAccess(uri, source, pos, memberCtx.Prefix, file, parenAfterCursor)
			return sanitizeCompletions(filterByPrefix(items, memberCtx.Filter))
		}
	}
	if strings.HasSuffix(trimmed, "|>") {
		return sanitizeCompletions(p.completePipe(rk))
	}
	if strings.Contains(trimmed, "#[") && !strings.Contains(trimmed, "]") {
		return sanitizeCompletions(p.completeAttribute(attributeNamePrefix(trimmed), source, rk, file))
	}
	words := strings.Fields(trimmed)
	if len(words) >= 1 && (words[len(words)-1] == "new" || (len(words) >= 2 && words[len(words)-2] == "new")) {
		return sanitizeCompletions(p.completeNew(prefix, source, rk, file))
	}
	if len(words) >= 1 && words[0] == "use" {
		return sanitizeCompletions(p.completeUse(prefix, rk))
	}
	// Builder column/relation argument completion: ->where('col, ->with('rel
	if method, partial, quote, ok := extractBuilderArgContext(trimmed); ok {
		if items := p.completeBuilderArg(method, partial, quote, prefix, source, pos, file); items != nil {
			return sanitizeCompletions(items)
		}
	}
	// Array key completion: $var['partial or $var['key1']['partial (nested)
	if ctx := parseArrayKeyContext(prefix); ctx != nil {
		return sanitizeCompletions(p.completeArrayKeys(source, pos, ctx, file))
	}
	// Config result array access: config('database')['connections']['
	if ctx := parseConfigResultArrayContext(prefix); ctx != nil {
		return sanitizeCompletions(p.completeConfigResultKeys(ctx))
	}
	// Config key completion: config('database.|') with dot-notation navigation
	if configPath, partial, quote, ok := extractConfigArgContext(trimmed); ok {
		return sanitizeCompletions(p.completeConfigKeys(configPath, partial, quote))
	}
	if p.framework == "laravel" && p.routes != nil {
		if partial, quote, ok := frameworklaravel.ExtractRouteNameArgContext(trimmed); ok {
			return sanitizeCompletions(p.completeLaravelRouteNames(partial, quote))
		}
	}
	if p.framework == "laravel" && p.translations != nil {
		if txCtx := frameworklaravel.DetectTranslationContext(line, pos.Character); txCtx != nil {
			return sanitizeCompletions(p.translations.Complete(txCtx))
		}
	}
	if p.framework == "symfony" {
		if partial, quote, ok := frameworksymfony.ExtractRouteNameContext(trimmed); ok {
			return sanitizeCompletions(p.completeSymfonyRouteNames(partial, quote))
		}
		if p.symfonyTranslations != nil {
			if txCtx := frameworksymfony.DetectSymfonyTranslationContext(line, pos.Character); txCtx != nil {
				return sanitizeCompletions(p.symfonyTranslations.Complete(txCtx))
			}
		}
	}
	if filter, quoteCtx, ok := extractContainerArgContext(trimmed); ok {
		return sanitizeCompletions(p.completeContainerResolve(source, filter, quoteCtx, file, rk))
	}
	// Variables in scope: `$` or `$na`
	if typed, ok := typedVariablePrefix(prefix); ok {
		return sanitizeCompletions(p.completeVariables(typed, lines, pos, file))
	}
	// Detect namespace path typing (contains \)
	search := extractLastWord(prefix)
	if strings.Contains(search, "\\") {
		return sanitizeCompletions(p.completeNamespacePath(search, rk))
	}
	items := p.completeGlobal(prefix, source, rk, file)
	// Add $this if inside a class method
	lastWord := ""
	if w := strings.Fields(strings.TrimSpace(prefix)); len(w) > 0 {
		lastWord = w[len(w)-1]
	}
	if lastWord == "" || strings.HasPrefix("$this", strings.ToLower(lastWord)) {
		if file != nil {
			if resolve.FindEnclosingClass(file, pos) != "" {
				items = append(items, protocol.CompletionItem{
					Label:    "$this",
					Kind:     protocol.CompletionItemKindVariable,
					Detail:   "Current object instance",
					SortText: rankKey(rankMember, 0, "$this"),
				})
			}
		}
	}
	return sanitizeCompletions(items)
}

func (p *Provider) completeLaravelViews(ctx *frameworklaravel.ViewContext) []protocol.CompletionItem {
	matches := p.views.Search(ctx.Partial)
	items := make([]protocol.CompletionItem, 0, len(matches))
	for _, view := range matches {
		insertText := view.Name
		if strings.HasPrefix(view.Name, ctx.Partial) {
			insertText = view.Name[len(ctx.Partial):]
		}
		items = append(items, protocol.CompletionItem{
			Label:      view.Name,
			Kind:       protocol.CompletionItemKindModule,
			Detail:     "Laravel view",
			InsertText: insertText,
			FilterText: view.Name,
			SortText:   "0" + view.Name,
		})
	}
	return items
}

func (p *Provider) completeSymfonyRouteNames(partial, quote string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	lowerPartial := strings.ToLower(partial)
	wrap := "'"
	if quote == `"` {
		wrap = `"`
	}

	for _, route := range frameworksymfony.DiscoverRoutes(p.index) {
		if lowerPartial != "" && !strings.HasPrefix(strings.ToLower(route.Name), lowerPartial) {
			continue
		}

		insertText := route.Name
		if quote == "" {
			insertText = wrap + route.Name + wrap
		}

		detail := route.Path
		if detail == "" {
			detail = route.URI
		}

		items = append(items, protocol.CompletionItem{
			Label:      route.Name,
			Kind:       protocol.CompletionItemKindValue,
			Detail:     detail,
			InsertText: insertText,
			SortText:   "0" + route.Name,
		})
	}

	return items
}

func (p *Provider) completeMemberAccess(uri, source string, pos protocol.Position, prefix string, file *parser.FileNode, parenAfterCursor bool) []protocol.CompletionItem {
	// Use typed resolver first to get generic context
	resolved := p.ResolveChainTypeTyped(source, prefix, "->", pos, file)
	if resolved.FQN == "object" && len(resolved.Fields) > 0 {
		return objectShapeCompletions(resolved)
	}
	typeName := resolved.MemberOwner()
	if typeName == "" {
		// Fall back to standard resolver
		typeName = p.resolveChainType(source, prefix, "->", pos, file)
	}
	if typeName == "" {
		return nil
	}
	if p.container != nil {
		if binding := p.container.ResolveDependency(typeName); binding != nil {
			typeName = binding.Concrete
		}
	}
	var items []protocol.CompletionItem
	enclosing := resolve.FindEnclosingClass(file, pos)
	members := p.index.GetClassMembers(typeName)
	// What @mixin tags bring in, after the class's own members.
	for _, mixin := range p.resolver.MixinTypes(resolved) {
		members = append(members, p.index.GetClassMembers(mixin.MemberOwner())...)
	}
	distance := memberDistances(members)
	seen := make(map[string]bool, len(members))
	for _, m := range members {
		// A class can reach its own private members, on any instance of itself.
		if m.IsStatic || (m.Visibility == "private" && (enclosing == "" || m.ParentFQN != enclosing)) {
			continue
		}
		// Members come closest first, so an override hides what it overrides.
		if seen[m.Name] {
			continue
		}
		seen[m.Name] = true
		item := protocol.CompletionItem{Label: m.Name, Detail: formatDetail(m), Documentation: formatDocumentation(m), SortText: rankKey(rankMember, distance[m.ParentFQN], strings.TrimPrefix(m.Name, "$"))}
		switch m.Kind {
		case symbols.KindMethod:
			item.Kind = protocol.CompletionItemKindMethod
			if parenAfterCursor {
				item.InsertText = m.Name
			} else {
				item.InsertText = m.Name + "($0)"
				item.InsertTextFormat = 2
			}
		case symbols.KindProperty:
			item.Label = strings.TrimPrefix(m.Name, "$")
			item.Kind = protocol.CompletionItemKindProperty
		}
		items = append(items, item)
	}
	return items
}

// objectShapeCompletions offers the properties of an object shape such as
// object{name: string, age?: int}.
func objectShapeCompletions(rt resolve.ResolvedType) []protocol.CompletionItem {
	items := make([]protocol.CompletionItem, 0, len(rt.Fields))
	for _, f := range rt.Fields {
		if f.Key == "" {
			continue
		}
		items = append(items, protocol.CompletionItem{
			Label:    f.Key,
			Kind:     protocol.CompletionItemKindProperty,
			Detail:   f.Type.FullString(),
			SortText: rankKey(rankMember, 0, f.Key),
		})
	}
	return items
}

func (p *Provider) completeStaticAccess(source, prefix string, pos protocol.Position, file *parser.FileNode, parenAfterCursor bool) []protocol.CompletionItem {
	typeName := p.resolveChainType(source, prefix, "::", pos, file)
	// $class:: where $class is a class-string<Foo>.
	if strings.HasPrefix(strings.TrimSpace(extractClassBefore(prefix, "::")), "$") || typeName == "" || typeName == "string" || strings.HasPrefix(typeName, "class-string") {
		if t := resolve.ClassStringTarget(p.ResolveChainTypeTyped(source, prefix, "::", pos, file)); t.FQN != "" && !resolve.IsBuiltinTypeName(t.FQN) {
			typeName = t.FQN
		}
	}
	if typeName == "" {
		return nil
	}
	// Resolve Laravel facades: show the concrete class's instance methods as
	// static completions alongside the facade's own members.
	var facadeConcrete string
	if p.container != nil && p.framework == "laravel" {
		facadeConcrete = p.container.ResolveFacade(typeName)
	}
	// Inside an instance method, self::, static:: and parent:: can also call
	// instance methods (parent::__construct(), parent::setData()), since they
	// keep $this. Private members of another class stay out of reach.
	instanceCalls := false
	enclosing := ""
	if op := strings.LastIndex(prefix, "::"); op >= 0 && forwardsCallingClass(prefix[:op+2]) {
		enclosing = resolve.FindEnclosingClass(file, pos)
		if m := resolve.FindEnclosingMethod(file, pos); m != nil && !m.IsStatic {
			instanceCalls = true
		}
	}
	var items []protocol.CompletionItem
	seen := map[string]bool{}
	memberSets := []string{typeName}
	if facadeConcrete != "" {
		memberSets = append(memberSets, facadeConcrete)
	}
	for set, fqn := range memberSets {
		members := p.index.GetClassMembers(fqn)
		distance := memberDistances(members)
		for _, m := range members {
			isFacadeForwarded := fqn == facadeConcrete && m.Kind == symbols.KindMethod
			isInstanceCall := instanceCalls && m.Kind == symbols.KindMethod
			if !isFacadeForwarded && !isInstanceCall && !m.IsStatic && m.Kind != symbols.KindConstant && m.Kind != symbols.KindEnumCase {
				continue
			}
			if enclosing != "" && m.Visibility == "private" && m.ParentFQN != enclosing {
				continue
			}
			if seen[m.Name] {
				continue
			}
			seen[m.Name] = true
			// A facade's forwarded methods come after the facade's own members.
			item := protocol.CompletionItem{Label: m.Name, Detail: formatDetail(m), Documentation: formatDocumentation(m), SortText: rankKey(rankMember, set*100+distance[m.ParentFQN], strings.TrimPrefix(m.Name, "$"))}
			switch m.Kind {
			case symbols.KindMethod:
				item.Kind = protocol.CompletionItemKindMethod
				if parenAfterCursor {
					item.InsertText = m.Name
				} else {
					item.InsertText = m.Name + "($0)"
					item.InsertTextFormat = 2
				}
			case symbols.KindConstant:
				item.Kind = protocol.CompletionItemKindConstant
			case symbols.KindEnumCase:
				item.Kind = protocol.CompletionItemKindEnumMember
			case symbols.KindProperty:
				if m.IsStatic {
					item.Kind = protocol.CompletionItemKindProperty
				} else {
					continue
				}
			}
			items = append(items, item)
		}
	}
	return items
}

// ResolveChainTypeTyped resolves the type of the expression before op (-> or ::)
// preserving generic context. Used by hover and completion for rich type display.
func (p *Provider) ResolveChainTypeTyped(source, prefix, op string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return resolve.ResolvedType{}
	}
	return p.resolveAccessChainTyped(prefix, idx+len(op), source, pos, file)
}

// resolveChainType resolves the class FQN from the expression before op (-> or ::).
// Handles: $var->, $this->, self::, static::, parent::, ClassName::,
// $var::, new ClassName()->, (new ClassName)->, method chains, and
// container calls: app('request')->, app(Request::class)->, resolve(...)->
func (p *Provider) resolveChainType(source, prefix, op string, pos protocol.Position, file *parser.FileNode) string {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return ""
	}
	// wordStart is after the operator (where the member name would be)
	return p.resolveAccessChain(prefix, idx+len(op), source, pos, file)
}

// resolveAccessChainTyped resolves the chain like resolveAccessChain but returns
// a ResolvedType that preserves generic context (e.g., Builder<Category>).
func (p *Provider) resolveAccessChainTyped(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType {
	return p.resolveChainWithContext(line, wordStart, source, pos, file, nil)
}

// resolveChainWithContext is the generic-aware chain resolver. It walks backward
// through the chain and tracks generic type parameters at each step.
func (p *Provider) resolveChainWithContext(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode, ctx []resolve.ResolvedType) resolve.ResolvedType {
	i := wordStart

	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return resolve.ResolvedType{}
	}

	op := ""
	if line[i-2] == '-' && line[i-1] == '>' {
		op = "->"
		i -= 2
		if i > 0 && line[i-1] == '?' {
			i-- // ?->
		}
	} else if line[i-2] == ':' && line[i-1] == ':' {
		op = "::"
		i -= 2
	} else {
		return resolve.ResolvedType{}
	}

	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	parenEnd := 0
	callArgStr := "" // captures the text inside (...) for method-level template inference
	if i > 0 && line[i-1] == ')' {
		parenEnd = i
		depth := 1
		i--
		for i > 0 && depth > 0 {
			i--
			if line[i] == ')' {
				depth++
			} else if line[i] == '(' {
				depth--
			}
		}
		// Capture argument content: text between ( and )
		if i >= 0 && i+1 < parenEnd-1 {
			callArgStr = strings.TrimSpace(line[i+1 : parenEnd-1])
		}

		if op == "->" && p.container != nil {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if concrete := p.resolveContainerCallType(callExpr, source, file); concrete != "" {
				if concrete == "-" {
					return resolve.ResolvedType{}
				}
				return resolve.ResolvedType{FQN: concrete}
			}
		}

		if op == "->" {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if newClass := extractNewClass(callExpr); newClass != "" {
				fqn := p.resolveClassNameFromSource(newClass, source, file)
				if args := extractNewArgs(callExpr); args != "" {
					ctx := resolve.ExprContext{Lines: p.resolver.Lines(source), Pos: pos, File: file}
					if rt := p.resolver.NewExpressionType(newClass, args, ctx); rt.FQN == fqn {
						return rt
					}
				}
				return resolve.ResolvedType{FQN: fqn}
			}
		}

		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	// An offset: $data['user']->, $rows[0]->
	if parenEnd == 0 && i > 0 && line[i-1] == ']' {
		closeAt := i - 1
		depth := 0
		j := closeAt
		for ; j >= 0; j-- {
			if line[j] == ']' {
				depth++
			} else if line[j] == '[' {
				depth--
				if depth == 0 {
					break
				}
			}
		}
		if j <= 0 {
			return resolve.ResolvedType{}
		}
		base := p.ResolveExpressionTypeTyped(strings.TrimSpace(line[:j]), source, pos, file)
		if base.IsEmpty() {
			return resolve.ResolvedType{}
		}
		return p.resolver.OffsetType(base, line[j+1:closeAt])
	}

	end := i
	for i > 0 && resolve.IsWordChar(line[i-1]) {
		i--
	}
	if i > 0 && line[i-1] == '$' {
		i--
	}
	if i >= end {
		return resolve.ResolvedType{}
	}
	target := line[i:end]

	// Resolve variables and keywords using standard resolution. self, static
	// and parent are keywords, which PHP matches regardless of case.
	switch keywordOrTarget(target) {
	case "$this":
		if file != nil {
			return p.resolver.ResolveVariableTypeTyped("$this", p.resolver.Lines(source), pos, file)
		}
		return resolve.ResolvedType{}
	case "self", "static":
		if file != nil {
			return resolve.ResolvedType{FQN: resolve.FindEnclosingClass(file, pos)}
		}
		return resolve.ResolvedType{}
	case "parent":
		if file != nil {
			classFQN := resolve.FindEnclosingClass(file, pos)
			if classFQN != "" {
				chain := p.index.GetInheritanceChain(classFQN)
				if len(chain) > 0 {
					return resolve.ResolvedType{FQN: chain[0]}
				}
			}
		}
		return resolve.ResolvedType{}
	}

	if strings.HasPrefix(target, "$") {
		rt := p.resolver.ResolveVariableTypeTyped(target, p.resolver.Lines(source), pos, file)
		if parenEnd > 0 && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "->") && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
			// $fn(…): what calling it gives.
			return p.resolver.InvokeType(rt)
		}
		return rt
	}

	// Try as class name — this is the chain origin (e.g., Category::). A word
	// after -> or :: is a member (UserRole::Client, $x->Client), never a class.
	if fqn := p.chainOriginClassName(line, i, target, source, file); fqn != "" {
		if sym := p.index.Lookup(fqn); sym != nil {
			switch sym.Kind {
			case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
				return resolve.ResolvedType{FQN: fqn}
			}
		} else if fqn != target && !strings.Contains(fqn, "::") {
			return resolve.ResolvedType{FQN: fqn}
		}
	}

	// Recursive chain resolution with context propagation
	if file == nil {
		return resolve.ResolvedType{}
	}
	// A function call starts the chain: identity($x)->…
	if parenEnd > 0 && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "->") && !strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
		ctx := resolve.ExprContext{Lines: p.resolver.Lines(source), Pos: pos, File: file}
		if rt := p.resolver.FunctionCallType(target, callArgStr, ctx); !rt.IsEmpty() {
			return rt
		}
	}
	ownerResolved := p.resolveChainWithContext(line, i, source, pos, file, nil)
	if ownerResolved.IsEmpty() {
		return resolve.ResolvedType{}
	}

	ownerFQN := ownerResolved.BaseFQN()
	if strings.HasSuffix(strings.TrimRight(line[:i], " \t"), "::") {
		ownerResolved = resolve.ClassStringTarget(ownerResolved) // $class::create()
	}
	member, foundOn := p.resolver.FindMemberOn(ownerResolved, target, parenEnd > 0)
	if member != nil && foundOn.FQN != ownerResolved.FQN {
		// A member a @mixin brings in. A fluent one gives back the object it
		// was called on, as forwarding through __call does.
		if p.resolver.ReturnsCallingClass(member) {
			return ownerResolved
		}
		ownerResolved = foundOn
	}
	ownerFQN = ownerResolved.BaseFQN()
	if member == nil {
		// A property of an object shape: object{user: User}.
		if ownerResolved.FQN == "object" && parenEnd == 0 {
			for _, f := range ownerResolved.Fields {
				if f.Key == target {
					return f.Type
				}
			}
		}
		return resolve.ResolvedType{}
	}
	ownerFQN = p.resolver.ReceiverFor(ownerResolved, member)

	// Pass the owner's generic params as context for template resolution.
	// E.g., Builder<Category> passes [Category] so Builder::get() → Collection<int, Category>.
	// callerFQN is the owner class for late static binding (Category::query() → static=Category).
	ownerContext := ownerResolved.Params

	// self::, parent:: and static:: pass the calling class along, so a method
	// returning static gives the class being edited: parent::create() in Child
	// is a Child.
	callerFQN := ownerFQN
	if forwardsCallingClass(line[:i]) {
		if enclosing := p.resolveAccessChain("static->__dummy__", len("static->"), source, pos, file); enclosing != "" {
			callerFQN = enclosing
		}
	}
	var call *resolve.CallSite
	if parenEnd > 0 {
		call = p.resolver.CallSiteFor(callArgStr, resolve.ExprContext{Lines: p.resolver.Lines(source), Pos: pos, File: file})
	}
	return p.resolver.MemberTypeForCall(member, file, callerFQN, ownerContext, call)
}

// extractNewArgs returns the constructor arguments of the `new Foo(…)`
// expression that s ends with, as extractNewClass reads it.
func extractNewArgs(s string) string {
	t := strings.TrimSpace(s)
	for strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") && parenBalanced(t[1:len(t)-1]) {
		t = strings.TrimSpace(t[1 : len(t)-1])
	}
	if !strings.HasSuffix(t, ")") {
		return ""
	}
	depth := 0
	for i := len(t) - 1; i >= 0; i-- {
		switch t[i] {
		case ')':
			depth++
		case '(':
			depth--
			if depth == 0 {
				return t[i+1 : len(t)-1]
			}
		}
	}
	return ""
}

// resolveAccessChain walks left through a chain of -> and :: accesses in the
// line text and returns the FQN of the class at that point. wordStart is the
// position immediately before the operator (-> or ::) to resolve.
// Handles recursive chains like Category::query()->with('form')->.
func (p *Provider) resolveAccessChain(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode) string {
	i := wordStart

	// Skip whitespace before the operator
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return ""
	}

	// Detect operator type
	op := ""
	if line[i-2] == '-' && line[i-1] == '>' {
		op = "->"
		i -= 2
		if i > 0 && line[i-1] == '?' {
			i-- // ?->
		}
	} else if line[i-2] == ':' && line[i-1] == ':' {
		op = "::"
		i -= 2
	} else {
		return ""
	}

	// Skip whitespace before operator
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	// Skip past a method call's closing paren: $foo->bar()->baz
	parenEnd := 0
	if i > 0 && line[i-1] == ')' {
		parenEnd = i
		depth := 1
		i--
		for i > 0 && depth > 0 {
			i--
			if line[i] == ')' {
				depth++
			} else if line[i] == '(' {
				depth--
			}
		}

		// Check for container call pattern (only for ->)
		if op == "->" && p.container != nil {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if concrete := p.resolveContainerCallType(callExpr, source, file); concrete != "" {
				if concrete == "-" {
					return ""
				}
				return concrete
			}
		}

		// Check for "new ClassName(...)" pattern
		if op == "->" {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if newClass := extractNewClass(callExpr); newClass != "" {
				return p.resolveClassNameFromSource(newClass, source, file)
			}
		}

		// Skip whitespace before paren
		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	// Extract the target word
	end := i
	for i > 0 && resolve.IsWordChar(line[i-1]) {
		i--
	}
	// Include $ for variables
	if i > 0 && line[i-1] == '$' {
		i--
	}
	if i >= end {
		return ""
	}
	target := line[i:end]

	// Resolve the target to a class FQN
	switch keywordOrTarget(target) {
	case "$this", "self", "static":
		if file != nil {
			return resolve.FindEnclosingClass(file, pos)
		}
		return ""
	case "parent":
		if file != nil {
			classFQN := resolve.FindEnclosingClass(file, pos)
			if classFQN != "" {
				chain := p.index.GetInheritanceChain(classFQN)
				if len(chain) > 0 {
					return chain[0]
				}
			}
		}
		return ""
	}

	// $variable-> or $variable::
	if strings.HasPrefix(target, "$") {
		return p.resolver.ResolveVariableType(target, p.resolver.Lines(source), pos, file)
	}

	// Try as a class name (for static access or direct use). A word after
	// -> or :: is a member (UserRole::Client, $x->Client), never a class.
	if fqn := p.chainOriginClassName(line, i, target, source, file); fqn != "" {
		if sym := p.index.Lookup(fqn); sym != nil {
			switch sym.Kind {
			case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
				return fqn
			}
			// sym exists but is a method/property — don't return a member FQN as a class
		} else if fqn != target && !strings.Contains(fqn, "::") {
			// Resolved to a different name (via use statement/namespace) but not in index yet — trust it
			// Skip if it contains "::" (it's a member FQN, not a class)
			return fqn
		}
	}

	// Not a class — must be a method/property in a chain.
	// Recursively resolve the owner, then look up the target as a member.
	if file == nil {
		return ""
	}
	ownerFQN := p.resolveAccessChain(line, i, source, pos, file)
	if ownerFQN == "" {
		return ""
	}
	member := p.resolver.FindAccessedMember(ownerFQN, target, parenEnd > 0)
	if member == nil {
		return ""
	}
	// A method returning static/$this/self gives back the class it was called
	// on, not the one that declares it: Child::create() is a Child. Calls through
	// self::, parent:: and static:: pass the calling class along, which here is
	// the class being edited.
	if p.resolver.ReturnsCallingClass(member) {
		if forwardsCallingClass(line[:i]) {
			if enclosing := p.resolveAccessChain("static->__dummy__", len("static->"), source, pos, file); enclosing != "" {
				return enclosing
			}
		}
		return ownerFQN
	}
	return p.resolver.MemberType(member, file)
}

// forwardsCallingClass reports whether the code before a member name ends in
// `self::`, `parent::` or `static::`, which keep the calling class for static.
func forwardsCallingClass(before string) bool {
	before = strings.TrimRight(before, " \t")
	if !strings.HasSuffix(before, "::") {
		return false
	}
	before = strings.TrimRight(before[:len(before)-2], " \t")
	start := len(before)
	for start > 0 && (isWordByte(before[start-1])) {
		start--
	}
	if start > 0 && before[start-1] == '$' {
		return false // $parent:: is a variable
	}
	switch strings.ToLower(before[start:]) {
	case "self", "parent", "static":
		return true
	}
	return false
}

// keywordOrTarget lower-cases target when it's one of the class keywords
// self, static or parent, so SELF:: and Parent:: resolve like their lower-case
// forms; any other name is returned as it is.
func keywordOrTarget(target string) string {
	switch lower := strings.ToLower(target); lower {
	case "self", "static", "parent":
		return lower
	}
	return target
}

func isWordByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// chainOriginClassName resolves target, which starts at line[start], as a
// class name, unless it follows a member operator (-> or ::) and so names a
// member such as an enum case, constant or property.
func (p *Provider) chainOriginClassName(line string, start int, target, source string, file *parser.FileNode) string {
	before := strings.TrimRight(line[:start], " \t")
	if strings.HasSuffix(before, "->") || strings.HasSuffix(before, "::") {
		return ""
	}
	return p.resolveClassNameFromSource(target, source, file)
}

// resolveClassNameFromSource resolves a short or FQN class name using
// the source file's use statements and namespace.
func (p *Provider) resolveClassNameFromSource(name, source string, file *parser.FileNode) string {
	if name == "" {
		return ""
	}
	// Already fully qualified
	if strings.HasPrefix(name, "\\") {
		fqn := strings.TrimPrefix(name, "\\")
		if p.index.Lookup(fqn) != nil {
			return fqn
		}
		return fqn
	}

	if file == nil {
		// Try direct lookup by name
		syms := p.index.LookupByName(name)
		if best := symbols.PickBestStandalone(syms, name); best != nil {
			return best.FQN
		}
		return name
	}

	// Check use statements
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range file.Uses {
		if u.Alias == parts[0] {
			if len(parts) > 1 {
				return u.FullName + "\\" + parts[1]
			}
			return u.FullName
		}
	}
	// Try in current namespace
	if file.Namespace != "" {
		fqn := file.Namespace + "\\" + name
		if p.index.Lookup(fqn) != nil {
			return fqn
		}
	}
	// Try as global
	if p.index.Lookup(name) != nil {
		return name
	}
	// Fallback: search by short name
	syms := p.index.LookupByName(name)
	if best := symbols.PickBestStandalone(syms, name); best != nil {
		return best.FQN
	}
	return name
}

// extractNewClass extracts the class name from patterns like:
// "new ClassName()", "(new ClassName())", "new ClassName", "(new ClassName)"
func extractNewClass(s string) string {
	t := strings.TrimSpace(s)

	// Strip wrapping parens: "(new ClassName())" → "new ClassName()"
	for strings.HasPrefix(t, "(") && strings.HasSuffix(t, ")") {
		inner := t[1 : len(t)-1]
		// Only strip if the parens are balanced wrapping parens (not constructor args)
		if parenBalanced(inner) {
			t = strings.TrimSpace(inner)
		} else {
			break
		}
	}

	// Strip constructor args: "new ClassName(...)" → "new ClassName"
	if strings.HasSuffix(t, ")") {
		depth := 1
		i := len(t) - 2
		for i >= 0 && depth > 0 {
			if t[i] == ')' {
				depth++
			} else if t[i] == '(' {
				depth--
			}
			i--
		}
		t = strings.TrimSpace(t[:i+1])
	}

	// Extract class name after "new"
	if idx := strings.LastIndex(t, "new "); idx >= 0 {
		className := strings.TrimSpace(t[idx+4:])
		if className != "" && !strings.ContainsAny(className, " \t(") {
			return className
		}
	}
	return ""
}

func parenBalanced(s string) bool {
	depth := 0
	for _, ch := range s {
		if ch == '(' {
			depth++
		} else if ch == ')' {
			depth--
			if depth < 0 {
				return false
			}
		}
	}
	return depth == 0
}

func (p *Provider) completeNew(prefix, source string, rk nameRanker, file *parser.FileNode) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	words := strings.Fields(prefix)
	search := ""
	if len(words) > 1 {
		search = words[len(words)-1]
		if search == "new" {
			search = ""
		}
	}
	for _, sym := range p.index.SearchByPrefix(search) {
		if sym.Kind != symbols.KindClass {
			continue
		}
		item := protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindClass, Detail: sym.FQN, InsertText: sym.Name + "($0)", InsertTextFormat: 2, SortText: rk.key(sym, sym.Name)}
		item.AdditionalTextEdits = buildAutoImportEdit(sym.FQN, source, file)
		items = append(items, item)
	}
	return items
}

func (p *Provider) completeUse(prefix string, rk nameRanker) []protocol.CompletionItem {
	parts := strings.Fields(prefix)
	search := ""
	if len(parts) > 1 {
		search = parts[len(parts)-1]
	}
	// If typing a namespace path, use namespace-aware completion
	if strings.Contains(search, "\\") {
		return p.completeNamespacePath(search, rk)
	}
	// Otherwise show all top-level namespace segments + global symbols
	var items []protocol.CompletionItem
	_, nsSegs := p.index.SearchByFQNPrefix("")
	for _, seg := range nsSegs {
		if search == "" || strings.HasPrefix(strings.ToLower(seg), strings.ToLower(search)) {
			items = append(items, protocol.CompletionItem{
				Label:    seg,
				Kind:     protocol.CompletionItemKindModule,
				Detail:   seg,
				SortText: "0" + seg,
			})
		}
	}
	if search != "" {
		for _, sym := range p.index.SearchByPrefix(search) {
			if sym.Kind == symbols.KindMethod || sym.Kind == symbols.KindProperty {
				continue
			}
			items = append(items, protocol.CompletionItem{Label: sym.FQN, Kind: symKind(sym.Kind), Detail: sym.Name, SortText: rk.key(sym, sym.FQN)})
		}
	}
	return items
}

func (p *Provider) completePipe(rk nameRanker) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	for _, sym := range p.index.SearchByPrefix("") {
		if sym.Kind == symbols.KindFunction && len(sym.Params) > 0 {
			items = append(items, protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindFunction, Detail: formatDetail(sym), SortText: rk.key(sym, sym.Name)})
		}
	}
	return items
}

// attributeNamePrefix extracts the partial attribute class name the user is
// typing after the last `#[` on the line, e.g. `#[Ch` -> "Ch" and
// `#[Unique(...), Ro` -> "Ro". Returns "" when no name has been started yet.
func attributeNamePrefix(trimmed string) string {
	idx := strings.LastIndex(trimmed, "#[")
	if idx < 0 {
		return ""
	}
	tail := trimmed[idx+2:]
	// A new attribute starts after the last top-level comma in the group.
	if comma := strings.LastIndex(tail, ","); comma >= 0 {
		tail = tail[comma+1:]
	}
	return strings.TrimSpace(tail)
}

func (p *Provider) completeAttribute(prefix, source string, rk nameRanker, file *parser.FileNode) []protocol.CompletionItem {
	attrs := [][2]string{
		{"Override", "PHP 8.3"}, {"Deprecated", "PHP 8.4"}, {"SensitiveParameter", "Sensitive in stack traces"}, {"AllowDynamicProperties", "Allow dynamic props"},
	}
	if p.framework == "symfony" {
		attrs = append(attrs, [2]string{"Route", "Define route"}, [2]string{"AsController", "Register controller"}, [2]string{"AsCommand", "Register command"}, [2]string{"Autowire", "Autowire service"}, [2]string{"AsEventListener", "Event listener"}, [2]string{"AsMessageHandler", "Message handler"})
	}
	seen := map[string]bool{}
	var items []protocol.CompletionItem
	for _, a := range attrs {
		seen[a[0]] = true
		items = append(items, protocol.CompletionItem{Label: a[0], Kind: protocol.CompletionItemKindClass, Detail: a[1]})
	}
	// Once the user starts typing a name, surface matching attribute classes
	// from the index (user-defined and vendor attributes like `Char`, `Unique`).
	if prefix != "" && p.index != nil {
		for _, sym := range p.index.SearchByPrefix(prefix) {
			if sym == nil || sym.Kind != symbols.KindClass || seen[sym.Name] {
				continue
			}
			seen[sym.Name] = true
			items = append(items, protocol.CompletionItem{
				Label:               sym.Name,
				Kind:                protocol.CompletionItemKindClass,
				Detail:              sym.FQN,
				SortText:            rk.key(sym, sym.Name),
				AdditionalTextEdits: buildAutoImportEdit(sym.FQN, source, file),
			})
		}
	}
	return items
}

func (p *Provider) completeNamespacePath(search string, rk nameRanker) []protocol.CompletionItem {
	var items []protocol.CompletionItem

	// Strip leading \ for absolute namespace paths
	fqnPrefix := strings.TrimPrefix(search, "\\")

	// Ensure trailing \ so we search within the namespace
	if !strings.HasSuffix(fqnPrefix, "\\") {
		// User is mid-segment: "Illuminate\Fo" → search prefix "Illuminate\" with filter "Fo"
		if idx := strings.LastIndex(fqnPrefix, "\\"); idx >= 0 {
			nsPrefix := fqnPrefix[:idx+1] // "Illuminate\"
			filter := strings.ToLower(fqnPrefix[idx+1:])
			syms, nsSegs := p.index.SearchByFQNPrefix(nsPrefix)

			// Add matching namespace segments
			for _, seg := range nsSegs {
				if filter == "" || strings.HasPrefix(strings.ToLower(seg), filter) {
					items = append(items, protocol.CompletionItem{
						Label:    seg,
						Kind:     protocol.CompletionItemKindModule,
						Detail:   nsPrefix + seg,
						SortText: "0" + seg,
					})
				}
			}
			// Add matching direct symbols
			for _, sym := range syms {
				if filter != "" && !strings.HasPrefix(strings.ToLower(sym.Name), filter) {
					continue
				}
				item := protocol.CompletionItem{
					Label:    sym.Name,
					Kind:     symKind(sym.Kind),
					Detail:   sym.FQN,
					SortText: rk.key(sym, sym.Name),
				}
				if sym.Kind == symbols.KindFunction {
					item.InsertText = sym.Name + "($0)"
					item.InsertTextFormat = 2
				}
				items = append(items, item)
			}
			return items
		}
	}

	// Exact namespace prefix with trailing \
	syms, nsSegs := p.index.SearchByFQNPrefix(fqnPrefix)

	// Add child namespace segments
	for _, seg := range nsSegs {
		items = append(items, protocol.CompletionItem{
			Label:    seg,
			Kind:     protocol.CompletionItemKindModule,
			Detail:   fqnPrefix + seg,
			SortText: "0" + seg,
		})
	}
	// Add direct symbols in this namespace
	for _, sym := range syms {
		item := protocol.CompletionItem{
			Label:    sym.Name,
			Kind:     symKind(sym.Kind),
			Detail:   sym.FQN,
			SortText: rk.key(sym, sym.Name),
		}
		if sym.Kind == symbols.KindFunction {
			item.InsertText = sym.Name + "($0)"
			item.InsertTextFormat = 2
		}
		items = append(items, item)
	}
	return items
}

func filterByPrefix(items []protocol.CompletionItem, prefix string) []protocol.CompletionItem {
	if prefix == "" {
		return items
	}
	lp := strings.ToLower(prefix)
	var filtered []protocol.CompletionItem
	for _, item := range items {
		if strings.HasPrefix(strings.ToLower(item.Label), lp) {
			filtered = append(filtered, item)
		}
	}
	return filtered
}

func extractLastWord(prefix string) string {
	trimmed := strings.TrimSpace(prefix)
	words := strings.Fields(trimmed)
	if len(words) == 0 {
		return ""
	}
	return words[len(words)-1]
}

func (p *Provider) completeGlobal(prefix, source string, rk nameRanker, file *parser.FileNode) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	words := strings.Fields(strings.TrimSpace(prefix))
	search := ""
	if len(words) > 0 {
		search = words[len(words)-1]
	}
	lsearch := strings.ToLower(search)
	// PHP primitive types — highest priority
	for _, t := range []string{"string", "int", "float", "bool", "array", "object", "callable", "iterable", "void", "never", "null", "mixed", "self", "static", "true", "false"} {
		if search == "" || strings.HasPrefix(t, lsearch) {
			items = append(items, protocol.CompletionItem{Label: t, Kind: protocol.CompletionItemKindTypeParameter, SortText: keywordKey(t)})
		}
	}
	for _, kw := range []string{"abstract", "class", "const", "enum", "extends", "final", "fn", "for", "foreach", "function", "if", "implements", "interface", "match", "namespace", "new", "private", "protected", "public", "readonly", "return", "switch", "throw", "trait", "try", "use", "while", "yield"} {
		if search == "" || strings.HasPrefix(kw, lsearch) {
			items = append(items, protocol.CompletionItem{Label: kw, Kind: protocol.CompletionItemKindKeyword, SortText: keywordKey(kw)})
		}
	}
	for _, sym := range p.index.SearchByPrefix(search) {
		// In standalone context only show callable/referenceable symbols:
		// functions, classes, interfaces, enums, traits
		switch sym.Kind {
		case symbols.KindFunction:
			// Skip magic methods and namespaced functions that are likely
			// class methods misidentified by the parser (real global helpers
			// like collect(), config() have no namespace in their FQN)
			if strings.HasPrefix(sym.Name, "__") {
				continue
			}
			if sym.ParentFQN != "" {
				continue
			}
			// Namespaced functions from vendor are almost always parser leaks
			// (class methods that escaped their class body). Skip them unless
			// they're from the project source.
			if strings.Contains(sym.FQN, "\\") && sym.Source == symbols.SourceVendor {
				continue
			}
			item := protocol.CompletionItem{Label: sym.Name, Kind: symKind(sym.Kind), Detail: sym.FQN, SortText: rk.key(sym, sym.Name)}
			item.InsertText = sym.Name + "($0)"
			item.InsertTextFormat = 2
			items = append(items, item)
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
			// Only show type-level symbols when user is actively typing a name
			if search != "" {
				item := protocol.CompletionItem{Label: sym.Name, Kind: symKind(sym.Kind), Detail: sym.FQN, SortText: rk.key(sym, sym.Name)}
				item.AdditionalTextEdits = buildAutoImportEdit(sym.FQN, source, file)
				items = append(items, item)
			}
		}
	}
	return items
}

func extractNamespace(source string) string {
	return sourcectx.Namespace(source)
}

func extractVariableBefore(prefix, op string) string {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return ""
	}
	before := strings.TrimSpace(prefix[:idx])
	for i := len(before) - 1; i >= 0; i-- {
		if before[i] == '$' {
			return before[i:]
		}
		if !(before[i] >= 'a' && before[i] <= 'z') && !(before[i] >= 'A' && before[i] <= 'Z') && !(before[i] >= '0' && before[i] <= '9') && before[i] != '_' {
			break
		}
	}
	return ""
}

func extractClassBefore(prefix, op string) string {
	idx := strings.LastIndex(prefix, op)
	if idx < 0 {
		return ""
	}
	before := strings.TrimSpace(prefix[:idx])
	for i := len(before) - 1; i >= 0; i-- {
		ch := before[i]
		if !(ch >= 'a' && ch <= 'z') && !(ch >= 'A' && ch <= 'Z') && !(ch >= '0' && ch <= '9') && ch != '_' && ch != '\\' {
			return before[i+1:]
		}
	}
	return before
}

func symKind(kind symbols.SymbolKind) protocol.CompletionItemKind {
	switch kind {
	case symbols.KindClass:
		return protocol.CompletionItemKindClass
	case symbols.KindInterface:
		return protocol.CompletionItemKindInterface
	case symbols.KindEnum:
		return protocol.CompletionItemKindEnum
	case symbols.KindFunction:
		return protocol.CompletionItemKindFunction
	case symbols.KindMethod:
		return protocol.CompletionItemKindMethod
	case symbols.KindProperty:
		return protocol.CompletionItemKindProperty
	case symbols.KindConstant:
		return protocol.CompletionItemKindConstant
	case symbols.KindEnumCase:
		return protocol.CompletionItemKindEnumMember
	default:
		return protocol.CompletionItemKindText
	}
}

// formatDetail builds the detail string shown next to the completion label.
// Format: ParentFQN::name(params): returnType
// e.g.  Illuminate\Database\Eloquent\Collection::all($columns): array<TKey, TValue>
func formatDetail(sym *symbols.Symbol) string {
	owner := sym.ParentFQN
	switch sym.Kind {
	case symbols.KindMethod:
		params := fmtParams(sym)
		ret := resolveReturnType(sym)
		if owner != "" {
			return fmt.Sprintf("%s::%s(%s): %s", owner, sym.Name, params, ret)
		}
		return fmt.Sprintf("%s(%s): %s", sym.Name, params, ret)
	case symbols.KindProperty:
		typ := sym.Type
		if typ == "" {
			typ = docblockVarType(sym)
		}
		if typ == "" {
			typ = "mixed"
		}
		if owner != "" {
			return fmt.Sprintf("%s::$%s: %s", owner, strings.TrimPrefix(sym.Name, "$"), typ)
		}
		return typ
	case symbols.KindConstant, symbols.KindEnumCase:
		if owner != "" {
			return owner + "::" + sym.Name
		}
		return sym.FQN
	default:
		return sym.FQN
	}
}

func fmtParams(sym *symbols.Symbol) string {
	var params []string
	for _, p := range sym.Params {
		s := ""
		if p.Type != "" {
			s += types.RenderTypeExpr(p.Type) + " "
		}
		if p.IsVariadic {
			s += "..."
		}
		s += p.Name
		params = append(params, s)
	}
	return strings.Join(params, ", ")
}

// resolveReturnType gets the return type from the type hint or @return docblock.
// Keeps generic type parameters (e.g. array<TKey, TValue>) intact.
func resolveReturnType(sym *symbols.Symbol) string {
	if sym.ReturnType != "" {
		return types.RenderTypeExpr(sym.ReturnType)
	}
	if sym.DocComment != "" {
		if doc := parser.ParseDocBlock(sym.DocComment); doc != nil && doc.Return.Type != "" {
			return types.RenderTypeExpr(doc.Return.Type)
		}
	}
	return "mixed"
}

// docblockVarType extracts the type from a @var docblock annotation.
func docblockVarType(sym *symbols.Symbol) string {
	if sym.DocComment == "" {
		return ""
	}
	doc := parser.ParseDocBlock(sym.DocComment)
	if doc == nil {
		return ""
	}
	if vars, ok := doc.Tags["var"]; ok && len(vars) > 0 {
		typ, _ := types.ExtractDocTypeString(vars[0])
		return types.RenderTypeExpr(typ)
	}
	return ""
}

func formatDocumentation(sym *symbols.Symbol) string {
	if sym.DocComment == "" {
		return ""
	}
	doc := parser.ParseDocBlock(sym.DocComment)
	if doc == nil || doc.Summary == "" {
		return ""
	}
	return doc.Summary
}
