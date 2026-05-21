package analyzer

import (
	"encoding/json"
	"os"
	"sort"
	"strings"

	"github.com/open-southeners/tusk-php/internal/composer"
	"github.com/open-southeners/tusk-php/internal/container"
	frameworklaravel "github.com/open-southeners/tusk-php/internal/framework/laravel"
	frameworksymfony "github.com/open-southeners/tusk-php/internal/framework/symfony"
	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
	"github.com/open-southeners/tusk-php/internal/scope"
	sourcectx "github.com/open-southeners/tusk-php/internal/source"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

type Analyzer struct {
	index        *symbols.Index
	container    *container.ContainerAnalyzer
	resolver     *resolve.Resolver
	framework    string
	routes       *frameworklaravel.RouteIndex
	views        *frameworklaravel.Views
	translations *frameworklaravel.TranslationResolver
}

func NewAnalyzer(index *symbols.Index, ca *container.ContainerAnalyzer) *Analyzer {
	framework := ""
	if ca != nil {
		framework = ca.Framework()
	}
	return &Analyzer{index: index, container: ca, resolver: resolve.NewResolver(index), framework: framework}
}

func (a *Analyzer) SetChainResolver(fn func(expr string, source string, pos protocol.Position, file *parser.FileNode) string) {
	a.resolver.ChainResolver = fn
}

func (a *Analyzer) SetTypedChainResolver(fn func(expr string, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType) {
	a.resolver.TypedChainResolver = fn
}

func (a *Analyzer) SetTranslationResolver(resolver *frameworklaravel.TranslationResolver) {
	a.translations = resolver
}

func (a *Analyzer) SetLaravelRouteIndex(routeIndex *frameworklaravel.RouteIndex) {
	a.routes = routeIndex
}

func (a *Analyzer) SetViewResolver(resolver *frameworklaravel.Views) {
	a.views = resolver
}

func (a *Analyzer) FindDefinition(uri, source string, pos protocol.Position) *protocol.Location {
	if a.framework == "symfony" {
		if routeName, _, ok := frameworksymfony.RouteNameAtPosition(source, pos); ok {
			if route := frameworksymfony.FindRoute(a.index, routeName); route != nil {
				return &protocol.Location{URI: route.URI, Range: route.DeclRange}
			}
		}
	}
	ctx := sourcectx.Analyze(uri, source, pos)
	if ctx == nil {
		return nil
	}
	line := ctx.Line

	file := ctx.File

	// Handle container call arguments: app('request'), app(Request::class)
	// Go to the concrete class definition
	if a.container != nil {
		if loc := a.definitionForContainerArg(line, pos, source, file, false); loc != nil {
			return loc
		}
	}
	if a.framework == "laravel" && a.views != nil {
		if loc := a.definitionForLaravelView(line, pos.Character); loc != nil {
			return loc
		}
	}
	if a.translations != nil {
		if loc := a.definitionForTranslationKey(line, pos.Character); loc != nil {
			return loc
		}
	}
	if a.framework == "laravel" && a.routes != nil {
		if loc := a.definitionForRouteName(line, pos.Character); loc != nil {
			return loc
		}
	}

	word := ctx.SymbolText
	if word == "" {
		return nil
	}

	// Handle $variable → go to its type definition
	if strings.HasPrefix(word, "$") {
		return a.definitionForVariable(word, source, pos, file)
	}

	// Check for -> or :: access (method/property on a class)
	if ctx.AccessKind != sourcectx.AccessNone {
		if classFQN := a.resolveAccessChain(ctx.JoinedLine, ctx.JoinedWordStart, source, pos, file); classFQN != "" {
			if sym := a.resolver.FindMember(classFQN, word); sym != nil {
				return symbolLocation(sym)
			}
		}
	}

	// Resolve via use statements
	if file != nil {
		for _, u := range ctx.Uses {
			if u.Alias == word {
				if sym := a.index.Lookup(u.FullName); sym != nil {
					return symbolLocation(sym)
				}
			}
		}
		// Try as class name in current namespace
		fqn := a.resolver.ResolveClassName(word, file)
		if fqn != word {
			if sym := a.index.Lookup(fqn); sym != nil {
				return symbolLocation(sym)
			}
		}
	}

	// FQN with backslashes
	if strings.Contains(word, "\\") {
		if sym := a.index.Lookup(word); sym != nil {
			return symbolLocation(sym)
		}
	}

	// Fallback: lookup by short name — prefer standalone-appropriate symbols
	// (functions/classes over methods/properties) since we have no access chain.
	lookupName := word
	if idx := strings.LastIndex(word, "\\"); idx >= 0 {
		lookupName = word[idx+1:]
	}
	syms := a.index.LookupByName(lookupName)
	if best := symbols.PickBestStandalone(syms, word); best != nil {
		return symbolLocation(best)
	}

	return nil
}

func (a *Analyzer) definitionForTranslationKey(line string, character int) *protocol.Location {
	ctx := frameworklaravel.DetectTranslationContext(line, character)
	if ctx == nil {
		return nil
	}
	return a.translations.Definition(ctx.Key)
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

func (a *Analyzer) definitionForLaravelView(line string, character int) *protocol.Location {
	ctx, ok := frameworklaravel.ExtractViewDefinitionContext(line, character)
	if !ok {
		return nil
	}
	view := a.views.Find(ctx.Name)
	if view == nil {
		return nil
	}
	return &protocol.Location{
		URI: view.URI,
		Range: protocol.Range{
			Start: protocol.Position{Line: 0, Character: 0},
			End:   protocol.Position{Line: 0, Character: 0},
		},
	}
}

// definitionForVariable resolves a $variable to its type's definition.
func (a *Analyzer) definitionForVariable(varName string, source string, pos protocol.Position, file *parser.FileNode) *protocol.Location {
	if file == nil {
		return nil
	}
	typeName := a.resolver.ResolveVariableType(varName, resolve.SplitLines(source), pos, file)
	if typeName == "" {
		return nil
	}
	if sym := a.index.Lookup(typeName); sym != nil {
		return symbolLocation(sym)
	}
	return nil
}

// definitionForContainerArg checks if the cursor is inside a container resolution
// call argument (e.g. app('request'), app(Request::class)) and returns the
// definition of the concrete class.
func (a *Analyzer) definitionForContainerArg(line string, pos protocol.Position, source string, file *parser.FileNode, preferType bool) *protocol.Location {
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
		arg = a.resolver.ResolveClassName(className, file)
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

// resolveAccessChain walks left through -> and :: chains to return the FQN
// of the class that owns the member at wordStart.
func (a *Analyzer) resolveAccessChain(line string, wordStart int, source string, pos protocol.Position, file *parser.FileNode) string {
	i := wordStart
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i < 2 {
		return ""
	}

	if line[i-2] == '-' && line[i-1] == '>' {
		i -= 2
	} else if line[i-2] == ':' && line[i-1] == ':' {
		i -= 2
	} else {
		return ""
	}

	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}

	// Skip past closing paren for method chains
	if i > 0 && line[i-1] == ')' {
		parenEnd := i
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

		// Check if this is a container call: app(...), resolve(...)
		if a.container != nil {
			callExpr := strings.TrimSpace(line[:parenEnd])
			if concrete := a.resolveContainerCallType(callExpr, source, file); concrete != "" {
				return concrete
			}
		}

		for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
			i--
		}
	}

	end := i
	for i > 0 && resolve.IsWordChar(line[i-1]) {
		i--
	}
	if i > 0 && line[i-1] == '$' {
		i--
	}
	if i >= end {
		return ""
	}
	target := line[i:end]

	if file == nil {
		return ""
	}

	switch target {
	case "$this", "self", "static":
		return resolve.FindEnclosingClass(file, pos)
	case "parent":
		classFQN := resolve.FindEnclosingClass(file, pos)
		if classFQN == "" {
			return ""
		}
		chain := a.index.GetInheritanceChain(classFQN)
		if len(chain) > 0 {
			return chain[0]
		}
		return ""
	}

	if strings.HasPrefix(target, "$") {
		return a.resolver.ResolveVariableType(target, resolve.SplitLines(source), pos, file)
	}

	// Try as a class name (for static access like Logger::create)
	if fqn := a.resolver.ResolveClassName(target, file); fqn != "" {
		if sym := a.index.Lookup(fqn); sym != nil {
			switch sym.Kind {
			case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
				return fqn
			}
		}
	}

	// Recursive chain resolution
	ownerFQN := a.resolveAccessChain(line, i, source, pos, file)
	if ownerFQN == "" {
		return ""
	}
	member := a.resolver.FindMember(ownerFQN, target)
	if member == nil {
		return ""
	}
	return a.resolver.MemberType(member, file)
}

// resolveContainerCallType resolves a container call expression to a concrete FQN.
// Uses the shared ExtractContainerCallArg helper from the resolve package.
func (a *Analyzer) resolveContainerCallType(expr string, source string, file *parser.FileNode) string {
	arg := resolve.ExtractContainerCallArg(expr)
	if arg == "" {
		return ""
	}
	if strings.HasSuffix(arg, "::class") {
		className := strings.TrimSuffix(arg, "::class")
		arg = a.resolver.ResolveClassName(className, file)
	}
	if binding := a.container.ResolveDependency(arg); binding != nil {
		return binding.Concrete
	}
	if a.index.Lookup(arg) != nil {
		return arg
	}
	return ""
}

func symbolLocation(sym *symbols.Symbol) *protocol.Location {
	if sym.URI == "" || sym.URI == "builtin" {
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
	ctx := sourcectx.Analyze(uri, source, pos)
	if ctx == nil {
		return nil
	}
	word := ctx.SymbolText
	if word == "" {
		return nil
	}

	file := ctx.File

	if a.container != nil {
		if loc := a.definitionForContainerArg(ctx.Line, pos, source, file, true); loc != nil {
			return singleLocation(loc)
		}
	}

	if strings.HasPrefix(word, "$") {
		return a.locationsForTypeName(a.resolver.ResolveVariableType(word, resolve.SplitLines(source), pos, file), file)
	}

	if ctx.AccessKind != sourcectx.AccessNone {
		classFQN := a.resolveAccessChain(ctx.JoinedLine, ctx.JoinedWordStart, source, pos, file)
		if classFQN != "" {
			if member := a.resolver.FindMember(classFQN, word); member != nil {
				return a.locationsForMemberType(member, file)
			}
		}
	}

	sym := a.resolveSymbolAtCursor(uri, source, pos, word, file)
	if sym == nil {
		return nil
	}

	switch sym.Kind {
	case symbols.KindProperty, symbols.KindMethod:
		return a.locationsForMemberType(sym, file)
	case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
		return singleLocation(symbolLocation(sym))
	default:
		return nil
	}
}

func (a *Analyzer) FindImplementation(uri, source string, pos protocol.Position) []protocol.Location {
	word, _ := sourcectx.WordAt(source, pos)
	if word == "" {
		return nil
	}

	file := parser.ParseFile(source)
	sym := a.resolveSymbolAtCursor(uri, source, pos, word, file)
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
			return a.findMemberImplementations(sym, a.index.GetImplementors(parent.FQN))
		case parent.Kind == symbols.KindClass && parent.IsAbstract && sym.IsAbstract:
			return a.findMemberImplementations(sym, a.index.GetDescendants(parent.FQN))
		}
	}

	return nil
}

func (a *Analyzer) locationsForMemberType(member *symbols.Symbol, file *parser.FileNode) []protocol.Location {
	return a.locationsForTypeName(a.resolver.MemberType(member, file), file)
}

func (a *Analyzer) locationsForTypeName(typeName string, file *parser.FileNode) []protocol.Location {
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
	typeName = a.resolver.ResolveClassName(typeName, file)
	return singleLocation(symbolLocation(a.index.Lookup(typeName)))
}

func (a *Analyzer) findTypeImplementations(sym *symbols.Symbol) []protocol.Location {
	return a.classSymbolsToLocations(a.index.GetImplementors(sym.FQN), true)
}

func (a *Analyzer) findAbstractClassImplementations(sym *symbols.Symbol) []protocol.Location {
	return a.classSymbolsToLocations(a.index.GetDescendants(sym.FQN), true)
}

func (a *Analyzer) findMemberImplementations(sym *symbols.Symbol, owners []*symbols.Symbol) []protocol.Location {
	var locs []protocol.Location
	for _, owner := range owners {
		if owner == nil || (owner.Kind != symbols.KindClass && owner.Kind != symbols.KindEnum) || owner.IsAbstract {
			continue
		}
		member := a.resolver.FindMember(owner.FQN, sym.Name)
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
	word, _ := sourcectx.WordAt(source, pos)
	if word == "" {
		return nil
	}

	file := parser.ParseFile(source)

	// Try to resolve as a symbol first — this handles properties ($name in declarations)
	// and member access contexts before falling back to variable scope
	sym := a.resolveSymbolAtCursor(uri, source, pos, word, file)

	// Variable references — local scope only (if not resolved as a property/symbol)
	if sym == nil && strings.HasPrefix(word, "$") && word != "$this" {
		return a.findVariableReferences(uri, source, pos, word, file)
	}
	if sym == nil {
		// Fallback: definition-only lookup by name (original behavior)
		var locs []protocol.Location
		for _, s := range a.index.LookupByName(word) {
			if s.URI != "builtin" {
				locs = append(locs, protocol.Location{URI: s.URI, Range: s.Range})
			}
		}
		return locs
	}

	return a.findSymbolOccurrences(sym, readDocument)
}

// resolveSymbolAtCursor resolves the word at cursor to a Symbol, handling
// member access chains, use imports, and namespace resolution.
func (a *Analyzer) resolveSymbolAtCursor(uri, source string, pos protocol.Position, word string, file *parser.FileNode) *symbols.Symbol {
	ctx := sourcectx.Analyze(uri, source, pos)

	// Check for member access context (->method or ::method)
	if ctx != nil && ctx.AccessKind != sourcectx.AccessNone {
		classFQN := a.resolveAccessChain(ctx.JoinedLine, ctx.JoinedWordStart, source, pos, file)
		if classFQN != "" {
			if member := a.resolver.FindMember(classFQN, word); member != nil {
				return member
			}
		}
	}

	// Check if it's a class property declaration ($name on a property line)
	if strings.HasPrefix(word, "$") && file != nil {
		bare := strings.TrimPrefix(word, "$")
		classFQN := resolve.FindEnclosingClass(file, pos)
		if classFQN != "" {
			propFQN := classFQN + "::" + bare
			if sym := a.index.Lookup(propFQN); sym != nil {
				return sym
			}
			// Also try with $ prefix (some indexes store it with $)
			propFQN = classFQN + "::" + word
			if sym := a.index.Lookup(propFQN); sym != nil {
				return sym
			}
		}
	}

	// Check if we're on a member declaration inside the enclosing type.
	if ctx != nil && ctx.EnclosingFQN != "" {
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
		fqn = a.resolver.ResolveClassName(word, file)
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

	// Fallback by short name
	syms := a.index.LookupByName(word)
	return symbols.PickBestStandalone(syms, word)
}

// findSymbolOccurrences scans all indexed files for references to the given symbol.
// Returns locations for both definition sites and usage sites.
func (a *Analyzer) findSymbolOccurrences(sym *symbols.Symbol, readDocument func(string) string) []protocol.Location {
	var locs []protocol.Location
	name := sym.Name

	// Include the definition itself
	if sym.URI != "" && sym.URI != "builtin" {
		locs = append(locs, protocol.Location{URI: sym.URI, Range: sym.Range})
	}

	bareName := strings.TrimPrefix(name, "$")

	allURIs := a.index.GetAllFileURIs()
	for _, fileURI := range allURIs {
		var source string
		if readDocument != nil {
			source = readDocument(fileURI)
		}
		// Prefer the in-memory source stored by the index to avoid redundant disk I/O.
		// Fall back to disk only when neither the caller-supplied reader nor the index
		// has a stored copy (e.g. files indexed without source via IndexFile on older paths).
		if source == "" {
			source = a.index.GetFileSource(fileURI)
		}
		if source == "" {
			source = a.readFileFromDisk(fileURI)
		}
		if source == "" {
			continue
		}
		lines := strings.Split(source, "\n")

		for i, line := range lines {
			switch sym.Kind {
			case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
				locs = append(locs, findWordLocations(fileURI, line, i, name)...)
				// Also find as last segment of FQN (e.g. \Cacheable in use App\Contracts\Cacheable)
				locs = append(locs, findAccessLocations(fileURI, line, i, "\\"+name, 1, name)...)

			case symbols.KindMethod:
				locs = append(locs, findWordLocations(fileURI, line, i, name)...)

			case symbols.KindProperty:
				// Declaration form: $name
				locs = append(locs, findWordLocations(fileURI, line, i, "$"+bareName)...)
				// Access form: ->name
				locs = append(locs, findAccessLocations(fileURI, line, i, "->"+bareName, 2, bareName)...)

			case symbols.KindFunction:
				locs = append(locs, findWordLocations(fileURI, line, i, name)...)

			case symbols.KindConstant, symbols.KindEnumCase:
				locs = append(locs, findWordLocations(fileURI, line, i, name)...)
			}
		}
	}

	// Deduplicate (definition may appear twice: once from sym.Range, once from file scan)
	return deduplicateLocations(locs)
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

// findWordLocations finds all word-boundary occurrences of `word` on a line.
func findWordLocations(uri, line string, lineNum int, word string) []protocol.Location {
	var locs []protocol.Location
	offset := 0
	for {
		idx := strings.Index(line[offset:], word)
		if idx < 0 {
			break
		}
		col := offset + idx
		end := col + len(word)
		// Word boundary before
		if col > 0 && (resolve.IsWordChar(line[col-1]) || line[col-1] == '$') {
			offset = end
			continue
		}
		// Word boundary after
		if end < len(line) && resolve.IsWordChar(line[end]) {
			offset = end
			continue
		}
		locs = append(locs, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: col},
				End:   protocol.Position{Line: lineNum, Character: end},
			},
		})
		offset = end
	}
	return locs
}

// findAccessLocations finds occurrences of a needle (e.g. "->name") on a line
// and returns locations pointing to the identifier part (after skipChars).
func findAccessLocations(uri, line string, lineNum int, needle string, skipChars int, identName string) []protocol.Location {
	var locs []protocol.Location
	offset := 0
	for {
		idx := strings.Index(line[offset:], needle)
		if idx < 0 {
			break
		}
		col := offset + idx + skipChars
		end := col + len(identName)
		if end < len(line) && resolve.IsWordChar(line[end]) {
			offset = end
			continue
		}
		locs = append(locs, protocol.Location{
			URI: uri,
			Range: protocol.Range{
				Start: protocol.Position{Line: lineNum, Character: col},
				End:   protocol.Position{Line: lineNum, Character: end},
			},
		})
		offset = end
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
	case symbols.KindVariable:
		return protocol.SymbolKindVariable
	case symbols.KindEnumCase:
		return protocol.SymbolKindEnumMember
	case symbols.KindNamespace:
		return protocol.SymbolKindNamespace
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

func highlightKindForRange(rng protocol.Range, declRange protocol.Range, cursorRange protocol.Range) protocol.DocumentHighlightKind {
	switch {
	case sameRange(rng, declRange):
		return protocol.DocumentHighlightKindWrite
	case sameRange(rng, cursorRange):
		return protocol.DocumentHighlightKindText
	default:
		return protocol.DocumentHighlightKindRead
	}
}

func sameRange(a, b protocol.Range) bool {
	return a.Start == b.Start && a.End == b.End
}

func (a *Analyzer) GetDocumentSymbols(uri, source string) []protocol.DocumentSymbol {
	file := parser.ParseFile(source)
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

func (a *Analyzer) GetWorkspaceSymbols(query string) []protocol.SymbolInformation {
	query = strings.TrimSpace(query)

	candidates := a.index.SearchByPrefix(query)
	if strings.Contains(query, "\\") {
		if fqns, _ := a.index.SearchByFQNPrefix(strings.TrimPrefix(query, "\\")); len(fqns) > 0 {
			candidates = append(candidates, fqns...)
		}
	}

	type scoredSymbol struct {
		sym   *symbols.Symbol
		score int
	}

	seen := make(map[string]bool, len(candidates))
	scored := make([]scoredSymbol, 0, len(candidates))
	lowerQuery := strings.ToLower(query)

	for _, sym := range candidates {
		if sym == nil || sym.FQN == "" || sym.Name == "" || sym.URI == "" || sym.URI == "builtin" {
			continue
		}
		if seen[sym.FQN] {
			continue
		}
		seen[sym.FQN] = true

		score := 3
		switch {
		case lowerQuery == "":
			score = 3
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

func (a *Analyzer) GetDocumentHighlights(uri, source string, pos protocol.Position) []protocol.DocumentHighlight {
	word, wordRange := sourcectx.WordAt(source, pos)
	if word == "" {
		return nil
	}

	file := parser.ParseFile(source)
	sym := a.resolveSymbolAtCursor(uri, source, pos, word, file)
	if sym == nil && strings.HasPrefix(word, "$") && word != "$this" {
		return highlightVariableOccurrences(source, pos, word)
	}

	lines := strings.Split(source, "\n")
	if sym == nil {
		var highlights []protocol.DocumentHighlight
		for i, line := range lines {
			for _, loc := range findWordLocations(uri, line, i, word) {
				highlights = append(highlights, protocol.DocumentHighlight{Range: loc.Range, Kind: protocol.DocumentHighlightKindText})
			}
		}
		return deduplicateHighlights(highlights)
	}

	var highlights []protocol.DocumentHighlight
	for i, line := range lines {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindEnum, symbols.KindTrait:
			for _, loc := range findWordLocations(uri, line, i, sym.Name) {
				highlights = append(highlights, protocol.DocumentHighlight{Range: loc.Range, Kind: highlightKindForRange(loc.Range, sym.Range, wordRange)})
			}
			for _, loc := range findAccessLocations(uri, line, i, "\\"+sym.Name, 1, sym.Name) {
				highlights = append(highlights, protocol.DocumentHighlight{Range: loc.Range, Kind: protocol.DocumentHighlightKindRead})
			}
		case symbols.KindMethod, symbols.KindFunction, symbols.KindConstant, symbols.KindEnumCase:
			for _, loc := range findWordLocations(uri, line, i, sym.Name) {
				highlights = append(highlights, protocol.DocumentHighlight{Range: loc.Range, Kind: highlightKindForRange(loc.Range, sym.Range, wordRange)})
			}
		case symbols.KindProperty:
			bareName := strings.TrimPrefix(sym.Name, "$")
			for _, loc := range findWordLocations(uri, line, i, "$"+bareName) {
				highlights = append(highlights, protocol.DocumentHighlight{Range: loc.Range, Kind: highlightKindForRange(loc.Range, sym.Range, wordRange)})
			}
			for _, loc := range findAccessLocations(uri, line, i, "->"+bareName, 2, bareName) {
				highlights = append(highlights, protocol.DocumentHighlight{Range: loc.Range, Kind: protocol.DocumentHighlightKindRead})
			}
		}
	}

	return deduplicateHighlights(highlights)
}

func (a *Analyzer) GetSignatureHelp(uri, source string, pos protocol.Position) *protocol.SignatureHelp {
	line := resolve.GetLineAt(source, pos.Line)
	if line == "" {
		return nil
	}
	prefix := line[:min(pos.Character, len(line))]
	funcName, activeParam := extractCallInfo(prefix)
	if funcName == "" {
		return nil
	}
	syms := a.index.LookupByName(funcName)
	if len(syms) == 0 {
		return nil
	}
	sym := symbols.PickBestStandalone(syms, funcName)
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

func extractCallInfo(prefix string) (string, int) {
	activeParam := 0
	depth := 0
	parenPos := -1
	for i := len(prefix) - 1; i >= 0; i-- {
		switch prefix[i] {
		case ')':
			depth++
		case '(':
			if depth == 0 {
				parenPos = i
				goto found
			}
			depth--
		case ',':
			if depth == 0 {
				activeParam++
			}
		}
	}
	return "", 0
found:
	if parenPos <= 0 {
		return "", 0
	}
	end := parenPos
	start := end - 1
	for start >= 0 && resolve.IsWordChar(prefix[start]) {
		start--
	}
	start++
	if start >= end {
		return "", 0
	}
	return prefix[start:end], activeParam
}

func formatParamLabel(sym *symbols.Symbol) string {
	var ps []string
	for _, p := range sym.Params {
		s := ""
		if p.Type != "" {
			s = p.Type + " "
		}
		s += p.Name
		ps = append(ps, s)
	}
	ret := sym.ReturnType
	if ret == "" {
		ret = "mixed"
	}
	return "(" + strings.Join(ps, ", ") + "): " + ret
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
	ctx := sourcectx.Analyze(uri, source, pos)
	if ctx == nil {
		return nil
	}
	word, wordRange := ctx.SymbolText, ctx.WordRange
	if word == "" || word == "$this" {
		return nil
	}

	file := ctx.File

	// Try resolving as a symbol (class, method, property, function, etc.)
	sym := a.resolveSymbolAtCursor(uri, source, pos, word, file)

	// Reject built-ins
	if sym != nil && sym.Source == symbols.SourceBuiltin {
		return nil
	}

	// Reject vendor symbols (renaming dependencies is not useful)
	if sym != nil && sym.Source == symbols.SourceVendor {
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

	// Check for member access context (method/property on -> or ::)
	if ctx.AccessKind != sourcectx.AccessNone {
		return &protocol.PrepareRenameResult{Range: wordRange, Placeholder: word}
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
	ctx := sourcectx.Analyze(uri, source, pos)
	if ctx == nil {
		return nil
	}
	word := ctx.SymbolText
	if word == "" || word == "$this" {
		return nil
	}

	file := ctx.File

	// Try resolving as a symbol first (handles properties, methods, classes, etc.)
	sym := a.resolveSymbolAtCursor(uri, source, pos, word, file)

	// If it's a known symbol, rename it across the workspace
	if sym != nil {
		if sym.Source == symbols.SourceBuiltin || sym.Source == symbols.SourceVendor {
			return nil
		}
		return a.renameSymbol(sym, newName, readDocument)
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
func (a *Analyzer) renameSymbol(sym *symbols.Symbol, newName string, readDocument func(string) string) *protocol.WorkspaceEdit {
	locs := a.findSymbolOccurrences(sym, readDocument)
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

	file := parser.ParseFile(source)
	if file == nil {
		return actions
	}

	actions = append(actions, a.unknownClassCodeActions(uri, source, file, params)...)
	actions = append(actions, a.importUnusedImportQuickFixes(uri, source, params)...)
	if action := a.generateConstructorCodeAction(uri, source, parseResult, params.Context.Only, params.Range.Start); action != nil {
		actions = append(actions, *action)
	}
	actions = append(actions, a.generateGetterSetterCodeActions(uri, source, parseResult, params.Context.Only, params.Range.Start)...)
	actions = append(actions, a.implementMissingMethodsCodeActions(uri, source, parseResult, params.Context.Only, params.Range.Start)...)
	if action := a.extractVariableCodeAction(uri, source, params); action != nil {
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
		if importCodeActionKindAllowed(params.Context.Only, "source") {
			actions = append(actions, protocol.CodeAction{
				Title:   "Copy Namespace: " + fqn,
				Kind:    "source",
				Command: &protocol.Command{Title: "Copy Namespace", Command: "tuskPhpLsp.copyNamespace", Arguments: []json.RawMessage{uriJSON}},
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
			if importCodeActionKindAllowed(params.Context.Only, "refactor.move") {
				actions = append(actions, protocol.CodeAction{
					Title: "Move to namespace...",
					Kind:  "refactor.move",
					Command: &protocol.Command{
						Title:     "Move to namespace",
						Command:   "tuskPhpLsp.moveToNamespace",
						Arguments: []json.RawMessage{uriJSON},
					},
				})
			}
		}
	}

	return actions
}

// GetFileNamespace returns the FQN of the primary class/interface/enum in a file,
// or the namespace if no named type exists.
func (a *Analyzer) GetFileNamespace(uri, source string) string {
	file := parser.ParseFile(source)
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
	file := parser.ParseFile(source)
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

		fileFile := parser.ParseFile(fileSource)
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
