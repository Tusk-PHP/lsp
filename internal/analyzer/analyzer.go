package analyzer

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/scope"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type Analyzer struct {
	index               *symbols.Index
	container           *container.ContainerAnalyzer
	engine              *resolve.Engine
	framework           string
	routes              *frameworklaravel.RouteIndex
	views               *frameworklaravel.Views
	translations        *frameworklaravel.TranslationResolver
	symfonyTranslations *frameworksymfony.TranslationResolver
	symfonyCommands     *frameworksymfony.CommandIndex
	// clientCommands lists the commands the client handles itself, from the
	// clientCommands initialization option. Code actions whose result only
	// makes sense client-side (e.g. copying to the clipboard) are offered only
	// when their command is listed here.
	clientCommands map[protocol.CommandName]bool
}

// DefinitionResult is the extended return shape from FindDefinitionWithManual.
// When ExternalManualURL is non-empty, the caller should treat this as
// "open this URL in an external browser" — Location will be nil. When
// ExternalManualURL is empty, Location is the standard go-to-definition
// target (which itself may be nil if no target was resolved).
//
// The two fields are mutually exclusive: at most one is populated per call.
type DefinitionResult struct {
	Location          *protocol.Location
	ExternalManualURL string
}

func NewAnalyzer(index *symbols.Index, ca *container.ContainerAnalyzer) *Analyzer {
	framework := ""
	if ca != nil {
		framework = ca.Framework()
	}
	return &Analyzer{index: index, container: ca, engine: container.NewEngine(index, ca), framework: framework}
}

func (a *Analyzer) SetChainResolver(fn func(expr string, source string, pos protocol.Position, file *parser.FileNode) string) {
	a.engine.DelegateChains(fn)
}

func (a *Analyzer) SetTypedChainResolver(fn func(expr string, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType) {
	a.engine.DelegateTypedChains(fn)
}

func (a *Analyzer) SetTranslationResolver(resolver *frameworklaravel.TranslationResolver) {
	a.translations = resolver
}

func (a *Analyzer) SetSymfonyTranslationResolver(resolver *frameworksymfony.TranslationResolver) {
	a.symfonyTranslations = resolver
}

// SetSymfonyCommands gives go-to-definition the project's console commands,
// for the name argument of a console application's find(), get() and has().
func (a *Analyzer) SetSymfonyCommands(commands *frameworksymfony.CommandIndex) {
	a.symfonyCommands = commands
}

// SetClientCommands records the commands the client says it handles itself.
func (a *Analyzer) SetClientCommands(commands []protocol.CommandName) {
	a.clientCommands = make(map[protocol.CommandName]bool, len(commands))
	for _, c := range commands {
		a.clientCommands[c] = true
	}
}

func (a *Analyzer) SetLaravelRouteIndex(routeIndex *frameworklaravel.RouteIndex) {
	a.routes = routeIndex
}

func (a *Analyzer) SetViewResolver(resolver *frameworklaravel.Views) {
	a.views = resolver
}

func (a *Analyzer) FindDefinition(uri, source string, pos protocol.Position) *protocol.Location {
	return a.resolveDefinitionSymbol(a.engine.Resolver(), uri, source, pos).loc
}

// declaredInSource swaps a property that has no position of its own for the
// method of the same name, if the class has one. An ide-helper
// @property-read $tags in a class docblock declares an Eloquent relation's
// property without saying where; tags() is what declares it in the code.
// Only go to definition does this: hover and types keep the property, whose
// type is the one the access has.
func declaredInSource(r *resolve.Resolver, owner resolve.ResolvedType, word string, member *symbols.Symbol) *symbols.Symbol {
	if member.Kind != symbols.KindProperty || member.Range != (protocol.Range{}) {
		return member
	}
	if method := r.FindAccessedMember(owner.BaseFQN(), word, resolve.CallAccess); method != nil && method.Kind == symbols.KindMethod {
		return method
	}
	return member
}

// classKeywordFQN returns the class that self, static or parent stands for at
// the cursor (in any letter case), or "" when word isn't one of them or there
// is no enclosing class. A word after -> or :: is a member, never the keyword.
func (a *Analyzer) classKeywordFQN(word string, ctx *sourcectx.CursorContext) string {
	if ctx == nil || ctx.AccessKind != sourcectx.AccessNone || ctx.EnclosingFQN == "" {
		return ""
	}
	switch strings.ToLower(word) {
	case "self", "static":
		return ctx.EnclosingFQN
	case "parent":
		if chain := a.index.GetInheritanceChain(ctx.EnclosingFQN); len(chain) > 0 {
			return chain[0]
		}
	}
	return ""
}

// FindDefinitionWithManual is like FindDefinition but, when openManualEnabled
// is true and the resolved symbol is a built-in (functions, classes, members),
// returns an ExternalManualURL pointing at the php.net manual page instead of
// a Location. When openManualEnabled is false, the behavior is identical to
// FindDefinition: builtins resolve to nil and project symbols return a
// Location as usual.
//
// locale is forwarded to symbols.PHPManualURL. Empty string is fine —
// the URL builder falls back to "en".
func (a *Analyzer) FindDefinitionWithManual(uri, source string, pos protocol.Position, openManualEnabled bool, locale string) DefinitionResult {
	target := a.resolveDefinitionSymbol(a.engine.Resolver(), uri, source, pos)
	if !openManualEnabled || target.sym == nil || !symbols.IsBuiltin(target.sym) {
		return DefinitionResult{Location: target.loc}
	}
	return DefinitionResult{ExternalManualURL: symbols.PHPManualURL(target.sym, target.owner, locale)}
}

// FindDeclaration returns where the symbol at pos is first declared: for a
// member reached through a class, or named in its own declaration, the
// interface, parent class or trait member it implements or overrides
// (symbols.Index.Prototype); for anything else, or a member nothing above
// declares, its definition.
func (a *Analyzer) FindDeclaration(uri, source string, pos protocol.Position) *protocol.Location {
	r := a.engine.Resolver()
	target := a.resolveDefinitionSymbol(r, uri, source, pos)
	if target.receiver == "" {
		return target.loc
	}
	if declared := symbolLocation(a.index.Prototype(target.receiver, target.sym)); declared != nil {
		return declared
	}
	return target.loc
}

// definitionTarget is what the cursor names: the symbol, and for a member the
// class that declares it and the class it was reached through, or declared in
// when the cursor is on its declaration. receiver is empty for a symbol found
// by its name alone, which may be any member of that name. loc is where
// the symbol is, nil for a builtin (symbolLocation filters them out) or for
// nothing resolved; callers telling the two apart inspect sym.
type definitionTarget struct {
	sym      *symbols.Symbol
	owner    *symbols.Symbol
	receiver string
	loc      *protocol.Location
}

// resolveDefinitionSymbol resolves the symbol at the cursor position.
func (a *Analyzer) resolveDefinitionSymbol(r *resolve.Resolver, uri, source string, pos protocol.Position) definitionTarget {
	ctx := sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri))
	if ctx == nil {
		return definitionTarget{}
	}
	if l := a.frameworkDefinition(r, source, ctx); l != nil {
		return definitionTarget{loc: l}
	}
	return a.symbolTarget(r, uri, source, ctx)
}

func (a *Analyzer) frameworkDefinition(r *resolve.Resolver, source string, ctx *sourcectx.CursorContext) *protocol.Location {
	line, pos := ctx.Line, ctx.Position
	if arg, ok := ctx.CallArg(); ok {
		if l := a.definitionForCallArg(r, arg, ctx.File); l != nil {
			return l
		}
	}
	if a.container != nil {
		if l := a.definitionForContainerArg(r, line, pos, source, ctx.File, false); l != nil {
			return l
		}
	}
	if a.framework == "laravel" && a.routes != nil {
		if l := a.definitionForRouteName(line, pos.Character); l != nil {
			return l
		}
	}
	return nil
}

func (a *Analyzer) symbolTarget(r *resolve.Resolver, uri, source string, ctx *sourcectx.CursorContext) definitionTarget {
	pos, file := ctx.Position, ctx.File
	word := ctx.SymbolText
	if word == "" {
		return definitionTarget{}
	}

	// Handle $variable → go to its type definition. After `::` a $name is a
	// static property (self::$cache), found with the other members below.
	if strings.HasPrefix(word, "$") && ctx.AccessKind != sourcectx.AccessStatic {
		return definitionTarget{loc: a.definitionForVariable(r, word, source, pos, file)}
	}

	// self, static and parent go to the class they stand for.
	if fqn := a.classKeywordFQN(word, ctx); fqn != "" {
		if s := a.index.Lookup(fqn); s != nil {
			return definitionTarget{sym: s, loc: symbolLocation(s)}
		}
		return definitionTarget{}
	}

	// Check for -> or :: access (method/property on a class)
	if ctx.AccessKind != sourcectx.AccessNone {
		owner := r.ReceiverTypeAt(source, pos, file)
		classFQN := owner.BaseFQN()
		if classFQN == "" {
			return definitionTarget{}
		}
		if member := r.AccessedMemberOn(owner, word, ctx.MemberAccess()); member != nil {
			member = declaredInSource(r, owner, word, member)
			// Owner is the *declaring* class (member.ParentFQN), not the
			// receiver — that's how php.net documents methods/properties:
			// ReflectionMethod::getParameters lives at
			// reflectionfunctionabstract.getparameters.php.
			ownerFQN := member.ParentFQN
			if ownerFQN == "" {
				ownerFQN = classFQN
			}
			ownerSym := a.index.Lookup(ownerFQN)
			return definitionTarget{sym: member, owner: ownerSym, receiver: classFQN, loc: symbolLocation(member)}
		}
		return definitionTarget{}
	}

	if member := a.index.MemberDeclaredAt(uri, pos, word); member != nil {
		return definitionTarget{sym: member, owner: a.index.Lookup(member.ParentFQN), receiver: member.ParentFQN, loc: symbolLocation(member)}
	}
	if ctx.FunctionCall {
		fn := r.CalledFunction(word, file)
		return definitionTarget{sym: fn, loc: symbolLocation(fn)}
	}

	// Resolve via use statements
	if file != nil {
		for _, u := range ctx.Uses {
			if u.Alias == word {
				if s := a.index.Lookup(u.FullName); s != nil {
					return definitionTarget{sym: s, loc: symbolLocation(s)}
				}
			}
		}
		// Try as class name in current namespace
		fqn := r.ResolveClassName(word, file)
		if fqn != word {
			if s := a.index.Lookup(fqn); s != nil {
				return definitionTarget{sym: s, loc: symbolLocation(s)}
			}
		}
	}

	// FQN with backslashes
	if strings.Contains(word, "\\") {
		if s := a.index.Lookup(word); s != nil {
			return definitionTarget{sym: s, loc: symbolLocation(s)}
		}
	}

	best := a.index.LookupStandalone(word, ctx.EnclosingFQN)
	return definitionTarget{sym: best, loc: symbolLocation(best)}
}

func (a *Analyzer) definitionForRouteName(line string, character int) *protocol.Location {
	name, ok := frameworklaravel.FindRouteNameReference(line, character)
	if !ok {
		return nil
	}
	route := a.routes.Find(name)
	if route == nil {
		return nil
	}
	return &protocol.Location{URI: route.URI, Range: route.Range}
}

func (a *Analyzer) definitionForCallArg(r *resolve.Resolver, arg sourcectx.CallArg, file *parser.FileNode) *protocol.Location {
	if arg.Value != sourcectx.ArgString || arg.Text == "" {
		return nil
	}
	if ctx, ok := frameworklaravel.ViewNameArg(arg); ok && a.framework == "laravel" && a.views != nil {
		if view := a.views.Find(ctx.Name); view != nil {
			return &protocol.Location{URI: view.URI}
		}
	}
	if ctx, ok := frameworklaravel.TranslationKeyArg(arg); ok && a.translations != nil {
		if l := a.translations.Definition(ctx.Key); l != nil {
			return l
		}
	}
	if a.framework == "symfony" {
		return a.symfonyCallArgDefinition(r, arg, file)
	}
	return nil
}

func (a *Analyzer) symfonyCallArgDefinition(r *resolve.Resolver, arg sourcectx.CallArg, file *parser.FileNode) *protocol.Location {
	if frameworksymfony.IsRouteNameArg(arg) {
		if route := frameworksymfony.FindRoute(a.index, arg.Text); route != nil {
			return &protocol.Location{URI: route.URI, Range: route.DeclRange}
		}
	}
	if a.symfonyCommands != nil && frameworksymfony.IsCommandNameArg(arg) {
		if cmd := a.symfonyCommands.Find(arg.Text); cmd != nil &&
			frameworksymfony.IsConsoleApplication(a.index, resolve.PlainTypeName(r.NodeType(file, arg.Receiver))) {
			return &protocol.Location{URI: cmd.URI, Range: cmd.Range}
		}
	}
	if ctx, ok := frameworksymfony.TranslationKeyArg(arg); ok && a.symfonyTranslations != nil {
		if ctx.Domain != "" {
			return a.symfonyTranslations.DefinitionInDomain(ctx.Key, ctx.Domain)
		}
		return a.symfonyTranslations.Definition(ctx.Key)
	}
	return nil
}

// definitionForVariable resolves a $variable to its type's definition.
func (a *Analyzer) definitionForVariable(r *resolve.Resolver, varName string, source string, pos protocol.Position, file *parser.FileNode) *protocol.Location {
	if file == nil {
		return nil
	}
	typeName := r.ResolveVariableType(varName, r.Lines(source), pos, file)
	if typeName == "" {
		return nil
	}
	// A variable that may hold several values goes to the first class of them.
	for _, name := range strings.Split(typeName, "|") {
		if sym := a.index.Lookup(name); sym != nil {
			return symbolLocation(sym)
		}
	}
	return nil
}

// definitionForContainerArg checks if the cursor is inside a container resolution
// call argument (e.g. app('request'), app(Request::class)) and returns the
// definition of the concrete class.
func (a *Analyzer) definitionForContainerArg(r *resolve.Resolver, line string, pos protocol.Position, source string, file *parser.FileNode, preferType bool) *protocol.Location {
	prefix := line[:min(pos.Character, len(line))]
	// Check if we're inside a container call
	patterns := []string{"app(", "resolve(", "->get(", "->make("}
	insideCall := false
	var argStart int
	for _, pat := range patterns {
		idx := strings.LastIndex(prefix, pat)
		if idx < 0 {
			continue
		}
		after := prefix[idx+len(pat):]
		// If there's already a closing paren before cursor, we're past the call
		if strings.Contains(after, ")") {
			continue
		}
		insideCall = true
		argStart = idx + len(pat)
		break
	}
	if !insideCall {
		return nil
	}

	// Also grab everything after cursor to the closing paren on the same line
	rest := line[min(pos.Character, len(line)):]
	closeIdx := strings.Index(rest, ")")
	var fullArg string
	if closeIdx >= 0 {
		fullArg = line[argStart : pos.Character+closeIdx]
	} else {
		fullArg = line[argStart:min(pos.Character, len(line))]
	}

	arg := strings.TrimSpace(fullArg)
	// Strip first arg only
	if commaIdx := strings.Index(arg, ","); commaIdx >= 0 {
		arg = strings.TrimSpace(arg[:commaIdx])
	}
	arg = strings.Trim(arg, "'\"")

	wasClassConst := false
	if strings.HasSuffix(arg, "::class") {
		wasClassConst = true
		className := strings.TrimSuffix(arg, "::class")
		arg = r.ResolveClassName(className, file)
	}

	if !preferType && !wasClassConst {
		if binding := a.container.LookupBinding(arg); binding != nil && binding.DefinitionURI != "" {
			return &protocol.Location{URI: binding.DefinitionURI, Range: binding.Definition}
		}
	}

	// Resolve via container bindings
	if binding := a.container.ResolveDependency(arg); binding != nil {
		if sym := a.index.Lookup(binding.Concrete); sym != nil {
			return symbolLocation(sym)
		}
	}
	// Direct FQN lookup
	if sym := a.index.Lookup(arg); sym != nil {
		return symbolLocation(sym)
	}
	return nil
}

func symbolLocation(sym *symbols.Symbol) *protocol.Location {
	if sym == nil || sym.URI == "" || symbols.IsBuiltin(sym) {
		return nil
	}
	return &protocol.Location{URI: sym.URI, Range: sym.Range}
}

func singleLocation(loc *protocol.Location) []protocol.Location {
	if loc == nil {
		return nil
	}
	return []protocol.Location{*loc}
}

func (a *Analyzer) FindTypeDefinition(uri, source string, pos protocol.Position) []protocol.Location {
	r := a.engine.Resolver()
	ctx := sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri))
	if ctx == nil {
		return nil
	}
	word := ctx.SymbolText
	if word == "" {
		return nil
	}

	file := ctx.File

	if a.container != nil {
		if loc := a.definitionForContainerArg(r, ctx.Line, pos, source, file, true); loc != nil {
			return singleLocation(loc)
		}
	}

	if strings.HasPrefix(word, "$") {
		return a.locationsForTypeName(r, r.ResolveVariableType(word, r.Lines(source), pos, file), file)
	}

	if ctx.AccessKind != sourcectx.AccessNone {
		classFQN := r.ReceiverTypeAt(source, pos, file).BaseFQN()
		if classFQN == "" {
			return nil
		}
		if member := r.FindMemberAnywhere(classFQN, word); member != nil {
			return a.locationsForMemberType(r, member, file)
		}
		return nil
	}

	sym := a.resolveSymbolAt(r, ctx.Name(), source)
	if sym == nil {
		return nil
	}

	switch sym.Kind {
	case symbols.KindProperty, symbols.KindMethod:
		return a.locationsForMemberType(r, sym, file)
	case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
		return singleLocation(symbolLocation(sym))
	default:
		return nil
	}
}

func (a *Analyzer) FindImplementation(uri, source string, pos protocol.Position) []protocol.Location {
	r := a.engine.Resolver()
	word, _ := sourcectx.WordAt(source, pos)
	if word == "" {
		return nil
	}

	sym := a.resolveSymbolAt(r, sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri)).Name(), source)
	if sym == nil {
		return nil
	}

	switch sym.Kind {
	case symbols.KindInterface:
		return a.findTypeImplementations(sym)
	case symbols.KindClass:
		if sym.IsAbstract {
			return a.findAbstractClassImplementations(sym)
		}
	case symbols.KindMethod:
		parent := a.index.Lookup(sym.ParentFQN)
		if parent == nil {
			return nil
		}
		switch {
		case parent.Kind == symbols.KindInterface:
			return a.findMemberImplementations(r, sym, a.index.GetImplementors(parent.FQN))
		case parent.Kind == symbols.KindClass && parent.IsAbstract && sym.IsAbstract:
			return a.findMemberImplementations(r, sym, a.index.GetDescendants(parent.FQN))
		}
	}

	return nil
}

func (a *Analyzer) locationsForMemberType(r *resolve.Resolver, member *symbols.Symbol, file *parser.FileNode) []protocol.Location {
	return a.locationsForTypeName(r, r.MemberType(member, file), file)
}

func (a *Analyzer) locationsForTypeName(r *resolve.Resolver, typeName string, file *parser.FileNode) []protocol.Location {
	typeName = strings.TrimSpace(typeName)
	typeName = strings.TrimPrefix(typeName, "?")
	if typeName == "" || symbols.IsPHPBuiltinType(typeName) {
		return nil
	}
	if strings.Contains(typeName, "|") {
		typeName = resolve.PickBestUnionPart(typeName)
	}
	if idx := strings.IndexByte(typeName, '<'); idx > 0 {
		typeName = typeName[:idx]
	}
	typeName = r.ResolveClassName(typeName, file)
	return singleLocation(symbolLocation(a.index.Lookup(typeName)))
}

func (a *Analyzer) findTypeImplementations(sym *symbols.Symbol) []protocol.Location {
	return a.classSymbolsToLocations(a.index.GetImplementors(sym.FQN), true)
}

func (a *Analyzer) findAbstractClassImplementations(sym *symbols.Symbol) []protocol.Location {
	return a.classSymbolsToLocations(a.index.GetDescendants(sym.FQN), true)
}

func (a *Analyzer) findMemberImplementations(r *resolve.Resolver, sym *symbols.Symbol, owners []*symbols.Symbol) []protocol.Location {
	var locs []protocol.Location
	for _, owner := range owners {
		if owner == nil || (owner.Kind != symbols.KindClass && owner.Kind != symbols.KindEnum) || owner.IsAbstract {
			continue
		}
		member := r.FindMember(owner.FQN, sym.Name)
		if member == nil || member.IsAbstract {
			continue
		}
		if loc := symbolLocation(member); loc != nil {
			locs = append(locs, *loc)
		}
	}
	return deduplicateLocations(locs)
}

func (a *Analyzer) classSymbolsToLocations(classes []*symbols.Symbol, concreteOnly bool) []protocol.Location {
	var locs []protocol.Location
	for _, classSym := range classes {
		if classSym == nil || (classSym.Kind != symbols.KindClass && classSym.Kind != symbols.KindEnum) {
			continue
		}
		if concreteOnly && classSym.IsAbstract {
			continue
		}
		if loc := symbolLocation(classSym); loc != nil {
			locs = append(locs, *loc)
		}
	}
	return deduplicateLocations(locs)
}

func (a *Analyzer) FindReferences(uri, source string, pos protocol.Position) []protocol.Location {
	return a.FindAllReferences(uri, source, pos, nil)
}

// FindAllReferences finds all occurrences of the symbol at the cursor across
// the workspace. readDocument is used to read file contents; if nil, falls back
// to reading from the index's stored file URIs via disk.
func (a *Analyzer) FindAllReferences(uri, source string, pos protocol.Position, readDocument func(string) string) []protocol.Location {
	r := a.engine.Resolver()
	word, _ := sourcectx.WordAt(source, pos)
	if word == "" {
		return nil
	}

	file := parser.ParseFileIn(source, a.index.GrammarOf(uri))

	// Try to resolve as a symbol first — this handles properties ($name in declarations)
	// and member access contexts before falling back to variable scope
	ctx := sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri))
	sym := a.resolveSymbolAt(r, ctx.Name(), source)

	// Built-in symbols are opaque to navigation: do not scan the workspace for refs.
	if symbols.IsBuiltin(sym) {
		return nil
	}

	// W1 guard: if we were in a member access chain and the receiver/member couldn't
	// be resolved, do not fall through to name-based lookups.
	if sym == nil && ctx != nil && (ctx.AccessKind != sourcectx.AccessNone || ctx.FunctionCall) {
		return nil
	}

	// Variable references — local scope only (if not resolved as a property/symbol)
	if sym == nil && strings.HasPrefix(word, "$") && word != "$this" {
		return a.findVariableReferences(uri, source, pos, word, file)
	}
	if sym == nil {
		// Fallback: definition-only lookup by name (original behavior)
		var locs []protocol.Location
		for _, s := range a.index.LookupByName(word) {
			if !symbols.IsBuiltin(s) && s.URI != "" {
				locs = append(locs, protocol.Location{URI: s.URI, Range: s.Range})
			}
		}
		return locs
	}

	return a.findSymbolOccurrences(sym, readDocument, uri, source)
}

// resolveSymbolAtCursor resolves the word at cursor to a Symbol, handling
// member access chains, use imports, and namespace resolution.
func (a *Analyzer) resolveSymbolAt(r *resolve.Resolver, ctx *sourcectx.NameContext, source string) *symbols.Symbol {
	if ctx == nil || ctx.SymbolText == "" {
		return nil
	}
	word, file, pos := ctx.SymbolText, ctx.File, ctx.Position

	// Check for member access context (->method or ::method).
	// W1 guard: if we're in an access chain but can't resolve the receiver or
	// the member, return nil — do not fall through to name-based lookups that
	// would match unrelated symbols across the project.
	if ctx.AccessKind != sourcectx.AccessNone {
		owner := r.ReceiverTypeAt(source, pos, file)
		if owner.BaseFQN() == "" {
			return nil
		}
		return r.AccessedMemberOn(owner, word, ctx.MemberAccess())
	}
	if ctx.FunctionCall {
		return r.CalledFunction(word, file)
	}

	// Check if it's a class property declaration ($name on a property line)
	if strings.HasPrefix(word, "$") && file != nil {
		bare := strings.TrimPrefix(word, "$")
		classFQN := resolve.FindEnclosingClass(file, pos)
		if classFQN != "" {
			if sym := a.index.Lookup(classFQN + "::" + word); sym != nil {
				return sym
			}
			if sym := a.index.Lookup(classFQN + "::" + bare); sym != nil && sym.Kind == symbols.KindProperty {
				return sym
			}
		}
	}

	// Check if we're on a member declaration inside the enclosing type.
	if ctx.EnclosingFQN != "" {
		if sym := a.index.Lookup(ctx.EnclosingFQN + "::" + word); sym != nil {
			return sym
		}
		if strings.HasPrefix(word, "$") {
			if sym := a.index.Lookup(ctx.EnclosingFQN + "::" + strings.TrimPrefix(word, "$")); sym != nil {
				return sym
			}
		}
	}

	// Try resolving as a class/interface/enum/trait name
	fqn := ""
	if file != nil {
		fqn = r.ResolveClassName(word, file)
	}
	if fqn != "" {
		if sym := a.index.Lookup(fqn); sym != nil {
			return sym
		}
	}

	// Try direct lookup
	if sym := a.index.Lookup(word); sym != nil {
		return sym
	}

	return a.index.LookupStandalone(word, ctx.EnclosingFQN)
}

// findVariableReferences finds all occurrences of a variable within its enclosing function scope.
func (a *Analyzer) findVariableReferences(uri, source string, pos protocol.Position, varName string, file *parser.FileNode) []protocol.Location {
	_ = file
	doc := scope.Collect(source)
	binding := doc.BindingAt(pos)
	if binding == nil || binding.Name != varName {
		return nil
	}
	var locs []protocol.Location
	for _, rng := range doc.Occurrences(binding) {
		locs = append(locs, protocol.Location{URI: uri, Range: rng})
	}
	return locs
}

// readFileFromDisk reads a file from disk given its URI.
func (a *Analyzer) readFileFromDisk(uri string) string {
	path := symbols.URIToPath(uri)
	content, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(content)
}

// deduplicateLocations removes duplicate locations (same URI, line, character).
func deduplicateLocations(locs []protocol.Location) []protocol.Location {
	type key struct {
		uri   string
		sLine int
		sChar int
	}
	seen := make(map[key]bool, len(locs))
	var result []protocol.Location
	for _, loc := range locs {
		k := key{loc.URI, loc.Range.Start.Line, loc.Range.Start.Character}
		if !seen[k] {
			seen[k] = true
			result = append(result, loc)
		}
	}
	return result
}

func deduplicateHighlights(highlights []protocol.DocumentHighlight) []protocol.DocumentHighlight {
	type key struct {
		sLine int
		sChar int
		eLine int
		eChar int
	}
	seen := make(map[key]bool, len(highlights))
	var result []protocol.DocumentHighlight
	for _, highlight := range highlights {
		k := key{
			sLine: highlight.Range.Start.Line,
			sChar: highlight.Range.Start.Character,
			eLine: highlight.Range.End.Line,
			eChar: highlight.Range.End.Character,
		}
		if seen[k] {
			continue
		}
		seen[k] = true
		result = append(result, highlight)
	}
	return result
}

func highlightVariableOccurrences(source string, pos protocol.Position, varName string) []protocol.DocumentHighlight {
	doc := scope.Collect(source)
	binding := doc.BindingAt(pos)
	if binding == nil || binding.Name != varName {
		return nil
	}
	ranges := doc.Occurrences(binding)
	highlights := make([]protocol.DocumentHighlight, 0, len(ranges))
	for _, rng := range ranges {
		kind := protocol.DocumentHighlightKindRead
		if sameRange(rng, binding.Decl) {
			kind = protocol.DocumentHighlightKindWrite
		}
		highlights = append(highlights, protocol.DocumentHighlight{Range: rng, Kind: kind})
	}
	return deduplicateHighlights(highlights)
}

func protocolSymbolKind(kind symbols.SymbolKind) protocol.SymbolKind {
	switch kind {
	case symbols.KindClass:
		return protocol.SymbolKindClass
	case symbols.KindInterface:
		return protocol.SymbolKindInterface
	case symbols.KindTrait:
		return protocol.SymbolKindClass
	case symbols.KindEnum:
		return protocol.SymbolKindEnum
	case symbols.KindFunction:
		return protocol.SymbolKindFunction
	case symbols.KindMethod:
		return protocol.SymbolKindMethod
	case symbols.KindProperty:
		return protocol.SymbolKindProperty
	case symbols.KindConstant:
		return protocol.SymbolKindConstant
	case symbols.KindEnumCase:
		return protocol.SymbolKindEnumMember
	default:
		return protocol.SymbolKindObject
	}
}

func symbolContainerName(sym *symbols.Symbol) string {
	switch sym.Kind {
	case symbols.KindMethod, symbols.KindProperty, symbols.KindConstant, symbols.KindEnumCase:
		return sym.ParentFQN
	default:
		if idx := strings.LastIndex(sym.FQN, "\\"); idx >= 0 {
			return sym.FQN[:idx]
		}
	}
	return ""
}

func sameRange(a, b protocol.Range) bool {
	return a.Start == b.Start && a.End == b.End
}

func (a *Analyzer) GetDocumentSymbols(uri, source string) []protocol.DocumentSymbol {
	file := parser.ParseFileIn(source, a.index.GrammarOf(uri))
	if file == nil {
		return nil
	}
	var ds []protocol.DocumentSymbol
	for _, cls := range file.Classes {
		if cls.Name == "" {
			continue
		}
		s := protocol.DocumentSymbol{Name: cls.Name, Kind: protocol.SymbolKindClass,
			Range: mkRange(cls.StartLine), SelectionRange: mkRange(cls.StartLine)}
		for _, m := range cls.Methods {
			if m.Name == "" {
				continue
			}
			s.Children = append(s.Children, protocol.DocumentSymbol{Name: m.Name, Detail: m.Visibility, Kind: protocol.SymbolKindMethod, Range: mkRange(m.StartLine), SelectionRange: mkRange(m.StartLine)})
		}
		for _, p := range cls.Properties {
			if p.Name == "" {
				continue
			}
			s.Children = append(s.Children, protocol.DocumentSymbol{Name: p.Name, Detail: p.Type.Name, Kind: protocol.SymbolKindProperty, Range: mkRange(p.StartLine), SelectionRange: mkRange(p.StartLine)})
		}
		ds = append(ds, s)
	}
	for _, iface := range file.Interfaces {
		if iface.Name == "" {
			continue
		}
		s := protocol.DocumentSymbol{Name: iface.Name, Kind: protocol.SymbolKindInterface, Range: mkRange(iface.StartLine), SelectionRange: mkRange(iface.StartLine)}
		ds = append(ds, s)
	}
	for _, en := range file.Enums {
		if en.Name == "" {
			continue
		}
		ds = append(ds, protocol.DocumentSymbol{Name: en.Name, Kind: protocol.SymbolKindEnum, Range: mkRange(en.StartLine), SelectionRange: mkRange(en.StartLine)})
	}
	for _, fn := range file.Functions {
		if fn.Name == "" {
			continue
		}
		ds = append(ds, protocol.DocumentSymbol{Name: fn.Name, Kind: protocol.SymbolKindFunction, Range: mkRange(fn.StartLine), SelectionRange: mkRange(fn.StartLine)})
	}
	return ds
}

func (a *Analyzer) GetFoldingRanges(uri, source string) []protocol.FoldingRange {
	ranges := parser.ExtractFoldingRanges(source)
	if len(ranges) == 0 {
		return nil
	}
	out := make([]protocol.FoldingRange, 0, len(ranges))
	for _, rng := range ranges {
		out = append(out, protocol.FoldingRange{
			StartLine: rng.StartLine,
			EndLine:   rng.EndLine,
			Kind:      protocol.FoldingRangeKind(rng.Kind),
		})
	}
	return out
}

const maxWorkspaceSymbols = 500

func emptyQueryScore(sym *symbols.Symbol) int {
	if sym.IsClassLike() {
		return 0
	}
	return 1
}

func (a *Analyzer) GetWorkspaceSymbols(query string) []protocol.SymbolInformation {
	query = strings.TrimSpace(query)

	var candidates []*symbols.Symbol
	switch {
	case query == "":
		candidates = a.index.FirstDeclaredByName(maxWorkspaceSymbols, symbols.SymbolKind.IsClassLike)
		candidates = append(candidates, a.index.FirstDeclaredByName(maxWorkspaceSymbols-len(candidates), func(k symbols.SymbolKind) bool { return !k.IsClassLike() })...)
	case strings.Contains(query, "\\"):
		candidates = a.index.SearchByPrefix(query)
		if fqns, _ := a.index.SearchByFQNPrefix(strings.TrimPrefix(query, "\\")); len(fqns) > 0 {
			candidates = append(candidates, fqns...)
		}
	default:
		candidates = a.index.SearchByPrefix(query)
	}

	type scoredSymbol struct {
		sym   *symbols.Symbol
		score int
	}

	seen := make(map[string]bool, len(candidates))
	scored := make([]scoredSymbol, 0, len(candidates))
	lowerQuery := strings.ToLower(query)

	for _, sym := range candidates {
		if sym == nil || sym.FQN == "" || sym.Name == "" || sym.URI == "" || symbols.IsBuiltin(sym) {
			continue
		}
		if seen[sym.FQN] {
			continue
		}
		seen[sym.FQN] = true

		score := 3
		switch {
		case query == "":
			score = emptyQueryScore(sym)
		case strings.EqualFold(sym.Name, query):
			score = 0
		case strings.EqualFold(sym.FQN, strings.TrimPrefix(query, "\\")):
			score = 1
		case strings.HasPrefix(strings.ToLower(sym.Name), lowerQuery):
			score = 2
		}
		scored = append(scored, scoredSymbol{sym: sym, score: score})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score < scored[j].score
		}
		if scored[i].sym.Name != scored[j].sym.Name {
			return scored[i].sym.Name < scored[j].sym.Name
		}
		if scored[i].sym.FQN != scored[j].sym.FQN {
			return scored[i].sym.FQN < scored[j].sym.FQN
		}
		return scored[i].sym.URI < scored[j].sym.URI
	})

	scored = scored[:min(len(scored), maxWorkspaceSymbols)]
	results := make([]protocol.SymbolInformation, 0, len(scored))
	for _, item := range scored {
		sym := item.sym
		results = append(results, protocol.SymbolInformation{
			Name:          sym.Name,
			Kind:          protocolSymbolKind(sym.Kind),
			Location:      protocol.Location{URI: sym.URI, Range: sym.Range},
			ContainerName: symbolContainerName(sym),
		})
	}
	return results
}

func (a *Analyzer) GetSignatureHelp(uri, source string, pos protocol.Position) *protocol.SignatureHelp {
	callee, activeParam, ok := sourcectx.CallAt(uri, source, pos, a.index.GrammarOf(uri))
	if !ok {
		return nil
	}
	sym := a.symbolTarget(a.engine.Resolver(), uri, source, callee).sym
	if sym == nil {
		return nil
	}
	sig := protocol.SignatureInformation{Label: sym.Name + formatParamLabel(sym)}
	for _, p := range sym.Params {
		l := ""
		if p.Type != "" {
			l = p.Type + " "
		}
		l += p.Name
		sig.Parameters = append(sig.Parameters, protocol.ParameterInformation{Label: l})
	}
	// Clamp activeParam so it never exceeds the last valid parameter index.
	// e.g. a 2-parameter function must have activeParameter in [0, 1].
	if maxParam := len(sym.Params) - 1; maxParam < 0 {
		activeParam = 0
	} else if activeParam > maxParam {
		activeParam = maxParam
	}
	return &protocol.SignatureHelp{Signatures: []protocol.SignatureInformation{sig}, ActiveParameter: activeParam}
}

// formatParams returns the parenthesized parameter list for a symbol, e.g.
// "(Type $a, $b)". Each parameter is rendered as "Type $name" when a type is
// present, or just "$name" otherwise. The return type is not included.
func formatParams(sym *symbols.Symbol) string {
	var ps []string
	for _, p := range sym.Params {
		s := ""
		if p.Type != "" {
			s = p.Type + " "
		}
		s += p.Name
		ps = append(ps, s)
	}
	return "(" + strings.Join(ps, ", ") + ")"
}

func formatParamLabel(sym *symbols.Symbol) string {
	ret := sym.ReturnType
	if ret == "" {
		ret = "mixed"
	}
	return formatParams(sym) + ": " + ret
}

// ReferenceVariant is one labeled copy-reference format option.
type ReferenceVariant struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// CopyReference resolves the symbol under the cursor and returns a set of
// labeled reference-string formats (FQN/short × bare/call/full-signature) for
// the editor to present and copy. Returns nil when the cursor is not on a
// resolvable named symbol.
func (a *Analyzer) CopyReference(uri, source string, pos protocol.Position) []ReferenceVariant {
	r := a.engine.Resolver()
	ctx := sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri))
	sym := a.resolveSymbolAt(r, ctx.Name(), source)
	if sym == nil {
		return nil
	}

	// Determine whether this is a type-level symbol (class/interface/enum/trait)
	// or a member-level symbol (method/property/constant/etc.).
	isTypeKind := sym.Kind == symbols.KindClass ||
		sym.Kind == symbols.KindInterface ||
		sym.Kind == symbols.KindEnum ||
		sym.Kind == symbols.KindTrait

	var classFQN string
	if isTypeKind {
		classFQN = sym.FQN
	} else {
		classFQN = sym.ParentFQN
	}
	if parser.IsAnonymousClass(classFQN) {
		return nil
	}

	// Fall back to treating the symbol itself as the class when ParentFQN is
	// absent (e.g. standalone functions have no parent).
	if !isTypeKind && classFQN == "" {
		classFQN = sym.FQN
		isTypeKind = true // emit class variants only
	}

	// Derive the short (unqualified) class name from the FQN.
	shortName := classFQN
	if i := strings.LastIndex(classFQN, "\\"); i >= 0 {
		shortName = classFQN[i+1:]
	}

	if isTypeKind {
		return []ReferenceVariant{
			{Label: "FQN", Value: classFQN},
			{Label: "Short name", Value: shortName},
		}
	}

	switch sym.Kind {
	case symbols.KindProperty:
		prop := sym.Name
		if !strings.HasPrefix(prop, "$") {
			prop = "$" + prop
		}
		return []ReferenceVariant{
			{Label: "FQN::$property", Value: classFQN + "::" + prop},
			{Label: "Short::$property", Value: shortName + "::" + prop},
			{Label: "FQN", Value: classFQN},
			{Label: "Short name", Value: shortName},
		}
	case symbols.KindConstant, symbols.KindEnumCase:
		return []ReferenceVariant{
			{Label: "FQN::CONSTANT", Value: classFQN + "::" + sym.Name},
			{Label: "Short::CONSTANT", Value: shortName + "::" + sym.Name},
			{Label: "FQN", Value: classFQN},
			{Label: "Short name", Value: shortName},
		}
	}

	// Method symbol: build call/signature variants.
	m := sym.Name
	params := formatParams(sym)
	ret := ""
	if sym.ReturnType != "" {
		ret = ": " + sym.ReturnType
	}

	return []ReferenceVariant{
		{Label: "FQN::member()", Value: classFQN + "::" + m + "()"},
		{Label: "FQN::member(signature)", Value: classFQN + "::" + m + params + ret},
		{Label: "Short::member()", Value: shortName + "::" + m + "()"},
		{Label: "Short::member(signature)", Value: shortName + "::" + m + params + ret},
		{Label: "FQN", Value: classFQN},
		{Label: "Short name", Value: shortName},
	}
}

func mkRange(line int) protocol.Range {
	return protocol.Range{Start: protocol.Position{Line: line}, End: protocol.Position{Line: line}}
}

// --- Rename support ---

// getWordRangeAt returns the word at the cursor and its exact range in the document.
func getWordRangeAt(source string, pos protocol.Position) (string, protocol.Range) {
	return sourcectx.WordAt(source, pos)
}

// PrepareRename checks if the symbol at the cursor can be renamed and returns
// the range and current name as a placeholder.
func (a *Analyzer) PrepareRename(uri, source string, pos protocol.Position) *protocol.PrepareRenameResult {
	r := a.engine.Resolver()
	ctx := sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri))
	if ctx == nil {
		return nil
	}
	word, wordRange := ctx.SymbolText, ctx.WordRange
	if word == "" || word == "$this" {
		return nil
	}

	file := ctx.File

	// Try resolving as a symbol (class, method, property, function, etc.)
	sym := a.resolveSymbolAt(r, ctx.Name(), source)

	// Reject built-ins
	if sym != nil && symbols.IsBuiltin(sym) {
		return nil
	}

	// Reject vendor and generated symbols (renaming dependencies is not
	// useful, and generated classes have no file to edit)
	if sym != nil && (sym.Source == symbols.SourceVendor || sym.Source == symbols.SourceGenerated) {
		return nil
	}

	if sym != nil && sym.AliasOf != nil {
		return nil
	}

	// If resolved as a known symbol, allow rename
	if sym != nil {
		// For properties, show the placeholder with the appropriate form
		placeholder := word
		if sym.Kind == symbols.KindProperty && !strings.HasPrefix(word, "$") {
			// Cursor is on ->prop access, show just the bare name
			placeholder = strings.TrimPrefix(sym.Name, "$")
		}
		return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: placeholder}
	}

	// Variables are always renameable (local scope)
	if strings.HasPrefix(word, "$") {
		binding := scope.Collect(source).BindingAt(pos)
		if !isPrepareRenameVariableBindingSupported(binding, word) {
			return nil
		}
		return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
	}

	// Check for member access context (method/property on -> or ::).
	// If we reach here with an access kind set, the receiver was unresolvable;
	// return nil rather than allowing a rename that could affect wrong symbols.
	if ctx.AccessKind != sourcectx.AccessNone || ctx.FunctionCall {
		return nil
	}

	// Allow rename for identifiers declared in the current file's AST
	if file != nil {
		for _, cls := range file.Classes {
			if cls.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
			for _, m := range cls.Methods {
				if m.Name == word {
					return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
				}
			}
		}
		for _, fn := range file.Functions {
			if fn.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
		for _, iface := range file.Interfaces {
			if iface.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
		for _, en := range file.Enums {
			if en.Name == word {
				return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
			}
		}
	}

	return nil
}

func isPrepareRenameVariableBindingSupported(binding *scope.Binding, word string) bool {
	if binding == nil || binding.Name != word {
		return false
	}
	switch binding.Kind {
	case scope.BindingVariable,
		scope.BindingParameter,
		scope.BindingForeachKey,
		scope.BindingForeachValue,
		scope.BindingCatch,
		scope.BindingDestructure,
		scope.BindingClosureUse:
		return true
	default:
		return false
	}
}

// Rename performs a rename of the symbol at the given position.
// readDocument is used to read file contents for files not open in the editor.
func (a *Analyzer) Rename(uri, source string, pos protocol.Position, newName string, readDocument func(string) string) *protocol.WorkspaceEdit {
	r := a.engine.Resolver()
	ctx := sourcectx.Analyze(uri, source, pos, a.index.GrammarOf(uri))
	if ctx == nil {
		return nil
	}
	word := ctx.SymbolText
	if word == "" || word == "$this" {
		return nil
	}

	file := ctx.File

	// Try resolving as a symbol first (handles properties, methods, classes, etc.)
	sym := a.resolveSymbolAt(r, ctx.Name(), source)

	// If it's a known symbol, rename it across the workspace
	if sym != nil {
		if symbols.IsBuiltin(sym) || sym.Source == symbols.SourceVendor || sym.Source == symbols.SourceGenerated || sym.AliasOf != nil {
			return nil
		}
		return a.renameSymbol(sym, newName, readDocument, uri, source)
	}

	// Fall back to variable rename (local scope)
	if strings.HasPrefix(word, "$") {
		return a.renameVariable(uri, source, pos, word, newName, file)
	}

	return nil
}

// renameVariable renames a variable within its enclosing function scope.
func (a *Analyzer) renameVariable(uri, source string, pos protocol.Position, oldName, newName string, file *parser.FileNode) *protocol.WorkspaceEdit {
	if !strings.HasPrefix(newName, "$") {
		newName = "$" + newName
	}
	_ = file
	doc := scope.Collect(source)
	binding := doc.BindingAt(pos)
	if binding == nil || binding.Name != oldName {
		return nil
	}

	var edits []protocol.TextEdit
	for _, rng := range doc.Occurrences(binding) {
		edits = append(edits, protocol.TextEdit{Range: rng, NewText: newName})
	}

	if len(edits) == 0 {
		return nil
	}
	return &protocol.WorkspaceEdit{
		Changes: map[string][]protocol.TextEdit{uri: edits},
	}
}

// renameSymbol renames a class, method, property, function, interface, enum, or trait
// across the entire workspace by finding all occurrences and generating TextEdits.
func (a *Analyzer) renameSymbol(sym *symbols.Symbol, newName string, readDocument func(string) string, uri, source string) *protocol.WorkspaceEdit {
	locs := a.findSymbolOccurrences(sym, readDocument, uri, source)
	if len(locs) == 0 {
		return nil
	}

	// Determine the replacement text based on what each location represents
	isProperty := sym.Kind == symbols.KindProperty
	bareName := strings.TrimPrefix(sym.Name, "$")
	newBare := strings.TrimPrefix(newName, "$")

	changes := make(map[string][]protocol.TextEdit)
	for _, loc := range locs {
		replaceText := newName
		if isProperty {
			// Check if this location is a $prop declaration or ->prop access
			locLen := loc.Range.End.Character - loc.Range.Start.Character
			if locLen == len(bareName) {
				// Access form (->prop) — use bare name
				replaceText = newBare
			} else {
				// Declaration form ($prop) — use $name
				if !strings.HasPrefix(newName, "$") {
					replaceText = "$" + newName
				}
			}
		}
		changes[loc.URI] = append(changes[loc.URI], protocol.TextEdit{
			Range:   loc.Range,
			NewText: replaceText,
		})
	}

	return &protocol.WorkspaceEdit{Changes: changes}
}

// findWordEdits finds all occurrences of `word` on `line` that appear as complete
// identifiers (not part of a longer word) and returns TextEdits to replace them.
func findWordEdits(line string, lineNum int, oldWord, newWord string) []protocol.TextEdit {
	var edits []protocol.TextEdit
	offset := 0
	for {
		idx := strings.Index(line[offset:], oldWord)
		if idx < 0 {
			break
		}
		col := offset + idx
		end := col + len(oldWord)
		// Check word boundary before
		if col > 0 && (resolve.IsWordChar(line[col-1]) || line[col-1] == '$') {
			offset = end
			continue
		}
		// Check word boundary after
		if end < len(line) && resolve.IsWordChar(line[end]) {
			offset = end
			continue
		}
		edits = append(edits, protocol.TextEdit{
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: col},
				End:   protocol.Position{Line: lineNum, Character: end},
			},
			NewText: newWord,
		})
		offset = end
	}
	return edits
}

// --- Code actions ---

// GetCodeActions returns available code actions for the given range.
func (a *Analyzer) GetCodeActions(uri, source string, params protocol.CodeActionParams) []protocol.CodeAction {
	actions := make([]protocol.CodeAction, 0)

	parseResult := parser.New().Parse(source)
	if parseResult == nil {
		return actions
	}

	file := parser.ParseFileIn(source, a.index.GrammarOf(uri))
	if file == nil {
		return actions
	}

	actions = append(actions, a.unknownClassCodeActions(uri, source, file, params)...)
	actions = append(actions, a.importUnusedImportQuickFixes(uri, source, params)...)
	actions = append(actions, IgnoreDiagnosticActions(uri, source, params)...)
	if action := a.generateConstructorCodeAction(uri, source, parseResult, params.Context.Only, params.Range.Start); action != nil {
		actions = append(actions, *action)
	}
	actions = append(actions, a.generateGetterSetterCodeActions(uri, source, parseResult, params.Context.Only, params.Range.Start)...)
	actions = append(actions, a.implementMissingMethodsCodeActions(uri, source, parseResult, params.Context.Only, params.Range.Start)...)
	if action := a.extractVariableCodeAction(uri, source, params); action != nil {
		actions = append(actions, *action)
	}
	if action := a.inlineVariableCodeAction(uri, source, params); action != nil {
		actions = append(actions, *action)
	}
	if action := a.importOrganizeImportsAction(uri, source, file, params.Context.Only); action != nil {
		actions = append(actions, *action)
	}

	// Offer "Copy Namespace" for any position
	ns := file.Namespace
	primaryClass := ""
	for _, cls := range file.Classes {
		primaryClass = cls.Name
		break
	}
	if primaryClass == "" {
		for _, iface := range file.Interfaces {
			primaryClass = iface.Name
			break
		}
	}
	if primaryClass == "" {
		for _, en := range file.Enums {
			primaryClass = en.Name
			break
		}
	}

	fqn := ns
	if primaryClass != "" {
		if ns != "" {
			fqn = ns + "\\" + primaryClass
		} else {
			fqn = primaryClass
		}
	}

	if fqn != "" {
		uriJSON, _ := json.Marshal(uri)
		if protocol.CodeActionSource.In(params.Context.Only) {
			actions = append(actions, protocol.CodeAction{
				Title:   "Copy Namespace: " + fqn,
				Kind:    protocol.CodeActionSource,
				Command: &protocol.Command{Title: "Copy Namespace", Command: protocol.CommandCopyNamespace, Arguments: []json.RawMessage{uriJSON}},
			})
		}
	}

	// Offer "Move to namespace" when cursor is on a type declaration
	if primaryClass != "" && ns != "" {
		// Check if cursor is on or near a declaration line
		cursorLine := params.Range.Start.Line
		isOnDecl := false
		for _, cls := range file.Classes {
			if cls.StartLine == cursorLine {
				isOnDecl = true
				break
			}
		}
		if !isOnDecl {
			for _, iface := range file.Interfaces {
				if iface.StartLine == cursorLine {
					isOnDecl = true
					break
				}
			}
		}
		if !isOnDecl {
			for _, en := range file.Enums {
				if en.StartLine == cursorLine {
					isOnDecl = true
					break
				}
			}
		}
		if !isOnDecl {
			for _, tr := range file.Traits {
				if tr.StartLine == cursorLine {
					isOnDecl = true
					break
				}
			}
		}
		if isOnDecl {
			uriJSON, _ := json.Marshal(uri)
			// The target namespace will be prompted by the editor extension
			// For now, provide the command with URI; the extension fills in the target
			if protocol.CodeActionRefactorMove.In(params.Context.Only) {
				actions = append(actions, protocol.CodeAction{
					Title: "Move to namespace...",
					Kind:  protocol.CodeActionRefactorMove,
					Command: &protocol.Command{
						Title:     "Move to namespace",
						Command:   protocol.CommandMoveToNamespace,
						Arguments: []json.RawMessage{uriJSON},
					},
				})
			}
		}
	}

	// Offer "Copy reference..." when cursor is on a resolvable named symbol.
	// The command only returns the reference strings; copying one is up to the
	// client, so the action is offered only to clients that declare they handle
	// tuskPhpLsp.copyReference. Elsewhere it would be a menu entry that does nothing.
	if a.clientCommands[protocol.CommandCopyReference] && protocol.CodeActionRefactor.In(params.Context.Only) {
		if refs := a.CopyReference(uri, source, params.Range.Start); len(refs) > 0 {
			uriJSON, _ := json.Marshal(uri)
			posJSON, _ := json.Marshal(params.Range.Start)
			actions = append(actions, protocol.CodeAction{
				Title: "Copy reference...",
				Kind:  protocol.CodeActionRefactor,
				Command: &protocol.Command{
					Title:     "Copy reference",
					Command:   protocol.CommandCopyReference,
					Arguments: []json.RawMessage{uriJSON, posJSON},
				},
			})
		}
	}

	return actions
}

// GetFileNamespace returns the FQN of the primary class/interface/enum in a file,
// or the namespace if no named type exists.
func (a *Analyzer) GetFileNamespace(uri, source string) string {
	file := parser.ParseFileIn(source, a.index.GrammarOf(uri))
	if file == nil {
		return ""
	}
	ns := file.Namespace
	for _, cls := range file.Classes {
		if ns != "" {
			return ns + "\\" + cls.Name
		}
		return cls.Name
	}
	for _, iface := range file.Interfaces {
		if ns != "" {
			return ns + "\\" + iface.Name
		}
		return iface.Name
	}
	for _, en := range file.Enums {
		if ns != "" {
			return ns + "\\" + en.Name
		}
		return en.Name
	}
	return ns
}

// MoveToNamespace moves the primary type in a file to a new namespace, updating
// the namespace declaration, all use imports across the workspace, FQN references,
// and computing the new file path per PSR-4 autoload mappings.
func (a *Analyzer) MoveToNamespace(uri, source, targetNS string, autoloadEntries []composer.AutoloadEntry, readDocument func(string) string) *protocol.WorkspaceEdit {
	file := parser.ParseFileIn(source, a.index.GrammarOf(uri))
	if file == nil {
		return nil
	}

	oldNS := file.Namespace

	// Find the primary type name
	typeName := ""
	for _, cls := range file.Classes {
		typeName = cls.Name
		break
	}
	if typeName == "" {
		for _, iface := range file.Interfaces {
			typeName = iface.Name
			break
		}
	}
	if typeName == "" {
		for _, en := range file.Enums {
			typeName = en.Name
			break
		}
	}
	if typeName == "" {
		for _, tr := range file.Traits {
			typeName = tr.Name
			break
		}
	}
	if typeName == "" || oldNS == "" {
		return nil
	}

	oldFQN := oldNS + "\\" + typeName
	newFQN := targetNS + "\\" + typeName

	edit := &protocol.WorkspaceEdit{
		Changes: make(map[string][]protocol.TextEdit),
	}

	// 1. Update namespace declaration in the source file
	lines := strings.Split(source, "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "namespace ") {
			// Find "namespace OldNS" and replace with "namespace NewNS"
			nsStart := strings.Index(line, "namespace ") + len("namespace ")
			nsEnd := nsStart
			for nsEnd < len(line) && line[nsEnd] != ';' && line[nsEnd] != ' ' && line[nsEnd] != '{' {
				nsEnd++
			}
			edit.Changes[uri] = append(edit.Changes[uri], protocol.TextEdit{
				Range: protocol.Range{
					Start: protocol.Position{Line: i, Character: nsStart},
					End:   protocol.Position{Line: i, Character: nsEnd},
				},
				NewText: targetNS,
			})
			break
		}
	}

	// 2. Update all references across the workspace
	//
	// We need to handle several reference patterns:
	//   a) Direct import:  use App\Http\Controllers\CategoryController;
	//   b) FQN reference:  \App\Http\Controllers\CategoryController::class
	//   c) Namespace import + partial ref:  use App\Http\Controllers;
	//      then  Controllers\CategoryController::class
	//   d) Parent namespace import:  use App\Http;
	//      then  Http\Controllers\CategoryController::class
	//
	// For (c) and (d), we need to compute the relative path change.
	// Old: App\Http\Controllers\CategoryController
	// New: App\Http\Controllers\Api\CategoryController
	// If a file imports "use App\Http\Controllers;" (alias "Controllers"),
	// then "Controllers\CategoryController" must become "Controllers\Api\CategoryController"

	allURIs := a.index.GetAllFileURIs()
	for _, fileURI := range allURIs {
		if fileURI == uri {
			continue // Skip the source file (namespace already updated above)
		}
		var fileSource string
		if readDocument != nil {
			fileSource = readDocument(fileURI)
		} else {
			fileSource = a.readFileFromDisk(fileURI)
		}
		if fileSource == "" {
			continue
		}

		fileFile := parser.ParseFileIn(fileSource, a.index.GrammarOf(fileURI))
		fileLines := strings.Split(fileSource, "\n")
		var fileEdits []protocol.TextEdit

		// Build a map of partial reference patterns from this file's use statements.
		// For each use statement that imports a parent namespace of the old FQN,
		// compute what the old partial reference looks like and what it should become.
		type partialRef struct {
			oldRef string // e.g. "Controllers\CategoryController"
			newRef string // e.g. "Controllers\Api\CategoryController"
		}
		var partials []partialRef

		if fileFile != nil {
			for _, u := range fileFile.Uses {
				// Check if this use statement imports a prefix of the old FQN
				// e.g. use App\Http\Controllers; (FullName = "App\Http\Controllers", Alias = "Controllers")
				if oldFQN == u.FullName {
					// Direct import of the exact class — handled below in (a)
					continue
				}
				if strings.HasPrefix(oldFQN, u.FullName+"\\") {
					// This use statement imports a parent namespace
					// The remainder after the imported namespace is the partial reference
					remainder := strings.TrimPrefix(oldFQN, u.FullName+"\\")
					oldPartial := u.Alias + "\\" + remainder

					newRemainder := strings.TrimPrefix(newFQN, u.FullName+"\\")
					if strings.HasPrefix(newFQN, u.FullName+"\\") {
						// New FQN still shares the same prefix — just update the remainder
						newPartial := u.Alias + "\\" + newRemainder
						partials = append(partials, partialRef{oldRef: oldPartial, newRef: newPartial})
					} else {
						// New FQN has a completely different prefix — the partial ref
						// becomes invalid; the use statement itself needs updating.
						// This is handled by the FQN replacement below.
					}
				}
			}
		}

		for i, line := range fileLines {
			trimmed := strings.TrimSpace(line)

			// (a) Direct import: "use OldFQN;" or "use OldFQN as Alias;"
			if strings.HasPrefix(trimmed, "use ") && strings.Contains(line, oldFQN) {
				idx := strings.Index(line, oldFQN)
				if idx >= 0 {
					fileEdits = append(fileEdits, protocol.TextEdit{
						Range: protocol.Range{
							Start: protocol.Position{Line: i, Character: idx},
							End:   protocol.Position{Line: i, Character: idx + len(oldFQN)},
						},
						NewText: newFQN,
					})
					continue
				}
			}

			// (b) Fully-qualified references: \OldFQN
			fqnWithSlash := "\\" + oldFQN
			newFQNWithSlash := "\\" + newFQN
			offset := 0
			for {
				idx := strings.Index(line[offset:], fqnWithSlash)
				if idx < 0 {
					break
				}
				col := offset + idx
				end := col + len(fqnWithSlash)
				if end < len(line) && resolve.IsWordChar(line[end]) {
					offset = end
					continue
				}
				fileEdits = append(fileEdits, protocol.TextEdit{
					Range: protocol.Range{
						Start: protocol.Position{Line: i, Character: col},
						End:   protocol.Position{Line: i, Character: end},
					},
					NewText: newFQNWithSlash,
				})
				offset = end
			}

			// (c) Partial namespace references: Controllers\CategoryController
			for _, pr := range partials {
				offset := 0
				for {
					idx := strings.Index(line[offset:], pr.oldRef)
					if idx < 0 {
						break
					}
					col := offset + idx
					end := col + len(pr.oldRef)
					// Word boundary check before (should not be preceded by another word char)
					if col > 0 && resolve.IsWordChar(line[col-1]) {
						offset = end
						continue
					}
					// Word boundary check after
					if end < len(line) && resolve.IsWordChar(line[end]) {
						offset = end
						continue
					}
					fileEdits = append(fileEdits, protocol.TextEdit{
						Range: protocol.Range{
							Start: protocol.Position{Line: i, Character: col},
							End:   protocol.Position{Line: i, Character: end},
						},
						NewText: pr.newRef,
					})
					offset = end
				}
			}
		}

		if len(fileEdits) > 0 {
			edit.Changes[fileURI] = fileEdits
		}
	}

	// 3. Compute file move via PSR-4
	newPath := composer.FQNToPath(newFQN, autoloadEntries)
	if newPath != "" {
		oldPath := symbols.URIToPath(uri)
		if newPath != oldPath {
			edit.DocumentChanges = append(edit.DocumentChanges, protocol.DocumentChange{
				RenameFile: &protocol.RenameFile{
					Kind:   "rename",
					OldURI: uri,
					NewURI: "file://" + newPath,
				},
			})
		}
	}

	return edit
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
