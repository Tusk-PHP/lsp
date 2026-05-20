package scope

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
)

type ScopeKind string

const (
	ScopeFile     ScopeKind = "file"
	ScopeFunction ScopeKind = "function"
	ScopeMethod   ScopeKind = "method"
	ScopeClosure  ScopeKind = "closure"
)

type BindingKind string

const (
	BindingVariable     BindingKind = "variable"
	BindingParameter    BindingKind = "parameter"
	BindingForeachKey   BindingKind = "foreach-key"
	BindingForeachValue BindingKind = "foreach-value"
	BindingCatch        BindingKind = "catch"
	BindingDestructure  BindingKind = "destructure"
	BindingImport       BindingKind = "import"
	BindingClosureUse   BindingKind = "closure-use"
)

type Scope struct {
	Kind     ScopeKind
	Name     string
	Range    protocol.Range
	Parent   *Scope
	Bindings []*Binding
}

type Binding struct {
	Name       string
	Kind       BindingKind
	Decl       protocol.Range
	Scope      *Scope
	Origin     *Binding
	References []protocol.Range
}

type Document struct {
	Scopes   []*Scope
	Bindings []*Binding
	root     *Scope
}

type Collector struct{}

func NewCollector() *Collector {
	return &Collector{}
}

func Collect(source string) *Document {
	return NewCollector().Collect(source)
}

func (c *Collector) Collect(source string) *Document {
	result := parser.New().Parse(source)
	return c.collectResult(result)
}

type scopeFrame struct {
	scope      *Scope
	braceDepth int
}

type pendingScope struct {
	scope         *Scope
	bodyOpenIndex int
}

func (c *Collector) collectResult(result *parser.ParseResult) *Document {
	doc := &Document{}
	lineCount := 0
	if result != nil {
		lineCount = len(result.Lines)
	}
	root := &Scope{
		Kind:  ScopeFile,
		Range: protocol.Range{Start: protocol.Position{}, End: protocol.Position{Line: max(lineCount-1, 0), Character: lineLength(result, lineCount-1)}},
	}
	doc.root = root
	doc.Scopes = append(doc.Scopes, root)

	if result == nil {
		return doc
	}

	for _, useStmt := range result.Uses {
		rng, ok := findUseAliasRange(result.Tokens, useStmt)
		if !ok {
			continue
		}
		doc.addBinding(root, useStmt.Alias, BindingImport, rng, nil)
	}

	active := []scopeFrame{{scope: root, braceDepth: 0}}
	var pending []pendingScope
	declByToken := map[int]*Binding{}
	braceDepth := 0

	for i := 0; i < len(result.Tokens); i++ {
		tok := result.Tokens[i]

		if tok.Kind == parser.TokenFunction {
			if ps, decls := parseFunctionLike(result.Tokens, i, active[len(active)-1].scope, doc, declByToken); ps != nil {
				pending = append(pending, *ps)
				for idx, binding := range decls {
					declByToken[idx] = binding
				}
			}
		}

		if tok.Kind == parser.TokenIdentifier && tok.Value == "foreach" {
			for idx, binding := range parseForeachBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

		if tok.Kind == parser.TokenIdentifier && tok.Value == "catch" {
			for idx, binding := range parseCatchBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

		if tok.Kind == parser.TokenOpenBracket {
			for idx, binding := range parseDestructureBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

		if tok.Kind == parser.TokenVariable {
			if _, ok := declByToken[i]; !ok {
				if next := nextSignificant(result.Tokens, i+1); next >= 0 && result.Tokens[next].Kind == parser.TokenEquals {
					binding := ensureBinding(doc, active[len(active)-1].scope, tok.Value, BindingVariable, tokenRange(tok))
					declByToken[i] = binding
				}
			}
		}

		if tok.Kind == parser.TokenOpenBrace {
			braceDepth++
			for len(pending) > 0 && pending[0].bodyOpenIndex == i {
				active = append(active, scopeFrame{scope: pending[0].scope, braceDepth: braceDepth})
				pending = pending[1:]
			}
			continue
		}

		if tok.Kind == parser.TokenVariable {
			if _, ok := declByToken[i]; ok {
				continue
			}
			if binding := resolveVariable(active, tok.Value); binding != nil {
				binding.References = append(binding.References, tokenRange(tok))
			}
			continue
		}

		if tok.Kind == parser.TokenCloseBrace {
			for len(active) > 1 && active[len(active)-1].braceDepth == braceDepth {
				active[len(active)-1].scope.Range.End = tokenRange(tok).End
				active = active[:len(active)-1]
			}
			if braceDepth > 0 {
				braceDepth--
			}
		}
	}

	for _, ps := range pending {
		if ps.scope.Range.End == (protocol.Position{}) {
			ps.scope.Range.End = root.Range.End
		}
	}

	return doc
}

func (d *Document) BindingAt(pos protocol.Position) *Binding {
	for _, binding := range d.Bindings {
		if containsPos(binding.Decl, pos) {
			return binding
		}
		for _, ref := range binding.References {
			if containsPos(ref, pos) {
				return binding
			}
		}
	}
	return nil
}

func (d *Document) Occurrences(binding *Binding) []protocol.Range {
	if binding == nil {
		return nil
	}
	root := binding.root()
	var ranges []protocol.Range
	for _, candidate := range d.Bindings {
		if candidate.root() != root {
			continue
		}
		ranges = append(ranges, candidate.Decl)
		ranges = append(ranges, candidate.References...)
	}
	return dedupeRanges(ranges)
}

func (b *Binding) root() *Binding {
	if b == nil {
		return nil
	}
	root := b
	for root.Origin != nil {
		root = root.Origin
	}
	return root
}

func ensureBinding(doc *Document, scope *Scope, name string, kind BindingKind, decl protocol.Range) *Binding {
	for _, binding := range scope.Bindings {
		if binding.Name == name {
			return binding
		}
	}
	return doc.addBinding(scope, name, kind, decl, nil)
}

func (d *Document) addBinding(scope *Scope, name string, kind BindingKind, decl protocol.Range, origin *Binding) *Binding {
	binding := &Binding{Name: name, Kind: kind, Decl: decl, Scope: scope, Origin: origin}
	scope.Bindings = append(scope.Bindings, binding)
	d.Bindings = append(d.Bindings, binding)
	return binding
}

func resolveVariable(active []scopeFrame, name string) *Binding {
	for i := len(active) - 1; i >= 0; i-- {
		scope := active[i].scope
		for j := len(scope.Bindings) - 1; j >= 0; j-- {
			binding := scope.Bindings[j]
			if binding.Name == name && binding.Kind != BindingImport {
				return binding
			}
		}
	}
	return nil
}

func parseFunctionLike(tokens []parser.Token, start int, parent *Scope, doc *Document, declByToken map[int]*Binding) (*pendingScope, map[int]*Binding) {
	name := ""
	kind := ScopeClosure
	nameIdx := nextSignificant(tokens, start+1)
	if nameIdx >= 0 && tokens[nameIdx].Kind == parser.TokenIdentifier {
		name = tokens[nameIdx].Value
		kind = ScopeFunction
		if enclosingClassName(tokens, start) != "" {
			kind = ScopeMethod
		}
		nameIdx = nextSignificant(tokens, nameIdx+1)
	}
	if nameIdx < 0 || tokens[nameIdx].Kind != parser.TokenOpenParen {
		return nil, nil
	}

	scope := &Scope{Kind: kind, Name: name, Parent: parent, Range: protocol.Range{Start: tokenRange(tokens[start]).Start}}
	doc.Scopes = append(doc.Scopes, scope)
	decls := map[int]*Binding{}

	closeParen := findMatching(tokens, nameIdx, parser.TokenOpenParen, parser.TokenCloseParen)
	if closeParen < 0 {
		return nil, decls
	}
	for i := nameIdx + 1; i < closeParen; i++ {
		if tokens[i].Kind != parser.TokenVariable {
			continue
		}
		binding := doc.addBinding(scope, tokens[i].Value, BindingParameter, tokenRange(tokens[i]), nil)
		decls[i] = binding
	}

	j := nextSignificant(tokens, closeParen+1)
	if kind == ScopeClosure && j >= 0 && tokens[j].Kind == parser.TokenUse {
		j = nextSignificant(tokens, j+1)
		if j >= 0 && tokens[j].Kind == parser.TokenOpenParen {
			useClose := findMatching(tokens, j, parser.TokenOpenParen, parser.TokenCloseParen)
			for i := j + 1; i < useClose; i++ {
				if tokens[i].Kind != parser.TokenVariable {
					continue
				}
				origin := resolveVariable([]scopeFrame{{scope: parent}}, tokens[i].Value)
				binding := doc.addBinding(scope, tokens[i].Value, BindingClosureUse, tokenRange(tokens[i]), origin)
				decls[i] = binding
			}
			j = nextSignificant(tokens, useClose+1)
		}
	}

	for j >= 0 && j < len(tokens) && tokens[j].Kind != parser.TokenOpenBrace && tokens[j].Kind != parser.TokenDoubleArrow && tokens[j].Kind != parser.TokenSemicolon {
		j++
		j = nextSignificant(tokens, j)
	}
	if j >= 0 && j < len(tokens) && tokens[j].Kind == parser.TokenOpenBrace {
		return &pendingScope{scope: scope, bodyOpenIndex: j}, decls
	}
	scope.Range.End = scope.Range.Start
	return nil, decls
}

func parseForeachBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := nextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return decls
	}
	close := findMatching(tokens, open, parser.TokenOpenParen, parser.TokenCloseParen)
	if close < 0 {
		return decls
	}

	depth := 1
	afterAs := false
	seenArrow := false
	var headerVars []int
	for i := open + 1; i < close; i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket:
			depth--
		case parser.TokenIdentifier:
			if depth == 1 && tokens[i].Value == "as" {
				afterAs = true
			}
		case parser.TokenDoubleArrow:
			if afterAs && depth == 1 {
				seenArrow = true
			}
		case parser.TokenVariable:
			if !afterAs {
				continue
			}
			if depth == 1 {
				headerVars = append(headerVars, i)
				continue
			}
			decls[i] = ensureBinding(doc, scope, tokens[i].Value, BindingDestructure, tokenRange(tokens[i]))
		}
	}
	for idx, tokIdx := range headerVars {
		kind := BindingForeachValue
		if seenArrow && idx == 0 {
			kind = BindingForeachKey
		}
		decls[tokIdx] = ensureBinding(doc, scope, tokens[tokIdx].Value, kind, tokenRange(tokens[tokIdx]))
	}
	return decls
}

func parseCatchBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := nextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return decls
	}
	close := findMatching(tokens, open, parser.TokenOpenParen, parser.TokenCloseParen)
	if close < 0 {
		return decls
	}
	for i := open + 1; i < close; i++ {
		if tokens[i].Kind == parser.TokenVariable {
			decls[i] = ensureBinding(doc, scope, tokens[i].Value, BindingCatch, tokenRange(tokens[i]))
		}
	}
	return decls
}

func parseDestructureBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	prev := prevSignificant(tokens, start-1)
	if prev >= 0 {
		switch tokens[prev].Kind {
		case parser.TokenEquals, parser.TokenDoubleArrow, parser.TokenArrow, parser.TokenDoubleColon:
			return decls
		}
	}
	close := findMatching(tokens, start, parser.TokenOpenBracket, parser.TokenCloseBracket)
	if close < 0 {
		return decls
	}
	next := nextSignificant(tokens, close+1)
	if next < 0 || tokens[next].Kind != parser.TokenEquals {
		return decls
	}
	for i := start + 1; i < close; i++ {
		if tokens[i].Kind == parser.TokenVariable {
			decls[i] = ensureBinding(doc, scope, tokens[i].Value, BindingDestructure, tokenRange(tokens[i]))
		}
	}
	return decls
}

func nextSignificant(tokens []parser.Token, i int) int {
	for i < len(tokens) {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i++
		default:
			return i
		}
	}
	return -1
}

func prevSignificant(tokens []parser.Token, i int) int {
	for i >= 0 {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i--
		default:
			return i
		}
	}
	return -1
}

func findMatching(tokens []parser.Token, start int, openKind, closeKind parser.TokenKind) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case openKind:
			depth++
		case closeKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func containsPos(r protocol.Range, pos protocol.Position) bool {
	if pos.Line < r.Start.Line || pos.Line > r.End.Line {
		return false
	}
	if pos.Line == r.Start.Line && pos.Character < r.Start.Character {
		return false
	}
	if pos.Line == r.End.Line && pos.Character > r.End.Character {
		return false
	}
	return true
}

func tokenRange(tok parser.Token) protocol.Range {
	return protocol.Range{
		Start: protocol.Position{Line: tok.Line, Character: tok.Column},
		End:   protocol.Position{Line: tok.Line, Character: tok.Column + len(tok.Value)},
	}
}

func lineLength(result *parser.ParseResult, idx int) int {
	if result == nil || idx < 0 || idx >= len(result.Lines) {
		return 0
	}
	return len(result.Lines[idx])
}

func findUseAliasRange(tokens []parser.Token, useStmt parser.UseStatement) (protocol.Range, bool) {
	for i := 0; i < len(tokens); i++ {
		if tokens[i].Kind != parser.TokenUse || tokens[i].Line != useStmt.Line {
			continue
		}
		aliasIdx := -1
		for j := i + 1; j < len(tokens) && tokens[j].Line == useStmt.Line; j++ {
			if tokens[j].Kind == parser.TokenIdentifier && tokens[j].Value == useStmt.Alias {
				aliasIdx = j
			}
		}
		if aliasIdx >= 0 {
			return tokenRange(tokens[aliasIdx]), true
		}
	}
	return protocol.Range{}, false
}

func enclosingClassName(tokens []parser.Token, idx int) string {
	depth := 0
	for i := idx; i >= 0; i-- {
		switch tokens[i].Kind {
		case parser.TokenCloseBrace:
			depth++
		case parser.TokenOpenBrace:
			if depth > 0 {
				depth--
			}
		case parser.TokenClass, parser.TokenTrait, parser.TokenInterface, parser.TokenEnum:
			if depth == 0 {
				return tokens[i].Value
			}
		}
	}
	return ""
}

func dedupeRanges(in []protocol.Range) []protocol.Range {
	type key struct {
		line int
		col  int
		end  int
	}
	seen := make(map[key]bool, len(in))
	var out []protocol.Range
	for _, rng := range in {
		k := key{line: rng.Start.Line, col: rng.Start.Character, end: rng.End.Character}
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, rng)
	}
	return out
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func ScopeNames(doc *Document) []string {
	var names []string
	for _, scope := range doc.Scopes {
		if scope.Name != "" {
			names = append(names, scope.Name)
		}
	}
	return names
}

func BindingNames(doc *Document) []string {
	var names []string
	for _, binding := range doc.Bindings {
		names = append(names, strings.TrimPrefix(binding.Name, "$"))
	}
	return names
}
