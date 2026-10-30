package scope

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
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
	// BindingImplicit is a variable first written by something other than an
	// assignment: `$a[] = 1`, or an argument the callee may write to.
	BindingImplicit BindingKind = "implicit"
)

type Scope struct {
	Kind     ScopeKind
	Name     string
	Range    protocol.Range
	ExprBody bool
	Parent   *Scope
	Bindings []*Binding
}

type Binding struct {
	Name        string
	Kind        BindingKind
	Decl        protocol.Range
	Scope       *Scope
	Origin      *Binding
	Assignments []*Assignment
	References  []protocol.Range
	Writes      []protocol.Range // later assignments; Decl is the first
}

type Assignment struct {
	Variable           protocol.Range
	Expression         protocol.Range
	Statement          protocol.Range
	RelativeBraceDepth int
	DirectStatement    bool
}

type Document struct {
	Scopes   []*Scope
	Bindings []*Binding
	// Properties lists `$name` tokens that are class properties rather than
	// variables: declarations, and static accesses such as `self::$name`.
	Properties []protocol.Range
	root       *Scope
}

type Collector struct {
	// OutArg, if set, reports whether a call may write to its arg'th argument
	// (0-based). callee is the function's name as written, or "" when it can't be
	// told from the source alone, such as for methods.
	OutArg func(callee string, arg int) bool
}

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
	exprEnd    int
}

type pendingScope struct {
	scope          *Scope
	bodyOpenIndex  int
	bodyCloseIndex int
	expressionBody bool
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

	active := []scopeFrame{{scope: root, braceDepth: 0, exprEnd: -1}}
	var pending []pendingScope
	declByToken := map[int]*Binding{}
	braceDepth := 0
	var classBodies []classBody
	pendingClassBody := -1

	popFinishedExpressions := func(i int) {
		for len(active) > 1 && active[len(active)-1].exprEnd == i {
			active[len(active)-1].scope.Range.End = tokenRange(result.Tokens[i]).End
			active = active[:len(active)-1]
		}
	}

	for idx, binding := range parseUseBindings(result.Tokens, doc, root) {
		declByToken[idx] = binding
	}

	for i := 0; i < len(result.Tokens); i++ {
		tok := result.Tokens[i]

		for len(pending) > 0 && pending[0].bodyOpenIndex == i && pending[0].expressionBody {
			active = append(active, scopeFrame{scope: pending[0].scope, braceDepth: braceDepth, exprEnd: pending[0].bodyCloseIndex})
			pending = pending[1:]
		}

		if isClassKeyword(result.Tokens, i) {
			pendingClassBody = classBodyOpen(result.Tokens, i)
		}

		inClassBody := len(classBodies) > 0 && classBodies[len(classBodies)-1].braceDepth == braceDepth && classBodies[len(classBodies)-1].activeLen == len(active)

		if !inClassBody && isGlobalOrStaticKeyword(tok) {
			for idx, binding := range parseGlobalOrStaticBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

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

		if tok.Kind == parser.TokenIdentifier && tok.Value == "list" {
			for idx, binding := range parseListDestructureBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

		if tok.Kind == parser.TokenOpenBracket {
			for idx, binding := range parseDestructureBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

		if tok.Kind == parser.TokenVariable {
			if _, isDecl := declByToken[i]; !isDecl && (inClassBody || isStaticPropertyAccess(result.Tokens, i)) {
				doc.Properties = append(doc.Properties, tokenRange(tok))
				popFinishedExpressions(i)
				continue
			}
		}

		if tok.Kind == parser.TokenVariable {
			_, isDecl := declByToken[i]
			if next := nextSignificant(result.Tokens, i+1); !isDecl && next >= 0 && result.Tokens[next].Kind == parser.TokenEquals {
				current := active[len(active)-1]
				binding := ensureBinding(doc, current.scope, tok.Value, BindingVariable, tokenRange(tok))
				declByToken[i] = binding
				if assignment := buildAssignment(result.Tokens, i, next, braceDepth-current.braceDepth); assignment != nil {
					binding.Assignments = append(binding.Assignments, assignment)
				}
			}
		}

		if tok.Kind == parser.TokenOpenBrace {
			braceDepth++
			for len(pending) > 0 && pending[0].bodyOpenIndex == i && !pending[0].expressionBody {
				active = append(active, scopeFrame{scope: pending[0].scope, braceDepth: braceDepth, exprEnd: -1})
				pending = pending[1:]
			}
			if i == pendingClassBody {
				classBodies = append(classBodies, classBody{braceDepth: braceDepth, activeLen: len(active)})
			}
			continue
		}

		if tok.Kind == parser.TokenStringLiteral {
			addInterpolatedReferences(tok, active)
		}

		if tok.Kind == parser.TokenVariable {
			if binding, ok := declByToken[i]; ok {
				if binding.Decl != tokenRange(tok) {
					binding.Writes = append(binding.Writes, tokenRange(tok))
				}
			} else if binding := resolveVariable(active, tok.Value); binding != nil {
				binding.References = append(binding.References, tokenRange(tok))
			} else if isSubscriptWrite(result.Tokens, i) || isOutArgument(result.Tokens, i, c.OutArg) {
				doc.addBinding(active[len(active)-1].scope, tok.Value, BindingImplicit, tokenRange(tok), nil)
			}
			popFinishedExpressions(i)
			continue
		}

		if tok.Kind == parser.TokenCloseBrace {
			for len(active) > 1 && active[len(active)-1].braceDepth == braceDepth {
				active[len(active)-1].scope.Range.End = tokenRange(tok).End
				active = active[:len(active)-1]
			}
			if len(classBodies) > 0 && classBodies[len(classBodies)-1].braceDepth == braceDepth {
				classBodies = classBodies[:len(classBodies)-1]
			}
			if braceDepth > 0 {
				braceDepth--
			}
		}

		popFinishedExpressions(i)
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
		for _, write := range binding.Writes {
			if containsPos(write, pos) {
				return binding
			}
		}
	}
	return nil
}

func (d *Document) ScopeAt(pos protocol.Position) *Scope {
	var best *Scope
	for _, scope := range d.Scopes {
		if !containsPos(scope.Range, pos) {
			continue
		}
		if best == nil || scopeContains(best.Range, scope.Range) {
			best = scope
		}
	}
	return best
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
		ranges = append(ranges, candidate.Writes...)
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
	if j >= 0 && j < len(tokens) && tokens[j].Kind == parser.TokenDoubleArrow {
		bodyStart := nextSignificant(tokens, j+1)
		bodyEnd := findArrowExpressionEnd(tokens, bodyStart)
		if bodyStart < 0 || bodyEnd < 0 {
			scope.Range.End = scope.Range.Start
			return nil, decls
		}
		scope.ExprBody = true
		scope.Range.End = tokenRange(tokens[bodyEnd]).End
		bindArrowCaptures(tokens, bodyStart, bodyEnd, parent, scope, doc, decls)
		return &pendingScope{
			scope:          scope,
			bodyOpenIndex:  bodyStart,
			bodyCloseIndex: bodyEnd,
			expressionBody: true,
		}, decls
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
		// A subscript, as in `$a[$key] = 1`, rather than a destructuring.
		case parser.TokenVariable, parser.TokenIdentifier, parser.TokenCloseBracket, parser.TokenCloseParen:
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

func parseListDestructureBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := nextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return decls
	}
	close := findMatching(tokens, open, parser.TokenOpenParen, parser.TokenCloseParen)
	if close < 0 {
		return decls
	}
	next := nextSignificant(tokens, close+1)
	if next < 0 || tokens[next].Kind != parser.TokenEquals {
		return decls
	}
	for i := open + 1; i < close; i++ {
		if tokens[i].Kind == parser.TokenVariable {
			decls[i] = ensureBinding(doc, scope, tokens[i].Value, BindingDestructure, tokenRange(tokens[i]))
		}
	}
	return decls
}

// classBody is a class-like's open body. A variable directly inside one, with no
// function opened since, is a property.
type classBody struct {
	braceDepth int
	activeLen  int
}

func isClassKeyword(tokens []parser.Token, i int) bool {
	switch tokens[i].Kind {
	case parser.TokenClass, parser.TokenInterface, parser.TokenTrait, parser.TokenEnum:
	default:
		return false
	}
	if prev := prevSignificant(tokens, i-1); prev >= 0 {
		switch tokens[prev].Kind {
		case parser.TokenDoubleColon, parser.TokenArrow, parser.TokenFunction:
			return false
		}
	}
	return true
}

// classBodyOpen returns the index of the brace opening the body of the class-like
// declared at start, or -1 if it's not a declaration. Only an anonymous class can
// have an argument list.
func classBodyOpen(tokens []parser.Token, start int) int {
	for i := nextSignificant(tokens, start+1); i >= 0 && i < len(tokens); i = nextSignificant(tokens, i+1) {
		switch tokens[i].Kind {
		case parser.TokenOpenBrace:
			return i
		case parser.TokenOpenParen:
			if tokens[start].Kind != parser.TokenClass {
				return -1
			}
			if i = findMatching(tokens, i, parser.TokenOpenParen, parser.TokenCloseParen); i < 0 {
				return -1
			}
		case parser.TokenSemicolon, parser.TokenCloseBrace, parser.TokenCloseParen, parser.TokenEquals:
			return -1
		}
	}
	return -1
}

// isStaticPropertyAccess matches `self::$name`, but not `self::$name()`, which
// calls the method named by the variable.
func isStaticPropertyAccess(tokens []parser.Token, i int) bool {
	prev := prevSignificant(tokens, i-1)
	if prev < 0 || tokens[prev].Kind != parser.TokenDoubleColon {
		return false
	}
	next := nextSignificant(tokens, i+1)
	return next < 0 || tokens[next].Kind != parser.TokenOpenParen
}

func isGlobalOrStaticKeyword(tok parser.Token) bool {
	return tok.Kind == parser.TokenStatic || (tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, "global"))
}

// parseGlobalOrStaticBindings declares the variables of `global $a, $b;` and
// `static $a, $b = 1;`. Those with an initializer are left to the assignment path.
func parseGlobalOrStaticBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	i := nextSignificant(tokens, start+1)
	for i >= 0 && tokens[i].Kind == parser.TokenVariable {
		next := nextSignificant(tokens, i+1)
		if next < 0 {
			break
		}
		if tokens[next].Kind != parser.TokenEquals {
			decls[i] = ensureBinding(doc, scope, tokens[i].Value, BindingVariable, tokenRange(tokens[i]))
		}
		depth := 0
		for next < len(tokens) && (depth > 0 || (tokens[next].Kind != parser.TokenComma && tokens[next].Kind != parser.TokenSemicolon)) {
			switch tokens[next].Kind {
			case parser.TokenOpenParen, parser.TokenOpenBracket:
				depth++
			case parser.TokenCloseParen, parser.TokenCloseBracket:
				depth--
			}
			next++
		}
		if next >= len(tokens) || tokens[next].Kind == parser.TokenSemicolon {
			break
		}
		i = nextSignificant(tokens, next+1)
	}
	return decls
}

// addInterpolatedReferences records the `$name` and `{$name` reads inside a
// double-quoted string. `${name}` isn't recorded, because a reference's range
// includes the dollar sign, which renaming would put in the wrong place.
func addInterpolatedReferences(tok parser.Token, active []scopeFrame) {
	if len(tok.Value) < 2 || tok.Value[0] != '"' {
		return
	}
	line, col := tok.Line, tok.Column
	for i := 0; i < len(tok.Value); i++ {
		switch ch := tok.Value[i]; {
		case ch == '\\':
			i++
			col++
		case ch == '\n':
			line++
			col = -1
		case ch == '$':
			end := i + 1
			for end < len(tok.Value) && isVariableChar(tok.Value[end], end == i+1) {
				end++
			}
			if end > i+1 {
				if binding := resolveVariable(active, tok.Value[i:end]); binding != nil {
					binding.References = append(binding.References, protocol.Range{
						Start: protocol.Position{Line: line, Character: col},
						End:   protocol.Position{Line: line, Character: col + end - i},
					})
				}
			}
		}
		col++
	}
}

func isVariableChar(ch byte, first bool) bool {
	return ch == '_' || (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || ch >= 0x80 || (!first && ch >= '0' && ch <= '9')
}

// isSubscriptWrite matches `$a[] = 1` and `$a['b'][$c] = 1`, which create $a.
func isSubscriptWrite(tokens []parser.Token, i int) bool {
	next := nextSignificant(tokens, i+1)
	if next < 0 || tokens[next].Kind != parser.TokenOpenBracket {
		return false
	}
	for next >= 0 && tokens[next].Kind == parser.TokenOpenBracket {
		end := findMatching(tokens, next, parser.TokenOpenBracket, parser.TokenCloseBracket)
		if end < 0 {
			return false
		}
		next = nextSignificant(tokens, end+1)
	}
	return next >= 0 && tokens[next].Kind == parser.TokenEquals
}

// notCalls are keywords that take a parenthesized list without being a call, so
// their variables are only ever read.
var notCalls = map[string]bool{
	"if": true, "elseif": true, "while": true, "for": true, "foreach": true, "switch": true,
	"catch": true, "match": true, "isset": true, "empty": true, "unset": true, "array": true,
	"list": true, "echo": true, "print": true, "exit": true, "die": true, "eval": true,
	"include": true, "include_once": true, "require": true, "require_once": true,
	"and": true, "or": true, "xor": true, "clone": true, "declare": true,
}

// isOutArgument reports whether the variable at i is a whole argument of a call
// that may write to it.
func isOutArgument(tokens []parser.Token, i int, outArg func(string, int) bool) bool {
	if outArg == nil {
		return false
	}
	prev, next := prevSignificant(tokens, i-1), nextSignificant(tokens, i+1)
	if prev < 0 || next < 0 {
		return false
	}
	if tokens[prev].Kind != parser.TokenOpenParen && tokens[prev].Kind != parser.TokenComma {
		return false
	}
	if tokens[next].Kind != parser.TokenCloseParen && tokens[next].Kind != parser.TokenComma {
		return false
	}

	arg, depth := 0, 0
	open := prev
	for ; open >= 0; open = prevSignificant(tokens, open-1) {
		kind := tokens[open].Kind
		if depth == 0 && kind == parser.TokenOpenParen {
			break
		}
		switch kind {
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth++
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			if depth == 0 {
				return false
			}
			depth--
		case parser.TokenComma:
			if depth == 0 {
				arg++
			}
		case parser.TokenSemicolon:
			if depth == 0 {
				return false
			}
		}
	}
	if open < 0 {
		return false
	}

	callee := prevSignificant(tokens, open-1)
	if callee < 0 {
		return false
	}
	switch tokens[callee].Kind {
	case parser.TokenVariable, parser.TokenCloseParen, parser.TokenCloseBracket:
		return outArg("", arg)
	case parser.TokenStatic:
		before := prevSignificant(tokens, callee-1)
		return before >= 0 && tokens[before].Kind == parser.TokenNew && outArg("", arg)
	case parser.TokenIdentifier:
	default:
		return false
	}

	name := tokens[callee].Value
	first := callee
	for {
		sep := prevSignificant(tokens, first-1)
		if sep < 0 || tokens[sep].Kind != parser.TokenBackslash {
			break
		}
		first = sep
		name = "\\" + name
		part := prevSignificant(tokens, sep-1)
		if part < 0 || tokens[part].Kind != parser.TokenIdentifier {
			break
		}
		first = part
		name = tokens[part].Value + name
	}
	if notCalls[strings.ToLower(name)] {
		return false
	}
	if before := prevSignificant(tokens, first-1); before >= 0 {
		switch tokens[before].Kind {
		case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenNew:
			return outArg("", arg)
		case parser.TokenFunction:
			return false
		}
	}
	return outArg(name, arg)
}

func parseUseBindings(tokens []parser.Token, doc *Document, scope *Scope) map[int]*Binding {
	decls := map[int]*Binding{}
	braceDepth := 0
	for i := 0; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenBrace:
			braceDepth++
		case parser.TokenCloseBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case parser.TokenUse:
			if braceDepth != 0 {
				continue
			}
			for idx, binding := range parseSingleUseBinding(tokens, i, doc, scope) {
				decls[idx] = binding
			}
		}
	}
	return decls
}

func parseSingleUseBinding(tokens []parser.Token, start int, doc *Document, scope *Scope) map[int]*Binding {
	decls := map[int]*Binding{}
	semi := nextTokenKind(tokens, start+1, parser.TokenSemicolon)
	if semi < 0 {
		return decls
	}
	openGroup := -1
	for i := start + 1; i < semi; i++ {
		if tokens[i].Kind == parser.TokenOpenBrace {
			openGroup = i
			break
		}
	}
	if openGroup < 0 {
		if idx := useAliasTokenIndex(tokens, start+1, semi); idx >= 0 {
			decls[idx] = doc.addBinding(scope, tokens[idx].Value, BindingImport, tokenRange(tokens[idx]), nil)
		}
		return decls
	}
	closeGroup := findMatching(tokens, openGroup, parser.TokenOpenBrace, parser.TokenCloseBrace)
	if closeGroup < 0 || closeGroup > semi {
		return decls
	}
	itemStart := openGroup + 1
	for i := openGroup + 1; i <= closeGroup; i++ {
		if i < closeGroup && tokens[i].Kind != parser.TokenComma {
			continue
		}
		if idx := useAliasTokenIndex(tokens, itemStart, i); idx >= 0 {
			decls[idx] = doc.addBinding(scope, tokens[idx].Value, BindingImport, tokenRange(tokens[idx]), nil)
		}
		itemStart = i + 1
	}
	return decls
}

func useAliasTokenIndex(tokens []parser.Token, start, end int) int {
	aliasIdx := -1
	lastIdentifierIdx := -1
	for i := start; i < end; i++ {
		if tokens[i].Kind == parser.TokenIdentifier {
			lastIdentifierIdx = i
			if tokens[i].Value == "as" {
				aliasIdx = nextSignificant(tokens, i+1)
				if aliasIdx >= end || tokens[aliasIdx].Kind != parser.TokenIdentifier {
					return -1
				}
				return aliasIdx
			}
		}
	}
	return lastIdentifierIdx
}

func nextTokenKind(tokens []parser.Token, start int, kind parser.TokenKind) int {
	for i := start; i < len(tokens); i++ {
		if tokens[i].Kind == kind {
			return i
		}
	}
	return -1
}

func bindArrowCaptures(tokens []parser.Token, bodyStart, bodyEnd int, parent, scope *Scope, doc *Document, decls map[int]*Binding) {
	active := []scopeFrame{
		{scope: parent, exprEnd: -1},
		{scope: scope, exprEnd: bodyEnd},
	}
	for i := bodyStart; i <= bodyEnd; i++ {
		if tokens[i].Kind != parser.TokenVariable {
			continue
		}
		if _, ok := decls[i]; ok {
			continue
		}
		binding := resolveVariable(active, tokens[i].Value)
		if binding == nil {
			continue
		}
		if binding.Scope != scope {
			binding = ensureArrowCapture(doc, scope, binding, tokenRange(tokens[i]))
			decls[i] = binding
			continue
		}
	}
}

func ensureArrowCapture(doc *Document, scope *Scope, origin *Binding, ref protocol.Range) *Binding {
	for _, binding := range scope.Bindings {
		if binding.Name == origin.Name && binding.Kind == BindingClosureUse && binding.Origin == origin {
			binding.References = append(binding.References, ref)
			return binding
		}
	}
	return doc.addBinding(scope, origin.Name, BindingClosureUse, ref, origin)
}

func findArrowExpressionEnd(tokens []parser.Token, start int) int {
	if start < 0 {
		return -1
	}
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen:
			parenDepth++
		case parser.TokenCloseParen:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return i - 1
			}
			if parenDepth > 0 {
				parenDepth--
			}
		case parser.TokenOpenBracket:
			bracketDepth++
		case parser.TokenCloseBracket:
			if bracketDepth == 0 && parenDepth == 0 && braceDepth == 0 {
				return i - 1
			}
			if bracketDepth > 0 {
				bracketDepth--
			}
		case parser.TokenOpenBrace:
			braceDepth++
		case parser.TokenCloseBrace:
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				return i - 1
			}
			if braceDepth > 0 {
				braceDepth--
			}
		case parser.TokenComma, parser.TokenSemicolon:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return prevSignificant(tokens, i-1)
			}
		}
	}
	return prevSignificant(tokens, len(tokens)-1)
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

func buildAssignment(tokens []parser.Token, variableIdx, equalsIdx, relativeBraceDepth int) *Assignment {
	stmtStart := findStatementStart(tokens, variableIdx)
	stmtEnd := findStatementEnd(tokens, equalsIdx+1)
	exprStart := nextSignificant(tokens, equalsIdx+1)
	if stmtStart < 0 || stmtEnd < 0 || exprStart < 0 {
		return nil
	}

	exprEnd := prevSignificant(tokens, stmtEnd-1)
	if exprEnd < exprStart {
		return nil
	}

	return &Assignment{
		Variable:           tokenRange(tokens[variableIdx]),
		Expression:         protocol.Range{Start: tokenRange(tokens[exprStart]).Start, End: tokenRange(tokens[exprEnd]).End},
		Statement:          protocol.Range{Start: tokenRange(tokens[stmtStart]).Start, End: tokenRange(tokens[stmtEnd]).End},
		RelativeBraceDepth: relativeBraceDepth,
		DirectStatement:    stmtStart == variableIdx && tokens[stmtEnd].Kind == parser.TokenSemicolon,
	}
}

func findStatementStart(tokens []parser.Token, first int) int {
	depthParen := 0
	depthBracket := 0
	for i := first - 1; i >= 0; i-- {
		tok := tokens[i]
		if isTriviaToken(tok.Kind) {
			continue
		}
		switch tok.Kind {
		case parser.TokenCloseParen:
			depthParen++
		case parser.TokenOpenParen:
			if depthParen > 0 {
				depthParen--
				continue
			}
		case parser.TokenCloseBracket:
			depthBracket++
		case parser.TokenOpenBracket:
			if depthBracket > 0 {
				depthBracket--
				continue
			}
		}
		if depthParen == 0 && depthBracket == 0 {
			switch tok.Kind {
			case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
				return nextSignificant(tokens, i+1)
			}
		}
	}
	return nextSignificant(tokens, 0)
}

func findStatementEnd(tokens []parser.Token, start int) int {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for i := start; i < len(tokens); i++ {
		if isTriviaToken(tokens[i].Kind) {
			continue
		}
		switch tokens[i].Kind {
		case parser.TokenOpenParen:
			depthParen++
		case parser.TokenCloseParen:
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return -1
			}
			if depthParen > 0 {
				depthParen--
			}
		case parser.TokenOpenBracket:
			depthBracket++
		case parser.TokenCloseBracket:
			if depthBracket == 0 && depthParen == 0 && depthBrace == 0 {
				return -1
			}
			if depthBracket > 0 {
				depthBracket--
			}
		case parser.TokenOpenBrace:
			depthBrace++
		case parser.TokenCloseBrace:
			if depthBrace == 0 && depthParen == 0 && depthBracket == 0 {
				return -1
			}
			if depthBrace > 0 {
				depthBrace--
			}
		case parser.TokenSemicolon:
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
		}
	}
	return -1
}

func isTriviaToken(kind parser.TokenKind) bool {
	return kind == parser.TokenWhitespace || kind == parser.TokenComment || kind == parser.TokenDocComment
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

func scopeContains(outer, inner protocol.Range) bool {
	if comparePos(outer.Start, inner.Start) > 0 {
		return false
	}
	return comparePos(outer.End, inner.End) >= 0
}

func comparePos(a, b protocol.Position) int {
	if a.Line != b.Line {
		if a.Line < b.Line {
			return -1
		}
		return 1
	}
	if a.Character < b.Character {
		return -1
	}
	if a.Character > b.Character {
		return 1
	}
	return 0
}
