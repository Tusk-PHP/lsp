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
	Rebinds     []Rebind         // the writes that give it a new value, in order
	declEffect  effect           // where the declaration's value is, when it's a write
	declCaught  []string         // the classes a catch declaration catches, as written
}

// Rebind is a write that gives a binding a new value: `$x = …`, `foreach (… as
// $x)`, `[$x] = …`. A parameter given a new value no longer has its declared
// type. `$x ??= …` is one that may not run: it keeps the value when there is
// one.
type Rebind struct {
	At protocol.Position // where the new value is written
	// Until is the end of the block the write is in, a loop's body for a
	// foreach variable. Past it, the binding may have an earlier value.
	Until protocol.Position
	// Conditional is set when the write may not run at all, as the braceless
	// body of `if (…) $x = …;`.
	Conditional bool
	// Caught lists the classes a `catch (A|B $e)` write catches, as written.
	Caught []string
	block  int // the block Until ends, or -1 outside any
	effect effect
}

// HoldsAt reports whether the value written here is the binding's value at
// pos, whichever way the code went to get there.
func (r *Rebind) HoldsAt(pos protocol.Position) bool {
	return !r.Conditional && r.takesEffectBy(pos) && !positionBefore(r.Until, pos)
}

// LatestRebindBefore returns the last write before pos that gives the binding a
// new value, or nil.
func (b *Binding) LatestRebindBefore(pos protocol.Position) *Rebind {
	var latest *Rebind
	for i := range b.Rebinds {
		if b.Rebinds[i].takesEffectBy(pos) {
			latest = &b.Rebinds[i]
		}
	}
	return latest
}

func positionBefore(a, b protocol.Position) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character)
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
	// The control flow ReachingWrites reads, in the order it appears.
	blocks []block
	jumps  []flowMark
	labels map[int][]protocol.Position // block → the case labels directly in it, in order
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
	// Rebinds wait for the end of the block they're in: blocks has one entry
	// per open brace, rootRebinds those outside any.
	var blocks [][]rebindRef
	var rootRebinds []rebindRef
	foreachVars := map[int]int{}   // foreach variable token → its loop body's `{`, or -1
	caught := map[int][]string{}   // catch variable token → the classes it catches, as written
	destructured := map[int]bool{} // variable tokens written by `[…] = …` or `list(…) = …`
	catchBodies := map[int]int{}   // catch variable token → its catch body's `{`
	// loop or catch body `{` → the rebinds of its foreach or catch variables,
	// which hold only to the end of that body.
	foreachBodies := map[int][]rebindRef{}
	flow := newFlowState(doc, result.Tokens)

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

		for len(pending) > 0 && pending[len(pending)-1].bodyOpenIndex == i && pending[len(pending)-1].expressionBody {
			body := pending[len(pending)-1]
			active = append(active, scopeFrame{scope: body.scope, braceDepth: braceDepth, exprEnd: body.bodyCloseIndex})
			pending = pending[:len(pending)-1]
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
			named := ScopeFunction
			if inClassBody {
				named = ScopeMethod
			}
			if ps, decls := parseFunctionLike(result.Tokens, i, active[len(active)-1].scope, doc, named); ps != nil {
				pending = append(pending, *ps)
				for idx, binding := range decls {
					declByToken[idx] = binding
				}
			}
		}

		if tok.Kind == parser.TokenIdentifier && tok.Value == "foreach" {
			body := foreachBodyOpen(result.Tokens, i)
			for idx, binding := range parseForeachBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				foreachVars[idx] = body
			}
		}

		if tok.Kind == parser.TokenIdentifier && tok.Value == "catch" {
			for idx, binding := range parseCatchBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				caught[idx] = catchTypes(result.Tokens, i, idx)
				if body := catchBodyOpen(result.Tokens, i); body >= 0 {
					catchBodies[idx] = body
				}
			}
		}

		if tok.Kind == parser.TokenIdentifier && tok.Value == "list" {
			for idx, binding := range parseListDestructureBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				destructured[idx] = true
			}
		}

		if tok.Kind == parser.TokenOpenBracket {
			for idx, binding := range parseDestructureBindings(result.Tokens, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				destructured[idx] = true
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
			if next := assignmentOperator(result.Tokens, i); !isDecl && next >= 0 && !isDynamicMemberName(result.Tokens, i) {
				current := active[len(active)-1]
				binding := ensureBinding(doc, current.scope, tok.Value, BindingVariable, tokenRange(tok))
				declByToken[i] = binding
				if assignment := buildAssignment(result.Tokens, i, next, braceDepth-current.braceDepth); assignment != nil {
					binding.Assignments = append(binding.Assignments, assignment)
				}
			}
		}

		if inPropertyHookBlock(classBodies, braceDepth, len(active)) && isPropertyHook(result.Tokens, i) {
			if ps, decls := parsePropertyHook(result.Tokens, i, active[len(active)-1].scope, doc); ps != nil {
				pending = append(pending, *ps)
				for idx, binding := range decls {
					declByToken[idx] = binding
				}
			}
		}

		if tok.Kind == parser.TokenIdentifier {
			flow.keyword(result.Tokens, i)
		}

		if tok.Kind == parser.TokenOpenBrace {
			braceDepth++
			idx := flow.openBlock(result.Tokens, i)
			for _, r := range foreachBodies[i] {
				r.get().block = idx
			}
			blocks = append(blocks, foreachBodies[i])
			delete(foreachBodies, i)
			for len(pending) > 0 && pending[len(pending)-1].bodyOpenIndex == i && !pending[len(pending)-1].expressionBody {
				active = append(active, scopeFrame{scope: pending[len(pending)-1].scope, braceDepth: braceDepth, exprEnd: -1})
				pending = pending[:len(pending)-1]
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
				_, isForeach := foreachVars[i]
				if binding.Decl == tokenRange(tok) {
					binding.declEffect = writeEffect(result.Tokens, i, isForeach)
					binding.declEffect.destructure = destructured[i]
					binding.declCaught = caught[i]
				} else {
					binding.Writes = append(binding.Writes, tokenRange(tok))
					if r, ok := newRebind(result.Tokens, i, foreachVars); ok {
						r.block = flow.current()
						r.effect = writeEffect(result.Tokens, i, isForeach)
						r.effect.destructure = destructured[i]
						r.Caught = caught[i]
						binding.Rebinds = append(binding.Rebinds, r)
						r := rebindRef{binding, len(binding.Rebinds) - 1}
						if body, ok := foreachVars[i]; ok && body >= 0 {
							foreachBodies[body] = append(foreachBodies[body], r)
						} else if body, ok := catchBodies[i]; ok {
							foreachBodies[body] = append(foreachBodies[body], r)
						} else if len(blocks) > 0 {
							blocks[len(blocks)-1] = append(blocks[len(blocks)-1], r)
						} else {
							rootRebinds = append(rootRebinds, r)
						}
					}
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
			if len(blocks) > 0 {
				for _, r := range blocks[len(blocks)-1] {
					r.get().Until = tokenRange(tok).Start
				}
				blocks = blocks[:len(blocks)-1]
			}
			flow.closeBlock(result.Tokens, i)
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

	// Blocks left open run to the end of the file.
	flow.closeAll(root.Range.End)
	for _, block := range append(blocks, rootRebinds) {
		for _, r := range block {
			r.get().Until = root.Range.End
		}
	}
	for _, rs := range foreachBodies {
		for _, r := range rs {
			r.get().Until = root.Range.End
		}
	}

	for _, frame := range active[1:] {
		if frame.exprEnd < 0 {
			frame.scope.Range.End = root.Range.End
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

// isWordToken reports whether t is a word, such as a keyword, rather than
// punctuation, a variable or a literal.
func isWordToken(t parser.Token) bool {
	v := t.Value
	if v == "" || t.Kind == parser.TokenVariable || t.Kind == parser.TokenStringLiteral {
		return false
	}
	for i := 0; i < len(v); i++ {
		c := v[i]
		if c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || i > 0 && c >= '0' && c <= '9' {
			continue
		}
		return false
	}
	return true
}

func parseFunctionLike(tokens []parser.Token, start int, parent *Scope, doc *Document, named ScopeKind) (*pendingScope, map[int]*Binding) {
	name := ""
	kind := ScopeClosure
	nameIdx := parser.NextSignificant(tokens, start+1)
	// `function &name(…)`, `function &(…)` and `fn &(…)` return by reference.
	if nameIdx >= 0 && tokens[nameIdx].Kind == parser.TokenAmpersand {
		nameIdx = parser.NextSignificant(tokens, nameIdx+1)
	}
	// A method may be named with a reserved word (`function use(…)`,
	// `function list(…)`), which the tokenizer reads as a keyword.
	if nameIdx >= 0 && (tokens[nameIdx].Kind == parser.TokenIdentifier || isWordToken(tokens[nameIdx])) {
		name = tokens[nameIdx].Value
		kind = named
		nameIdx = parser.NextSignificant(tokens, nameIdx+1)
	}
	if nameIdx < 0 || tokens[nameIdx].Kind != parser.TokenOpenParen {
		return nil, nil
	}

	scope := &Scope{Kind: kind, Name: name, Parent: parent, Range: protocol.Range{Start: tokenRange(tokens[start]).Start}}
	doc.Scopes = append(doc.Scopes, scope)
	decls := map[int]*Binding{}

	closeParen := parser.MatchingClose(tokens, nameIdx)
	if closeParen < 0 {
		return nil, decls
	}
	bindParameters(tokens, nameIdx, closeParen, scope, doc, decls)

	j := parser.NextSignificant(tokens, closeParen+1)
	if kind == ScopeClosure && j >= 0 && tokens[j].Kind == parser.TokenUse {
		j = parser.NextSignificant(tokens, j+1)
		if j >= 0 && tokens[j].Kind == parser.TokenOpenParen {
			useClose := parser.MatchingClose(tokens, j)
			for i := j + 1; i < useClose; i++ {
				if tokens[i].Kind != parser.TokenVariable {
					continue
				}
				origin := resolveVariable([]scopeFrame{{scope: parent}}, tokens[i].Value)
				binding := doc.addBinding(scope, tokens[i].Value, BindingClosureUse, tokenRange(tokens[i]), origin)
				decls[i] = binding
			}
			j = parser.NextSignificant(tokens, useClose+1)
		}
	}

	for j >= 0 && j < len(tokens) && tokens[j].Kind != parser.TokenOpenBrace && tokens[j].Kind != parser.TokenDoubleArrow && tokens[j].Kind != parser.TokenSemicolon {
		j++
		j = parser.NextSignificant(tokens, j)
	}
	if j >= 0 && j < len(tokens) && tokens[j].Kind == parser.TokenOpenBrace {
		return &pendingScope{scope: scope, bodyOpenIndex: j}, decls
	}
	if j >= 0 && j < len(tokens) && tokens[j].Kind == parser.TokenDoubleArrow {
		bodyStart := parser.NextSignificant(tokens, j+1)
		bodyEnd := parser.ArrowExpressionEnd(tokens, bodyStart)
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

func bindParameters(tokens []parser.Token, open, close int, scope *Scope, doc *Document, decls map[int]*Binding) {
	depth := 0
	named := false
	for i := open + 1; i < close; i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				named = false
			}
		case parser.TokenVariable:
			if depth == 0 && !named {
				named = true
				decls[i] = doc.addBinding(scope, tokens[i].Value, BindingParameter, tokenRange(tokens[i]), nil)
			}
		}
	}
}

func parseForeachBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := parser.NextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return decls
	}
	close := parser.MatchingClose(tokens, open)
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
	open := parser.NextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return decls
	}
	close := parser.MatchingClose(tokens, open)
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

// catchTypes returns the class names in `catch (A|\B\C $e)`, as written,
// for the catch at token start whose variable is token v.
func catchTypes(tokens []parser.Token, start, v int) []string {
	open := parser.NextSignificant(tokens, start+1)
	var names []string
	var b strings.Builder
	flush := func() {
		if b.Len() > 0 {
			names = append(names, b.String())
			b.Reset()
		}
	}
	for i := open + 1; i < v; i++ {
		switch tokens[i].Kind {
		case parser.TokenIdentifier, parser.TokenBackslash:
			b.WriteString(tokens[i].Value)
		case parser.TokenPipe:
			flush()
		case parser.TokenComment, parser.TokenDocComment:
		default:
			return nil
		}
	}
	flush()
	return names
}

// catchBodyOpen returns the `{` that opens the body of the catch at token
// start, or -1.
func catchBodyOpen(tokens []parser.Token, start int) int {
	open := parser.NextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return -1
	}
	close := parser.MatchingClose(tokens, open)
	if close < 0 {
		return -1
	}
	if body := parser.NextSignificant(tokens, close+1); body >= 0 && tokens[body].Kind == parser.TokenOpenBrace {
		return body
	}
	return -1
}

func parseDestructureBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	prev := parser.PrevSignificant(tokens, start-1)
	if prev >= 0 {
		switch tokens[prev].Kind {
		case parser.TokenEquals, parser.TokenDoubleArrow, parser.TokenArrow, parser.TokenDoubleColon:
			return decls
		// A subscript, as in `$a[$key] = 1`, rather than a destructuring.
		case parser.TokenVariable, parser.TokenIdentifier, parser.TokenCloseBracket, parser.TokenCloseParen:
			return decls
		}
	}
	close := parser.MatchingClose(tokens, start)
	if close < 0 {
		return decls
	}
	next := parser.NextSignificant(tokens, close+1)
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
	open := parser.NextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return decls
	}
	close := parser.MatchingClose(tokens, open)
	if close < 0 {
		return decls
	}
	next := parser.NextSignificant(tokens, close+1)
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
	if prev := parser.PrevSignificant(tokens, i-1); prev >= 0 {
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
	for i := parser.NextSignificant(tokens, start+1); i >= 0 && i < len(tokens); i = parser.NextSignificant(tokens, i+1) {
		switch tokens[i].Kind {
		case parser.TokenOpenBrace:
			return i
		case parser.TokenOpenParen:
			if tokens[start].Kind != parser.TokenClass {
				return -1
			}
			if i = parser.MatchingClose(tokens, i); i < 0 {
				return -1
			}
		case parser.TokenSemicolon, parser.TokenCloseBrace, parser.TokenCloseParen, parser.TokenEquals:
			return -1
		}
	}
	return -1
}

// isStaticPropertyAccess matches `self::$name`, but not `self::$name()`, which
// calls the method named by the variable, unless it names the class to
// instantiate: in `new static::$builder($q)` the parentheses hold the
// constructor's arguments.
func isStaticPropertyAccess(tokens []parser.Token, i int) bool {
	prev := parser.PrevSignificant(tokens, i-1)
	if prev < 0 || tokens[prev].Kind != parser.TokenDoubleColon {
		return false
	}
	next := parser.NextSignificant(tokens, i+1)
	return next < 0 || tokens[next].Kind != parser.TokenOpenParen || isNewClassReference(tokens, prev)
}

// isNewClassReference reports whether the `::` at i belongs to the class
// reference of a `new`, as in `new static::$a::$b()`, `new \Foo::$cls()`,
// `new $this->map[$k]::$cls()` or `new ($factory)::$cls()`. A call earlier in the
// chain ends the class reference, so `new Foo()::$m()` isn't one.
func isNewClassReference(tokens []parser.Token, i int) bool {
	j := parser.PrevSignificant(tokens, i-1)
	for j >= 0 {
		switch tokens[j].Kind {
		case parser.TokenCloseBracket:
			open := parser.MatchingOpen(tokens, j)
			if open < 0 {
				return false
			}
			j = parser.PrevSignificant(tokens, open-1)
			continue
		case parser.TokenCloseParen:
			open := parser.MatchingOpen(tokens, j)
			before := parser.PrevSignificant(tokens, open-1)
			return open >= 0 && before >= 0 && tokens[before].Kind == parser.TokenNew
		case parser.TokenVariable, parser.TokenIdentifier, parser.TokenStatic, parser.TokenBackslash:
		default:
			return false
		}
		prev := parser.PrevSignificant(tokens, j-1)
		if prev < 0 {
			return false
		}
		switch tokens[prev].Kind {
		case parser.TokenNew:
			return true
		case parser.TokenDoubleColon, parser.TokenArrow:
			j = parser.PrevSignificant(tokens, prev-1)
		case parser.TokenIdentifier, parser.TokenBackslash: // a qualified name
			j = prev
		default:
			return false
		}
	}
	return false
}

func isGlobalOrStaticKeyword(tok parser.Token) bool {
	return tok.Kind == parser.TokenStatic || (tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, "global"))
}

// parseGlobalOrStaticBindings declares the variables of `global $a, $b;` and
// `static $a, $b = 1;`. Those with an initializer are left to the assignment path.
func parseGlobalOrStaticBindings(tokens []parser.Token, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	i := parser.NextSignificant(tokens, start+1)
	for i >= 0 && tokens[i].Kind == parser.TokenVariable {
		next := parser.NextSignificant(tokens, i+1)
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
		i = parser.NextSignificant(tokens, next+1)
	}
	return decls
}

// addInterpolatedReferences records the `$name` and `{$name` reads inside a
// double-quoted string. `${name}` isn't recorded, because a reference's range
// includes the dollar sign, which renaming would put in the wrong place.
func addInterpolatedReferences(tok parser.Token, active []scopeFrame) {
	if !parser.Interpolates(tok.Value) {
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
// assignmentOperator returns the index of the `=` when the variable at i is
// assigned to, by `=` or by `??=`, which reads as `?`, `?`, `=`. It returns -1
// otherwise.
// rebindRef points at a rebind whose Until is filled in when its block ends.
// Rebinds are kept by value, so it holds an index rather than a pointer.
type rebindRef struct {
	binding *Binding
	index   int
}

func (r rebindRef) get() *Rebind { return &r.binding.Rebinds[r.index] }

// newRebind returns the rebind of the write at token i. `$x ??= …` may keep
// the binding's value, so it's conditional.
func newRebind(tokens []parser.Token, i int, foreachVars map[int]int) (r Rebind, ok bool) {
	r.At = tokenRange(tokens[i]).Start
	if body, isForeach := foreachVars[i]; isForeach {
		// A braceless or `foreach (): … endforeach;` body: where it ends isn't
		// tracked, so the value is never taken as certain.
		r.Conditional = body < 0
		return r, true
	}
	if op := assignmentOperator(tokens, i); op > 0 && tokens[op-1].Kind == parser.TokenQuestion {
		r.Conditional = true
		return r, true
	}
	r.Conditional = isBracelessBody(tokens, i) || isShortCircuitOperand(tokens, i)
	return r, true
}

// isBracelessBody reports whether the write at token i is the body of a
// control structure without braces, `if ($a) $x = 1;` or `else $x = 2;`, or
// follows a `case`, alternative-syntax or ternary colon, so it may not run.
func isBracelessBody(tokens []parser.Token, i int) bool {
	prev := parser.PrevSignificant(tokens, i-1)
	if prev < 0 {
		return false
	}
	switch tokens[prev].Kind {
	case parser.TokenCloseParen, parser.TokenColon:
		return true
	case parser.TokenIdentifier:
		return tokens[prev].Value == "else" || tokens[prev].Value == "do"
	}
	return false
}

// foreachBodyOpen returns the `{` that opens the body of the foreach at token
// start, or -1 when the body has no braces.
func foreachBodyOpen(tokens []parser.Token, start int) int {
	open := parser.NextSignificant(tokens, start+1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return -1
	}
	close := parser.MatchingClose(tokens, open)
	if close < 0 {
		return -1
	}
	body := parser.NextSignificant(tokens, close+1)
	if body < 0 || tokens[body].Kind != parser.TokenOpenBrace {
		return -1
	}
	return body
}

func assignmentOperator(tokens []parser.Token, i int) int {
	next := parser.NextSignificant(tokens, i+1)
	if next < 0 {
		return -1
	}
	if tokens[next].Kind == parser.TokenEquals {
		return next
	}
	if next+2 < len(tokens) && tokens[next].Kind == parser.TokenQuestion && tokens[next+1].Kind == parser.TokenQuestion && tokens[next+2].Kind == parser.TokenEquals {
		return next + 2
	}
	return -1
}

// inPropertyHookBlock reports whether this is directly inside the braces after
// a property's name: one level into a class body, and not in a function.
func inPropertyHookBlock(bodies []classBody, braceDepth, activeLen int) bool {
	if len(bodies) == 0 {
		return false
	}
	body := bodies[len(bodies)-1]
	return body.braceDepth+1 == braceDepth && body.activeLen == activeLen
}

func isPropertyHook(tokens []parser.Token, i int) bool {
	tok := tokens[i]
	if tok.Kind != parser.TokenIdentifier || (!strings.EqualFold(tok.Value, "get") && !strings.EqualFold(tok.Value, "set")) {
		return false
	}
	next := parser.NextSignificant(tokens, i+1)
	if next < 0 {
		return false
	}
	switch tokens[next].Kind {
	case parser.TokenOpenBrace, parser.TokenDoubleArrow, parser.TokenOpenParen:
		return true
	}
	return false
}

// parsePropertyHook gives a PHP 8.4 `get` or `set` hook its own scope. A `set`
// hook has its parameter, which is an implicit $value unless it names its own.
func parsePropertyHook(tokens []parser.Token, start int, parent *Scope, doc *Document) (*pendingScope, map[int]*Binding) {
	scope := &Scope{Kind: ScopeMethod, Name: tokens[start].Value, Parent: parent, Range: protocol.Range{Start: tokenRange(tokens[start]).Start}}
	doc.Scopes = append(doc.Scopes, scope)
	decls := map[int]*Binding{}

	j := parser.NextSignificant(tokens, start+1)
	if j >= 0 && tokens[j].Kind == parser.TokenOpenParen {
		closeParen := parser.MatchingClose(tokens, j)
		if closeParen < 0 {
			return nil, nil
		}
		bindParameters(tokens, j, closeParen, scope, doc, decls)
		j = parser.NextSignificant(tokens, closeParen+1)
	} else if strings.EqualFold(tokens[start].Value, "set") {
		doc.addBinding(scope, "$value", BindingParameter, tokenRange(tokens[start]), nil)
	}
	if j < 0 {
		return nil, nil
	}
	switch tokens[j].Kind {
	case parser.TokenOpenBrace:
		return &pendingScope{scope: scope, bodyOpenIndex: j}, decls
	case parser.TokenDoubleArrow:
		bodyStart := parser.NextSignificant(tokens, j+1)
		bodyEnd := parser.ArrowExpressionEnd(tokens, bodyStart)
		if bodyStart < 0 || bodyEnd < 0 {
			return nil, nil
		}
		scope.ExprBody = true
		scope.Range.End = tokenRange(tokens[bodyEnd]).End
		return &pendingScope{scope: scope, bodyOpenIndex: bodyStart, bodyCloseIndex: bodyEnd, expressionBody: true}, decls
	}
	return nil, nil
}

func isSubscriptWrite(tokens []parser.Token, i int) bool {
	next := parser.NextSignificant(tokens, i+1)
	if next < 0 || tokens[next].Kind != parser.TokenOpenBracket {
		return false
	}
	for next >= 0 && tokens[next].Kind == parser.TokenOpenBracket {
		end := parser.MatchingClose(tokens, next)
		if end < 0 {
			return false
		}
		next = parser.NextSignificant(tokens, end+1)
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
	prev, next := parser.PrevSignificant(tokens, i-1), parser.NextSignificant(tokens, i+1)
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
	for ; open >= 0; open = parser.PrevSignificant(tokens, open-1) {
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

	callee := parser.PrevSignificant(tokens, open-1)
	if callee < 0 {
		return false
	}
	switch tokens[callee].Kind {
	case parser.TokenVariable, parser.TokenCloseParen, parser.TokenCloseBracket:
		return outArg("", arg)
	case parser.TokenStatic:
		before := parser.PrevSignificant(tokens, callee-1)
		return before >= 0 && tokens[before].Kind == parser.TokenNew && outArg("", arg)
	case parser.TokenIdentifier:
	default:
		return false
	}

	name := tokens[callee].Value
	first := callee
	for {
		sep := parser.PrevSignificant(tokens, first-1)
		if sep < 0 || tokens[sep].Kind != parser.TokenBackslash {
			break
		}
		first = sep
		name = "\\" + name
		part := parser.PrevSignificant(tokens, sep-1)
		if part < 0 || tokens[part].Kind != parser.TokenIdentifier {
			break
		}
		first = part
		name = tokens[part].Value + name
	}
	if notCalls[strings.ToLower(name)] {
		return false
	}
	if before := parser.PrevSignificant(tokens, first-1); before >= 0 {
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
	closeGroup := parser.MatchingClose(tokens, openGroup)
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
				aliasIdx = parser.NextSignificant(tokens, i+1)
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

func buildAssignment(tokens []parser.Token, variableIdx, equalsIdx, relativeBraceDepth int) *Assignment {
	stmtStart := parser.StatementStart(tokens, variableIdx)
	stmtEnd := findStatementEnd(tokens, equalsIdx+1)
	exprStart := parser.NextSignificant(tokens, equalsIdx+1)
	if stmtEnd < 0 || exprStart < 0 {
		return nil
	}

	exprEnd := parser.PrevSignificant(tokens, stmtEnd-1)
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

func findStatementEnd(tokens []parser.Token, start int) int {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for i := start; i < len(tokens); i++ {
		if tokens[i].Kind.IsComment() {
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
