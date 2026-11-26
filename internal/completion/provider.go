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
	engine              *resolve.Engine
	framework           string
	arrayResolver       *models.FrameworkArrayResolver
	routes              *frameworklaravel.RouteIndex
	views               *frameworklaravel.Views
	translations        *frameworklaravel.TranslationResolver
	symfonyTranslations *frameworksymfony.TranslationResolver
	symfonyCommands     *frameworksymfony.CommandIndex
	// lazyDocumentation and lazyDetail leave those properties of member
	// items to completionItem/resolve, for clients that ask for them later.
	lazyDocumentation, lazyDetail bool
	// aliasStyle is how a class is imported when its short name is already
	// in use in the file (`autoImport.aliasStyle`).
	aliasStyle string
}

func NewProvider(index *symbols.Index, ca *container.ContainerAnalyzer, framework string) *Provider {
	p := &Provider{index: index, container: ca, engine: resolve.NewEngine(index), framework: framework}
	// Wire up chain resolution so ResolveVariableType can handle $var = Class::method()->chain()
	p.engine.ChainResolver = p.resolveExpressionType
	if ca != nil {
		p.engine.ContainerCall = p.resolveContainerCallType
	}
	return p
}

// ResolveExpressionType resolves the type of an expression like "Category::first()"
// or "$foo->bar()->baz()" as its class names, the way ResolveVariableType
// gives them. Used as the ChainResolver callback and exposed for diagnostics
// type resolution.
func (p *Provider) ResolveExpressionType(expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	return p.resolveExpressionType(p.engine.Resolver(), expr, source, pos, file)
}

func (p *Provider) resolveExpressionType(r *resolve.Resolver, expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	return resolve.PlainTypeName(r.ChainType(expr, source, pos, file))
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
	return p.engine.Resolver().ChainType(expr, source, pos, file)
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

// SetSymfonyCommands gives completion the project's console commands, for
// the name argument of a console application's find(), get() and has().
func (p *Provider) SetSymfonyCommands(commands *frameworksymfony.CommandIndex) {
	p.symfonyCommands = commands
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
	r := p.engine.Resolver()
	lines := r.Lines(source)
	line := resolve.LineAt(lines, pos.Line)
	prefix := ""
	if pos.Character <= len(line) {
		prefix = line[:pos.Character]
	}
	trimmed := strings.TrimSpace(prefix)

	ctx := sourcectx.Analyze(uri, source, pos, p.index.GrammarOf(uri))
	if ctx == nil {
		return nil
	}
	file := ctx.File
	arg, inArg := ctx.CallArg()
	if inArg && p.framework == "laravel" && p.views != nil {
		if viewCtx, ok := frameworklaravel.ViewNameArg(arg); ok {
			return sanitizeCompletions(p.completeLaravelViews(viewCtx))
		}
	}
	rk := newNameRanker(ctx.Namespace, ctx.Uses)

	switch ctx.AccessKind {
	case sourcectx.AccessStatic:
		items := p.completeStaticAccess(r, source, pos, file, ctx.Member.Call)
		return sanitizeCompletions(filterByPrefix(items, ctx.Member.Filter))
	case sourcectx.AccessInstance, sourcectx.AccessNullsafe:
		items := p.completeMemberAccess(r, source, pos, file, ctx.Member.Call)
		return sanitizeCompletions(filterByPrefix(items, ctx.Member.Filter))
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
	if inArg && isBuilderArg(arg) {
		if items := p.completeBuilderArg(r, arg, source, file); items != nil {
			return sanitizeCompletions(items)
		}
	}
	// Array key completion: $var['partial or $var['key1']['partial (nested)
	if ctx := parseArrayKeyContext(prefix); ctx != nil {
		return sanitizeCompletions(p.completeArrayKeys(source, pos, ctx, file))
	}
	if configCtx := configResultContext(ctx); configCtx != nil {
		return sanitizeCompletions(p.completeConfigResultKeys(configCtx))
	}
	if inArg {
		if configPath, partial, quote, ok := configArgContext(arg); ok {
			return sanitizeCompletions(p.completeConfigKeys(configPath, partial, quote))
		}
		if p.framework == "laravel" && p.routes != nil && frameworklaravel.IsRouteNameArg(arg) {
			return sanitizeCompletions(p.completeLaravelRouteNames(arg.Partial, arg.Quote))
		}
	}
	if p.framework == "laravel" && p.translations != nil {
		if txCtx := frameworklaravel.DetectTranslationContext(line, pos.Character); txCtx != nil {
			return sanitizeCompletions(p.translations.Complete(txCtx))
		}
	}
	if p.framework == "symfony" {
		if inArg && frameworksymfony.IsRouteNameArg(arg) {
			return sanitizeCompletions(p.completeSymfonyRouteNames(arg.Partial, arg.Quote))
		}
		if inArg && p.symfonyCommands != nil && frameworksymfony.IsCommandNameArg(arg) &&
			frameworksymfony.IsConsoleApplication(p.index, resolve.PlainTypeName(r.NodeType(file, arg.Receiver))) {
			return sanitizeCompletions(p.completeSymfonyCommandNames(arg.Partial, arg.Quote))
		}
		if p.symfonyTranslations != nil {
			if txCtx := frameworksymfony.DetectSymfonyTranslationContext(line, pos.Character); txCtx != nil {
				return sanitizeCompletions(p.symfonyTranslations.Complete(txCtx))
			}
		}
	}
	if inArg && isContainerArg(arg, source) {
		return sanitizeCompletions(p.completeContainerResolve(source, arg.Partial, arg.Quote, file, rk))
	}
	// Variables in scope: `$` or `$na`
	if typed, ok := typedVariablePrefix(prefix); ok {
		return sanitizeCompletions(p.completeVariables(r, typed, lines, pos, file))
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

// completeSymfonyCommandNames offers the project's console command names
// starting with partial.
func (p *Provider) completeSymfonyCommandNames(partial, quote string) []protocol.CompletionItem {
	var items []protocol.CompletionItem
	lowerPartial := strings.ToLower(partial)
	wrap := "'"
	if quote == `"` {
		wrap = `"`
	}
	for _, cmd := range p.symfonyCommands.All() {
		if lowerPartial != "" && !strings.HasPrefix(strings.ToLower(cmd.Name), lowerPartial) {
			continue
		}
		insertText := cmd.Name
		if quote == "" {
			insertText = wrap + cmd.Name + wrap
		}
		detail := cmd.Description
		if detail == "" {
			detail = cmd.ClassFQN
		}
		items = append(items, protocol.CompletionItem{
			Label:      cmd.Name,
			Kind:       protocol.CompletionItemKindValue,
			Detail:     detail,
			InsertText: insertText,
			SortText:   "0" + cmd.Name,
		})
	}
	return items
}

func (p *Provider) completeMemberAccess(r *resolve.Resolver, source string, pos protocol.Position, file *parser.FileNode, parenAfterCursor bool) []protocol.CompletionItem {
	resolved := r.ReceiverTypeAt(source, pos, file)
	if resolved.FQN == "object" && len(resolved.Fields) > 0 {
		return objectShapeCompletions(resolved)
	}
	typeName := resolved.MemberOwner()
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
	for _, mixin := range r.MixinTypes(resolved) {
		members = append(members, p.index.GetClassMembers(mixin.MemberOwner())...)
	}
	distance := memberDistances(members)
	seen := make(map[string]bool, len(members))
	// Local scopes of the model a query builder is for: $query->active().
	for _, sc := range r.Scopes(resolved) {
		seen[sc.Name] = true
		items = append(items, p.scopeItem(sc, parenAfterCursor))
	}
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
		item := protocol.CompletionItem{Label: m.Name, SortText: rankKey(rankMember, distance[m.ParentFQN], strings.TrimPrefix(m.Name, "$"))}
		p.describeMember(&item, m)
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

// SetAutoImportAliasStyle sets how a class is imported when its short name
// is already in use in the file: "suffix" (the default), "prefix" or
// "fullyQualified".
func (p *Provider) SetAutoImportAliasStyle(style string) {
	p.aliasStyle = normalizeAliasStyle(style)
}

// SetLazyResolve leaves the documentation and/or the detail of member items
// to ResolveCompletionItem. Only for clients that resolve those properties:
// others would never see them.
func (p *Provider) SetLazyResolve(documentation, detail bool) {
	p.lazyDocumentation, p.lazyDetail = documentation, detail
}

// describeMember fills in a member item's detail and documentation, or marks
// it to be filled in by completionItem/resolve. Over a class with hundreds of
// members, formatting them all is most of a completion request's time.
func (p *Provider) describeMember(item *protocol.CompletionItem, m *symbols.Symbol) {
	if !p.lazyDetail {
		item.Detail = formatDetail(m)
	}
	if !p.lazyDocumentation {
		item.Documentation = formatDocumentation(m)
	}
	if p.lazyDetail || p.lazyDocumentation {
		item.Data = &protocol.CompletionItemData{Symbol: m.FQN}
	}
}

// ResolveCompletionItem fills in what describeMember left for later.
func (p *Provider) ResolveCompletionItem(item protocol.CompletionItem) protocol.CompletionItem {
	if item.Data == nil || item.Data.Symbol == "" {
		return item
	}
	sym := p.index.Lookup(item.Data.Symbol)
	if sym == nil {
		return item
	}
	if item.Detail == "" {
		item.Detail = formatDetail(sym)
	}
	if item.Documentation == "" {
		item.Documentation = formatDocumentation(sym)
	}
	return item
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

func (p *Provider) completeStaticAccess(r *resolve.Resolver, source string, pos protocol.Position, file *parser.FileNode, parenAfterCursor bool) []protocol.CompletionItem {
	recv, _ := r.ReceiverAt(source, pos, file)
	typeName := ""
	// $class:: where $class is a class-string<Foo>.
	if t := resolve.ClassStringTarget(recv.Type); t.FQN != "" && !resolve.IsBuiltinTypeName(t.FQN) {
		typeName = t.FQN
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
	if recv.ForwardsCaller {
		enclosing = resolve.FindEnclosingClass(file, pos)
		if m := resolve.FindEnclosingMethod(file, pos); m != nil && !m.IsStatic {
			instanceCalls = true
		}
	}
	var items []protocol.CompletionItem
	seen := map[string]bool{}
	memberSets := []string{typeName}
	forwarded := map[string]bool{}
	if facadeConcrete != "" {
		memberSets = append(memberSets, facadeConcrete)
		forwarded[facadeConcrete] = true
	}
	// What the class forwards static calls to, as an Eloquent model does to
	// its query builder, and that type's @mixin (the base query builder).
	for _, fwd := range r.ForwardTypes(resolve.ResolvedType{FQN: typeName}) {
		for _, t := range append([]resolve.ResolvedType{fwd}, r.MixinTypes(fwd)...) {
			if fqn := t.MemberOwner(); fqn != "" && !forwarded[fqn] {
				memberSets = append(memberSets, fqn)
				forwarded[fqn] = true
			}
		}
	}
	// Local scopes, callable statically through the forward: Category::active().
	for _, fwd := range r.ForwardTypes(resolve.ResolvedType{FQN: typeName}) {
		for _, sc := range r.Scopes(fwd) {
			if seen[sc.Name] {
				continue
			}
			seen[sc.Name] = true
			items = append(items, p.scopeItem(sc, parenAfterCursor))
		}
	}
	for set, fqn := range memberSets {
		members := p.index.GetClassMembers(fqn)
		distance := memberDistances(members)
		for _, m := range members {
			isFacadeForwarded := forwarded[fqn] && m.Kind == symbols.KindMethod && m.Visibility == "public" && !strings.HasPrefix(m.Name, "__")
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
			item := protocol.CompletionItem{Label: m.Name, SortText: rankKey(rankMember, set*100+distance[m.ParentFQN], strings.TrimPrefix(m.Name, "$"))}
			p.describeMember(&item, m)
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
	imports := p.newImportPlanner(source, file)
	for _, sym := range p.index.SearchByPrefix(search) {
		if sym.Kind != symbols.KindClass {
			continue
		}
		name, edits := imports.class(sym.FQN)
		item := protocol.CompletionItem{Label: sym.Name, Kind: protocol.CompletionItemKindClass, Detail: sym.FQN, InsertText: name + "($0)", InsertTextFormat: 2, SortText: rk.key(sym, sym.Name)}
		item.AdditionalTextEdits = edits
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
		imports := p.newImportPlanner(source, file)
		for _, sym := range p.index.SearchByPrefix(prefix) {
			if sym == nil || sym.Kind != symbols.KindClass || seen[sym.Name] {
				continue
			}
			seen[sym.Name] = true
			item := protocol.CompletionItem{
				Label:    sym.Name,
				Kind:     protocol.CompletionItemKindClass,
				Detail:   sym.FQN,
				SortText: rk.key(sym, sym.Name),
			}
			name, edits := imports.class(sym.FQN)
			if name != sym.Name {
				item.InsertText = name
			}
			item.AdditionalTextEdits = edits
			items = append(items, item)
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
	var imports *importPlanner
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
				if imports == nil {
					imports = p.newImportPlanner(source, file)
				}
				item := protocol.CompletionItem{Label: sym.Name, Kind: symKind(sym.Kind), Detail: sym.FQN, SortText: rk.key(sym, sym.Name)}
				name, edits := imports.class(sym.FQN)
				if name != sym.Name {
					item.InsertText = name
				}
				item.AdditionalTextEdits = edits
				items = append(items, item)
			}
		}
	}
	return items
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

// scopeItem is the completion item for a model's local scope, called by its
// scope name (active for scopeActive) on the model or its query builder.
func (p *Provider) scopeItem(sc resolve.Scope, parenAfterCursor bool) protocol.CompletionItem {
	item := protocol.CompletionItem{Label: sc.Name, SortText: rankKey(rankMember, 0, sc.Name)}
	p.describeMember(&item, sc.Method)
	item.Label = sc.Name
	item.Kind = protocol.CompletionItemKindMethod
	if parenAfterCursor {
		item.InsertText = sc.Name
	} else {
		item.InsertText = sc.Name + "($0)"
		item.InsertTextFormat = 2
	}
	return item
}
