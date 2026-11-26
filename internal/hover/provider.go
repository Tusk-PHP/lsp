package hover

import (
	"fmt"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/phparray"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

type Provider struct {
	index         *symbols.Index
	container     *container.ContainerAnalyzer
	engine        *resolve.Engine
	framework     string
	arrayResolver *models.FrameworkArrayResolver
	cfg           *config.Config
	coverage      *coverage.Store
}

func NewProvider(index *symbols.Index, ca *container.ContainerAnalyzer, framework string) *Provider {
	p := &Provider{index: index, container: ca, engine: resolve.NewEngine(index), framework: framework}
	p.engine.ChainResolver = resolveExpressionType
	return p
}

// SetCoverage wires the workspace's test coverage reports into hover cards.
func (p *Provider) SetCoverage(store *coverage.Store) {
	p.coverage = store
}

// SetTypedChainResolver wires the generic-aware chain resolver for variable
// type inference (so $categories = Category::get() shows Collection<int, Category>).
func (p *Provider) SetTypedChainResolver(fn func(expr string, source string, pos protocol.Position, file *parser.FileNode) resolve.ResolvedType) {
	p.engine.DelegateTypedChains(fn)
}

// resolveExpressionType gives the class a chain expression like
// "Category::first()" ends with.
func resolveExpressionType(r *resolve.Resolver, expr string, source string, pos protocol.Position, file *parser.FileNode) string {
	return r.ChainType(expr, source, pos, file).BaseFQN()
}

// SetArrayResolver sets the framework array resolver for config hover.
func (p *Provider) SetArrayResolver(resolver *models.FrameworkArrayResolver) {
	p.arrayResolver = resolver
}

// SetConfig stores the server config so hover can read live settings (e.g.
// PHPManualLocale) at request time rather than at construction time, which
// allows config reloads to take effect without rebuilding the provider.
func (p *Provider) SetConfig(cfg *config.Config) {
	p.cfg = cfg
}

func (p *Provider) GetHover(uri, source string, pos protocol.Position) *protocol.Hover {
	ctx := sourcectx.Analyze(uri, source, pos)
	if ctx == nil {
		return nil
	}
	r := p.engine.Resolver()
	lines := strings.Split(source, "\n")
	line := ctx.Line

	file := ctx.File

	// Check for array key hover: $config['key'] or $config['db']['host'] — cursor on a key
	if ctx, ok := getArrayKeyContext(line, pos.Character); ok {
		return p.hoverArrayKey(source, pos, ctx, file)
	}

	// Check for config key hover: config('database.connections.mysql')
	if hover := p.hoverConfigKey(line, pos.Character); hover != nil {
		return hover
	}

	if isInStringLiteral(line, pos.Character) {
		return nil
	}

	word := ctx.SymbolText
	if word == "" {
		return nil
	}

	// No hover card for PHP primitive types (except self/static/parent which resolve to classes)
	if symbols.IsPHPBuiltinType(word) && word != "self" && word != "static" && word != "parent" {
		return nil
	}

	// Handle $variable hover. After `::` a $name is a static property
	// (self::$cache, static::$count), handled with the other members below.
	if strings.HasPrefix(word, "$") && ctx.AccessKind != sourcectx.AccessStatic {
		return p.hoverVariable(r, lines, pos, file, word)
	}

	// Handle self/static/parent keywords — resolve to enclosing class. They
	// are keywords, so any letter case counts.
	switch strings.ToLower(word) {
	case "self", "static", "parent":
		word = strings.ToLower(word)
	}
	if word == "self" || word == "static" || word == "parent" {
		if file != nil {
			var classFQN string
			if word == "parent" {
				enclosing := ctx.EnclosingFQN
				if enclosing != "" {
					chain := p.index.GetInheritanceChain(enclosing)
					if len(chain) > 0 {
						classFQN = chain[0]
					}
				}
			} else {
				classFQN = ctx.EnclosingFQN
			}
			if classFQN != "" {
				if sym := p.index.Lookup(classFQN); sym != nil {
					content := p.formatHover(sym)
					if content != "" {
						return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
					}
				}
			}
		}
	}

	// Check for -> or :: access context
	if ctx.AccessKind != sourcectx.AccessNone {
		owner := r.AccessOwnerType(ctx.JoinedLine, ctx.JoinedWordStart, source, pos, file)
		classFQN := owner.BaseFQN()
		if classFQN == "" {
			return nil
		}
		sym := r.AccessedMemberOn(owner, word, ctx.MemberAccess())
		// For Laravel facades, also look up the member on the concrete class
		if sym == nil && p.container != nil && p.framework == "laravel" {
			if concrete := p.container.ResolveFacade(classFQN); concrete != "" {
				sym = r.FindMember(concrete, word)
			}
		}
		if sym != nil {
			content := p.formatHover(sym)

			// Enhance with generic return type if available
			if sym.Kind == symbols.KindMethod {
				// Build the full expression up to and including the method call
				hoverPrefix := ctx.JoinedLine[:ctx.JoinedWordStart]
				trimmedPrefix := strings.TrimSpace(hoverPrefix)
				if strings.HasSuffix(trimmedPrefix, "->") || strings.HasSuffix(trimmedPrefix, "::") {
					expr := trimmedPrefix + word + "()"
					rt := r.ChainType(expr, source, pos, file)
					if rt.IsGeneric() || (rt.Nullable && rt.FQN != "") {
						content = replaceReturnType(content, rt.String())
					}
				}
			}

			if content != "" {
				return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
			}
		}
		// Member not found on the resolved class — don't fall through to
		// standalone lookups which would show unrelated symbols with the same name.
		return nil
	}

	if ctx.FunctionCall {
		fn := r.CalledFunction(word, file)
		if fn == nil {
			return nil
		}
		return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: p.formatHover(fn)}}
	}

	if ctx.Declared {
		declared := p.index.Lookup(declaredFQN(ctx))
		if declared == nil {
			return nil
		}
		content := p.formatHover(declared)
		if content == "" {
			return nil
		}
		return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
	}

	// Resolve the word via use statements
	if file != nil {
		for _, u := range ctx.Uses {
			if u.Alias == word {
				if sym := p.index.Lookup(u.FullName); sym != nil {
					content := p.formatHover(sym)
					if content != "" {
						return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
					}
				}
			}
		}
		// Try resolving as a class name in the current namespace context
		fqn := r.ResolveClassName(word, file)
		if fqn != word {
			if sym := p.index.Lookup(fqn); sym != nil {
				content := p.formatHover(sym)
				if content != "" {
					return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
				}
			}
		}
	}

	// If the word contains backslashes (FQN like Monolog\Logger), try direct FQN lookup
	if strings.Contains(word, "\\") {
		if sym := p.index.Lookup(word); sym != nil {
			content := p.formatHover(sym)
			if content != "" {
				return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
			}
		}
	}

	best := p.index.LookupStandalone(word, ctx.EnclosingFQN)
	if best == nil {
		return nil
	}
	content := p.formatHover(best)
	if content == "" {
		return nil
	}
	return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: content}}
}

func declaredFQN(ctx *sourcectx.CursorContext) string {
	switch {
	case ctx.EnclosingFQN != "":
		return ctx.EnclosingFQN + "::" + ctx.SymbolText
	case ctx.Namespace != "":
		return ctx.Namespace + "\\" + ctx.SymbolText
	}
	return ctx.SymbolText
}

func isInStringLiteral(line string, character int) bool {
	if character < 0 || character > len(line) {
		return false
	}

	inQuote := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inQuote != 0 {
			if i == character {
				return true
			}
			if ch == '\\' && i+1 < len(line) {
				i++
				if i == character {
					return true
				}
				continue
			}
			if ch == inQuote {
				if character == i {
					return true
				}
				inQuote = 0
			}
			continue
		}
		if ch == '\'' || ch == '"' {
			if character == i {
				return true
			}
			inQuote = ch
		}
	}

	return inQuote != 0 && character == len(line)
}

func (p *Provider) hoverVariable(r *resolve.Resolver, lines []string, pos protocol.Position, file *parser.FileNode, varName string) *protocol.Hover {
	if file == nil {
		return nil
	}

	// Try typed resolution first (preserves generics: array<int, Category>)
	resolvedType := r.ResolveVariableTypeTyped(varName, lines, pos, file)
	typeName := resolvedType.BaseFQN()
	typeDisplay := resolvedType.FullString()
	if typeDisplay == "" {
		// Fall back to non-typed resolution
		typeName = r.ResolveVariableType(varName, lines, pos, file)
		typeDisplay = typeName
	}
	if typeName != "" {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**%s**\n", varName))
		if summary := resolve.ParamDocSummary(file, pos, varName); summary != "" {
			sb.WriteString("\n" + summary + "\n")
		} else if sym := p.index.Lookup(typeName); sym != nil {
			if sym.DocComment != "" {
				if doc := parser.ParseDocBlock(sym.DocComment); doc != nil && doc.Summary != "" {
					sb.WriteString("\n" + doc.Summary + "\n")
				}
			}
		}
		sb.WriteString("\n" + phpFence(shortTypes(typeDisplay)+" "+varName))
		p.appendContainerBinding(&sb, typeName)
		return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: sb.String()}}
	}

	// A variable that exists but whose type can't be worked out.
	if r.IsLocalVariable(varName, lines, pos) {
		var sb strings.Builder
		sb.WriteString(fmt.Sprintf("**%s**\n", varName))
		sb.WriteString("\n" + phpFence("mixed "+varName))
		return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: sb.String()}}
	}
	return nil
}

func (p *Provider) formatHover(sym *symbols.Symbol) string {
	var sb strings.Builder

	// === 1. Header: bold FQN ===
	sb.WriteString("**" + sym.FQN + "**\n")

	doc := p.getEffectiveDocBlock(sym)

	// === 2. Code block: PHP declaration ===
	sb.WriteString("\n" + phpFence(p.formatHoverDeclaration(sym, doc)))

	// === 3. Summary (from docblock) ===
	if doc != nil && doc.Summary != "" {
		sb.WriteString("\n" + doc.Summary + "\n")
	}

	// === 4. Context line (parent class, override info) ===
	sb.WriteString(p.formatHoverContext(sym))

	// === 5. Extended docblock details ===
	sb.WriteString(p.formatDocBlockDetails(doc))

	// === 6. Test coverage ===
	sb.WriteString(p.formatCoverage(sym))

	// === 7. Container binding ===
	switch sym.Kind {
	case symbols.KindInterface, symbols.KindClass:
		p.appendContainerBinding(&sb, sym.FQN)
	}

	// === 8. PHP Manual link ===
	// Owner is the *declaring* class — that's how php.net documents methods,
	// properties, and class constants. Inherited members link to the parent
	// (e.g. ReflectionMethod::getParameters → reflectionfunctionabstract.*).
	var manualOwner *symbols.Symbol
	if sym.ParentFQN != "" {
		manualOwner = p.index.Lookup(sym.ParentFQN)
	}
	locale := ""
	if p.cfg != nil {
		locale = p.cfg.PHPManualLocale
	}
	if url := symbols.PHPManualURL(sym, manualOwner, locale); url != "" {
		sb.WriteString(fmt.Sprintf("\n[PHP Manual](%s)\n", url))
	}

	return sb.String()
}

// appendContainerBinding adds container binding info if available.
func (p *Provider) appendContainerBinding(sb *strings.Builder, fqn string) {
	if p.container == nil {
		return
	}
	if binding := p.container.ResolveDependency(fqn); binding != nil {
		sb.WriteString(fmt.Sprintf("\n---\n**Container Binding**\n- Concrete: `%s`\n- Singleton: %v\n", binding.Concrete, binding.Singleton))
	}
}

// appendMethodOrigin detects if a method overrides a parent method or implements an interface method.
func (p *Provider) appendMethodOrigin(sb *strings.Builder, sym *symbols.Symbol) {
	// Check interfaces
	ifaces := p.index.GetImplementedInterfaces(sym.ParentFQN)
	for _, ifaceFQN := range ifaces {
		ifaceSym := p.index.Lookup(ifaceFQN)
		if ifaceSym == nil {
			continue
		}
		for _, child := range ifaceSym.Children {
			if child.Kind == symbols.KindMethod && child.Name == sym.Name {
				sb.WriteString(fmt.Sprintf("\nImplements `%s::%s`\n", ifaceFQN, sym.Name))
				return
			}
		}
	}
	// Check parent chain
	chain := p.index.GetInheritanceChain(sym.ParentFQN)
	for _, parentFQN := range chain {
		parentSym := p.index.Lookup(parentFQN)
		if parentSym == nil {
			continue
		}
		for _, child := range parentSym.Children {
			if child.Kind == symbols.KindMethod && child.Name == sym.Name {
				sb.WriteString(fmt.Sprintf("\nOverrides `%s::%s`\n", parentFQN, sym.Name))
				return
			}
		}
	}
	// No origin to report: the header already names the declaring class.
}

// getEffectiveDocBlock returns the docblock for a symbol, falling back to parent/interface docs.
func (p *Provider) getEffectiveDocBlock(sym *symbols.Symbol) *parser.DocBlock {
	if sym.DocComment != "" {
		if doc := parser.ParseDocBlock(sym.DocComment); doc != nil {
			return doc
		}
	}
	// For methods, try inheriting from parent or interface
	if sym.Kind == symbols.KindMethod && sym.ParentFQN != "" {
		// Check interfaces
		ifaces := p.index.GetImplementedInterfaces(sym.ParentFQN)
		for _, ifaceFQN := range ifaces {
			ifaceSym := p.index.Lookup(ifaceFQN)
			if ifaceSym == nil {
				continue
			}
			for _, child := range ifaceSym.Children {
				if child.Kind == symbols.KindMethod && child.Name == sym.Name && child.DocComment != "" {
					if doc := parser.ParseDocBlock(child.DocComment); doc != nil {
						return doc
					}
				}
			}
		}
		// Check parent chain
		chain := p.index.GetInheritanceChain(sym.ParentFQN)
		for _, parentFQN := range chain {
			parentSym := p.index.Lookup(parentFQN)
			if parentSym == nil {
				continue
			}
			for _, child := range parentSym.Children {
				if child.Kind == symbols.KindMethod && child.Name == sym.Name && child.DocComment != "" {
					if doc := parser.ParseDocBlock(child.DocComment); doc != nil {
						return doc
					}
				}
			}
		}
	}
	return nil
}

// hoverArrayKeyContext holds the parsed context for hovering an array key.
type hoverArrayKeyContext struct {
	VarName    string   // e.g. "$config"
	AccessKeys []string // preceding completed keys, e.g. ["database"] for $config['database']['host']
	CurrentKey string   // the key the cursor is on, e.g. "host"
}

// getArrayKeyContext checks if the cursor is on a string key inside an array access
// expression like $config['key'] or $config['db']['host'].
func getArrayKeyContext(line string, character int) (*hoverArrayKeyContext, bool) {
	if character >= len(line) || len(line) < 4 {
		return nil, false
	}

	// Step 1: find the string literal boundaries around the cursor
	left := character
	for left > 0 && line[left] != '\'' && line[left] != '"' {
		if line[left] == ']' || line[left] == '[' {
			return nil, false
		}
		left--
	}
	if left <= 0 || (line[left] != '\'' && line[left] != '"') {
		return nil, false
	}
	openQuote := line[left]

	right := character
	if right < len(line) && line[right] == openQuote {
		right++
	} else {
		for right < len(line) && line[right] != openQuote {
			right++
		}
		if right >= len(line) {
			return nil, false
		}
		right++
	}

	if left+1 >= right-1 {
		return nil, false
	}
	currentKey := line[left+1 : right-1]
	if currentKey == "" {
		return nil, false
	}

	// Step 2: expect [ before the opening quote
	i := left - 1
	for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
		i--
	}
	if i < 0 || line[i] != '[' {
		return nil, false
	}
	i--

	// Step 3: collect preceding completed access keys backward: ]['key2']['key1']
	var accessKeys []string
	for i >= 0 {
		for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
			i--
		}
		if i < 0 || line[i] != ']' {
			break
		}
		i-- // skip ]

		if i < 0 || (line[i] != '\'' && line[i] != '"') {
			break
		}
		closeQ := line[i]
		i--

		keyEnd := i + 1
		for i >= 0 && line[i] != closeQ {
			i--
		}
		if i < 0 {
			break
		}
		key := line[i+1 : keyEnd]
		i-- // skip opening quote

		for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
			i--
		}
		if i < 0 || line[i] != '[' {
			break
		}
		i-- // skip [
		accessKeys = append([]string{key}, accessKeys...)
	}

	// Step 4: skip whitespace and extract $variable
	for i >= 0 && (line[i] == ' ' || line[i] == '\t') {
		i--
	}
	if i < 0 {
		return nil, false
	}

	end := i + 1
	for i >= 0 && resolve.IsWordChar(line[i]) {
		i--
	}
	if i >= 0 && line[i] == '$' {
		return &hoverArrayKeyContext{
			VarName:    line[i:end],
			AccessKeys: accessKeys,
			CurrentKey: currentKey,
		}, true
	}
	return nil, false
}

// getArrayKeyAt is a convenience wrapper for backward compatibility in tests.
func getArrayKeyAt(line string, character int) (varName, key string, ok bool) {
	ctx, found := getArrayKeyContext(line, character)
	if !found {
		return "", "", false
	}
	return ctx.VarName, ctx.CurrentKey, true
}

// hoverArrayKey provides hover information for an array key, including nested access.
func (p *Provider) hoverArrayKey(source string, pos protocol.Position, ctx *hoverArrayKeyContext, file *parser.FileNode) *protocol.Hover {
	// Resolve top-level shape
	fields := p.resolveArrayShape(source, pos, ctx.VarName, file)
	if len(fields) == 0 {
		fields = scanArrayKeysForHover(source, pos, ctx.VarName)
	}

	// Drill into nested shapes via accessKeys
	for _, accessKey := range ctx.AccessKeys {
		var nestedType string
		for _, f := range fields {
			if f.Key == accessKey {
				nestedType = f.Type
				break
			}
		}
		if nestedType == "" {
			return nil
		}
		fields = types.ParseArrayShape(nestedType)
		if len(fields) == 0 {
			return nil
		}
	}

	// Find the current key in the (possibly nested) shape
	for _, f := range fields {
		if f.Key == ctx.CurrentKey {
			var sb strings.Builder
			sb.WriteString("```php\n")
			if f.Type != "" {
				sb.WriteString(fmt.Sprintf("(array key) %s $%s", f.Type, ctx.CurrentKey))
			} else {
				sb.WriteString(fmt.Sprintf("(array key) $%s", ctx.CurrentKey))
			}
			if f.Optional {
				sb.WriteString(" (optional)")
			}
			sb.WriteString("\n```")
			return &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown", Value: sb.String()}}
		}
	}

	return nil
}

// hoverConfigKey provides hover for config key strings inside config('key.path').
func (p *Provider) hoverConfigKey(line string, character int) *protocol.Hover {
	if p.arrayResolver == nil {
		return nil
	}

	// Find config( before the cursor on this line
	configIdx := strings.LastIndex(line[:min(character+1, len(line))], "config(")
	if configIdx < 0 {
		return nil
	}
	after := line[configIdx+len("config("):]

	// Must have an opening quote
	if len(after) == 0 || (after[0] != '\'' && after[0] != '"') {
		return nil
	}
	openQuote := after[0]
	after = after[1:]

	// Find closing quote
	closeIdx := strings.IndexByte(after, openQuote)
	if closeIdx < 0 {
		return nil
	}

	// Check cursor is inside the string
	stringStart := configIdx + len("config(") + 1 // after open quote
	stringEnd := stringStart + closeIdx
	if character < stringStart || character > stringEnd {
		return nil
	}

	fullKey := after[:closeIdx]
	if fullKey == "" {
		return nil
	}

	// Resolve the config value at this path
	parts := strings.Split(fullKey, ".")
	configFile := parts[0]
	keys := p.arrayResolver.ParseConfigFile(configFile)
	if keys == nil {
		return nil
	}

	// For top-level key only (e.g. config('database')), show the file's shape
	if len(parts) == 1 {
		var keyNames []string
		for _, k := range keys {
			if k.Key != "" {
				keyNames = append(keyNames, k.Key)
			}
			if len(keyNames) >= 6 {
				keyNames = append(keyNames, "...")
				break
			}
		}
		return &protocol.Hover{Contents: protocol.MarkupContent{
			Kind:  "markdown",
			Value: fmt.Sprintf("```php\n(config) %s: array{%s}\n```", fullKey, strings.Join(keyNames, ", ")),
		}}
	}

	// Drill through dot segments to find the target
	var targetField *types.ShapeField
	for _, segment := range parts[1:] {
		found := false
		for i := range keys {
			if keys[i].Key == segment {
				targetField = &keys[i]
				found = true
				break
			}
		}
		if !found {
			return nil
		}
		// If there are more segments, drill into the nested shape
		nested := types.ParseArrayShape(targetField.Type)
		if nested != nil {
			keys = nested
		}
	}

	if targetField == nil {
		return nil
	}

	// Show the resolved type
	detail := targetField.Type
	if strings.HasPrefix(detail, "array{") {
		// Summarize nested keys
		inner := types.ParseArrayShape(detail)
		if len(inner) > 0 {
			var names []string
			for _, f := range inner {
				if f.Key != "" {
					names = append(names, f.Key)
				}
				if len(names) >= 6 {
					names = append(names, "...")
					break
				}
			}
			detail = "array{" + strings.Join(names, ", ") + "}"
		}
	}

	return &protocol.Hover{Contents: protocol.MarkupContent{
		Kind:  "markdown",
		Value: fmt.Sprintf("```php\n(config) %s: %s\n```", fullKey, detail),
	}}
}

// resolveArrayShape resolves shape fields for a variable from docblock annotations.
func (p *Provider) resolveArrayShape(source string, pos protocol.Position, varName string, file *parser.FileNode) []types.ShapeField {
	if file == nil {
		return nil
	}
	bare := strings.TrimPrefix(varName, "$")

	// Check method parameters
	for _, cls := range file.Classes {
		for _, m := range cls.Methods {
			if pos.Line >= m.StartLine {
				if m.DocComment != "" {
					doc := parser.ParseDocBlock(m.DocComment)
					if doc != nil {
						for _, param := range doc.Params {
							if param.Name == "$"+bare {
								if fields := types.ParseArrayShape(param.Type); len(fields) > 0 {
									return fields
								}
							}
						}
					}
				}
				for _, param := range m.Params {
					if param.Name == varName {
						if fields := types.ParseArrayShape(param.Type.Name); len(fields) > 0 {
							return fields
						}
					}
				}
			}
		}
	}
	for _, fn := range file.Functions {
		if pos.Line >= fn.StartLine {
			if fn.DocComment != "" {
				doc := parser.ParseDocBlock(fn.DocComment)
				if doc != nil {
					for _, param := range doc.Params {
						if param.Name == "$"+bare {
							if fields := types.ParseArrayShape(param.Type); len(fields) > 0 {
								return fields
							}
						}
					}
				}
			}
		}
	}

	// Check @var annotations
	lines := strings.Split(source, "\n")
	for i := pos.Line; i >= 0 && i >= pos.Line-10; i-- {
		if i >= len(lines) {
			continue
		}
		line := strings.TrimSpace(lines[i])
		if strings.Contains(line, "@var") && strings.Contains(line, varName) {
			varIdx := strings.Index(line, "@var ")
			if varIdx >= 0 {
				rest := strings.TrimSpace(line[varIdx+5:])
				typeStr, _ := types.ExtractDocTypeString(rest)
				if fields := types.ParseArrayShape(typeStr); len(fields) > 0 {
					return fields
				}
			}
		}
	}

	return nil
}

// scanArrayKeysForHover extracts keys from literal array assignments,
// preserving nested structure for drilling.
func scanArrayKeysForHover(source string, pos protocol.Position, varName string) []types.ShapeField {
	lines := strings.Split(source, "\n")
	var keys []types.ShapeField
	seen := make(map[string]bool)

	for i := pos.Line; i >= 0 && i >= pos.Line-200; i-- {
		if i >= len(lines) {
			continue
		}
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, varName) {
			rest := strings.TrimSpace(trimmed[len(varName):])
			if strings.HasPrefix(rest, "=") {
				rhs := strings.TrimSpace(rest[1:])
				if strings.HasPrefix(rhs, "[") || strings.HasPrefix(strings.ToLower(rhs), "array(") {
					arrayText := phparray.CollectArrayLiteral(lines, i)
					return phparray.ParseLiteralToShape(arrayText)
				}
			}
		}
		// Incremental: $var['key'] = ... (must have = after ])
		if strings.HasPrefix(trimmed, varName+"[") {
			after := trimmed[len(varName)+1:]
			if len(after) > 2 && (after[0] == '\'' || after[0] == '"') {
				q := after[0]
				endQ := strings.IndexByte(after[1:], q)
				if endQ > 0 {
					afterClose := strings.TrimSpace(after[endQ+2:])
					if !strings.HasPrefix(afterClose, "]") {
						continue
					}
					afterBracket := strings.TrimSpace(afterClose[1:])
					if !strings.HasPrefix(afterBracket, "=") {
						continue
					}
					key := after[1 : endQ+1]
					if !seen[key] {
						seen[key] = true
						keys = append(keys, types.ShapeField{Key: key})
					}
				}
			}
		}
	}
	return keys
}
