package scope

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/textpos"
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
	Range    textpos.Range
	ExprBody bool
	Parent   *Scope
	Bindings []*Binding
}

type Binding struct {
	Name        string
	Kind        BindingKind
	Decl        textpos.Range
	Scope       *Scope
	Origin      *Binding
	Assignments []*Assignment
	References  []textpos.Range
	Writes      []textpos.Range // later assignments; Decl is the first
	Rebinds     []Rebind        // the writes that give it a new value, in order
	declEffect  effect          // where the declaration's value is, when it's a write
	declCaught  []string        // the classes a catch declaration catches, as written
}

// Rebind is a write that gives a binding a new value: `$x = …`, `foreach (… as
// $x)`, `[$x] = …`. A parameter given a new value no longer has its declared
// type. `$x ??= …` is one that may not run: it keeps the value when there is
// one.
type Rebind struct {
	At textpos.Position // where the new value is written
	// Until is the end of the block the write is in, a loop's body for a
	// foreach variable. Past it, the binding may have an earlier value.
	Until textpos.Position
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
func (r *Rebind) HoldsAt(pos textpos.Position) bool {
	return !r.Conditional && r.takesEffectBy(pos) && !positionBefore(r.Until, pos)
}

// LatestRebindBefore returns the last write before pos that gives the binding a
// new value, or nil.
func (b *Binding) LatestRebindBefore(pos textpos.Position) *Rebind {
	var latest *Rebind
	for i := range b.Rebinds {
		if b.Rebinds[i].takesEffectBy(pos) {
			latest = &b.Rebinds[i]
		}
	}
	return latest
}

func positionBefore(a, b textpos.Position) bool {
	return a.Line < b.Line || (a.Line == b.Line && a.Character < b.Character)
}

type Assignment struct {
	Variable           textpos.Range
	Expression         textpos.Range
	Statement          textpos.Range
	RelativeBraceDepth int
	DirectStatement    bool
}

type Document struct {
	Scopes   []*Scope
	Bindings []*Binding
	// Properties lists `$name` tokens that are class properties rather than
	// variables: declarations, and static accesses such as `self::$name`.
	Properties []textpos.Range
	root       *Scope
	// The control flow ReachingWrites reads, in the order it appears.
	blocks []block
	jumps  []flowMark
	labels map[int][]textpos.Position // block → the case labels directly in it, in order
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
	return c.collect(lexTokens(source))
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

func (c *Collector) collect(t *tokens) *Document {
	doc := &Document{}
	root := &Scope{Kind: ScopeFile, Range: textpos.Range{End: t.fileEnd()}}
	doc.root = root
	doc.Scopes = append(doc.Scopes, root)

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
	flow := newFlowState(doc, t)

	popFinishedExpressions := func(i int) {
		for len(active) > 1 && active[len(active)-1].exprEnd == i {
			active[len(active)-1].scope.Range.End = t.rng(i).End
			active = active[:len(active)-1]
		}
	}

	for idx, binding := range parseUseBindings(t, doc, root) {
		declByToken[idx] = binding
	}

	nextString := 0
	for i := 0; i < len(t.list); i++ {
		kind := t.kind(i)

		for len(pending) > 0 && pending[len(pending)-1].bodyOpenIndex == i && pending[len(pending)-1].expressionBody {
			body := pending[len(pending)-1]
			active = append(active, scopeFrame{scope: body.scope, braceDepth: braceDepth, exprEnd: body.bodyCloseIndex})
			pending = pending[:len(pending)-1]
		}

		if isClassKeyword(t, i) {
			pendingClassBody = classBodyOpen(t, i)
		}

		inClassBody := len(classBodies) > 0 && classBodies[len(classBodies)-1].braceDepth == braceDepth && classBodies[len(classBodies)-1].activeLen == len(active)

		if !inClassBody && (kind == token.Static || kind == token.Global) {
			for idx, binding := range parseGlobalOrStaticBindings(t, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
			}
		}

		if kind == token.Function || kind == token.Fn {
			named := ScopeFunction
			if inClassBody {
				named = ScopeMethod
			}
			if ps, decls := parseFunctionLike(t, i, active[len(active)-1].scope, doc, named); ps != nil {
				pending = append(pending, *ps)
				for idx, binding := range decls {
					declByToken[idx] = binding
				}
			}
		}

		if kind == token.Foreach {
			body := foreachBodyOpen(t, i)
			for idx, binding := range parseForeachBindings(t, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				foreachVars[idx] = body
			}
		}

		if kind == token.Catch {
			for idx, binding := range parseCatchBindings(t, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				caught[idx] = catchTypes(t, i, idx)
				if body := catchBodyOpen(t, i); body >= 0 {
					catchBodies[idx] = body
				}
			}
		}

		if kind == token.List {
			for idx, binding := range parseListDestructureBindings(t, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				destructured[idx] = true
			}
		}

		if kind == token.LeftBracket {
			for idx, binding := range parseDestructureBindings(t, i, active[len(active)-1].scope, doc) {
				declByToken[idx] = binding
				destructured[idx] = true
			}
		}

		if kind == token.Variable {
			if _, isDecl := declByToken[i]; !isDecl && (inClassBody || isStaticPropertyAccess(t, i)) {
				doc.Properties = append(doc.Properties, t.rng(i))
				popFinishedExpressions(i)
				continue
			}
		}

		if kind == token.Variable {
			_, isDecl := declByToken[i]
			if next := assignmentOperator(t, i); !isDecl && next >= 0 && !isDynamicMemberName(t, i) {
				current := active[len(active)-1]
				binding := ensureBinding(doc, current.scope, t.text(i), BindingVariable, t.rng(i))
				declByToken[i] = binding
				if assignment := buildAssignment(t, i, next, braceDepth-current.braceDepth); assignment != nil {
					binding.Assignments = append(binding.Assignments, assignment)
				}
			}
		}

		if inPropertyHookBlock(classBodies, braceDepth, len(active)) && isPropertyHook(t, i) {
			if ps, decls := parsePropertyHook(t, i, active[len(active)-1].scope, doc); ps != nil {
				pending = append(pending, *ps)
				for idx, binding := range decls {
					declByToken[idx] = binding
				}
			}
		}

		flow.keyword(t, i)

		if kind == token.LeftBrace {
			braceDepth++
			idx := flow.openBlock(t, i)
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

		for ; nextString < len(t.strings) && t.strings[nextString].literal == i; nextString++ {
			if binding := resolveVariable(active, t.strings[nextString].name); binding != nil {
				binding.References = append(binding.References, t.strings[nextString].at)
			}
		}

		if kind == token.Variable {
			if binding, ok := declByToken[i]; ok {
				_, isForeach := foreachVars[i]
				if binding.Decl == t.rng(i) {
					binding.declEffect = writeEffect(t, i, isForeach)
					binding.declEffect.destructure = destructured[i]
					binding.declCaught = caught[i]
				} else {
					binding.Writes = append(binding.Writes, t.rng(i))
					if r, ok := newRebind(t, i, foreachVars); ok {
						r.block = flow.current()
						r.effect = writeEffect(t, i, isForeach)
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
			} else if binding := resolveVariable(active, t.text(i)); binding != nil {
				binding.References = append(binding.References, t.rng(i))
			} else if isSubscriptWrite(t, i) || isOutArgument(t, i, c.OutArg) {
				doc.addBinding(active[len(active)-1].scope, t.text(i), BindingImplicit, t.rng(i), nil)
			}
			popFinishedExpressions(i)
			continue
		}

		if kind == token.RightBrace {
			if len(blocks) > 0 {
				for _, r := range blocks[len(blocks)-1] {
					r.get().Until = t.rng(i).Start
				}
				blocks = blocks[:len(blocks)-1]
			}
			flow.closeBlock(t, i)
			for len(active) > 1 && active[len(active)-1].braceDepth == braceDepth {
				active[len(active)-1].scope.Range.End = t.rng(i).End
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
		if ps.scope.Range.End == (textpos.Position{}) {
			ps.scope.Range.End = root.Range.End
		}
	}

	return doc
}

func (d *Document) BindingAt(pos textpos.Position) *Binding {
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

func (d *Document) ScopeAt(pos textpos.Position) *Scope {
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

func (d *Document) Occurrences(binding *Binding) []textpos.Range {
	if binding == nil {
		return nil
	}
	root := binding.root()
	var ranges []textpos.Range
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

func ensureBinding(doc *Document, scope *Scope, name string, kind BindingKind, decl textpos.Range) *Binding {
	for _, binding := range scope.Bindings {
		if binding.Name == name {
			return binding
		}
	}
	return doc.addBinding(scope, name, kind, decl, nil)
}

func (d *Document) addBinding(scope *Scope, name string, kind BindingKind, decl textpos.Range, origin *Binding) *Binding {
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

func parseFunctionLike(t *tokens, start int, parent *Scope, doc *Document, named ScopeKind) (*pendingScope, map[int]*Binding) {
	name := ""
	kind := ScopeClosure
	nameIdx := t.next(start + 1)
	// `function &name(…)`, `function &(…)` and `fn &(…)` return by reference.
	if nameIdx >= 0 && isAmpersand(t.kind(nameIdx)) {
		nameIdx = t.next(nameIdx + 1)
	}
	// A method may be named with a reserved word (`function use(…)`,
	// `function list(…)`), which the tokenizer reads as a keyword.
	if nameIdx >= 0 && t.kind(nameIdx).IsSemiReserved() {
		name = t.text(nameIdx)
		kind = named
		nameIdx = t.next(nameIdx + 1)
	}
	if nameIdx < 0 || t.kind(nameIdx) != token.LeftParen {
		return nil, nil
	}

	scope := &Scope{Kind: kind, Name: name, Parent: parent, Range: textpos.Range{Start: t.rng(start).Start}}
	doc.Scopes = append(doc.Scopes, scope)
	decls := map[int]*Binding{}

	closeParen := t.matchingClose(nameIdx)
	if closeParen < 0 {
		return nil, decls
	}
	bindParameters(t, nameIdx, closeParen, scope, doc, decls)

	j := t.next(closeParen + 1)
	if kind == ScopeClosure && j >= 0 && t.kind(j) == token.Use {
		j = t.next(j + 1)
		if j >= 0 && t.kind(j) == token.LeftParen {
			useClose := t.matchingClose(j)
			for i := j + 1; i < useClose; i++ {
				if t.kind(i) != token.Variable {
					continue
				}
				origin := resolveVariable([]scopeFrame{{scope: parent}}, t.text(i))
				binding := doc.addBinding(scope, t.text(i), BindingClosureUse, t.rng(i), origin)
				decls[i] = binding
			}
			j = t.next(useClose + 1)
		}
	}

	for j >= 0 && j < len(t.list) && t.kind(j) != token.LeftBrace && t.kind(j) != token.DoubleArrow && !isStatementEnd(t.kind(j)) {
		j++
		j = t.next(j)
	}
	if j >= 0 && j < len(t.list) && t.kind(j) == token.LeftBrace {
		return &pendingScope{scope: scope, bodyOpenIndex: j}, decls
	}
	if j >= 0 && j < len(t.list) && t.kind(j) == token.DoubleArrow {
		bodyStart := t.next(j + 1)
		bodyEnd := t.arrowExpressionEnd(bodyStart)
		if bodyStart < 0 || bodyEnd < 0 {
			scope.Range.End = scope.Range.Start
			return nil, decls
		}
		scope.ExprBody = true
		scope.Range.End = t.rng(bodyEnd).End
		bindArrowCaptures(t, bodyStart, bodyEnd, parent, scope, doc, decls)
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

func bindParameters(t *tokens, open, close int, scope *Scope, doc *Document, decls map[int]*Binding) {
	depth := 0
	named := false
	for i := open + 1; i < close; i++ {
		switch k := t.kind(i); {
		case opensBracket(k):
			depth++
		case closesBracket(k):
			depth--
		case k == token.Comma:
			if depth == 0 {
				named = false
			}
		case k == token.Variable:
			if depth == 0 && !named {
				named = true
				decls[i] = doc.addBinding(scope, t.text(i), BindingParameter, t.rng(i), nil)
			}
		}
	}
}

func parseForeachBindings(t *tokens, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := t.next(start + 1)
	if open < 0 || t.kind(open) != token.LeftParen {
		return decls
	}
	close := t.matchingClose(open)
	if close < 0 {
		return decls
	}

	depth := 1
	afterAs := false
	seenArrow := false
	var headerVars []int
	for i := open + 1; i < close; i++ {
		switch t.kind(i) {
		case token.LeftParen, token.LeftBracket, token.Attribute:
			depth++
		case token.RightParen, token.RightBracket:
			depth--
		case token.As:
			if depth == 1 {
				afterAs = true
			}
		case token.DoubleArrow:
			if afterAs && depth == 1 {
				seenArrow = true
			}
		case token.Variable:
			if !afterAs {
				continue
			}
			if depth == 1 {
				headerVars = append(headerVars, i)
				continue
			}
			decls[i] = ensureBinding(doc, scope, t.text(i), BindingDestructure, t.rng(i))
		}
	}
	for idx, tokIdx := range headerVars {
		kind := BindingForeachValue
		if seenArrow && idx == 0 {
			kind = BindingForeachKey
		}
		decls[tokIdx] = ensureBinding(doc, scope, t.text(tokIdx), kind, t.rng(tokIdx))
	}
	return decls
}

func parseCatchBindings(t *tokens, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := t.next(start + 1)
	if open < 0 || t.kind(open) != token.LeftParen {
		return decls
	}
	close := t.matchingClose(open)
	if close < 0 {
		return decls
	}
	for i := open + 1; i < close; i++ {
		if t.kind(i) == token.Variable {
			decls[i] = ensureBinding(doc, scope, t.text(i), BindingCatch, t.rng(i))
		}
	}
	return decls
}

// catchTypes returns the class names in `catch (A|\B\C $e)`, as written,
// for the catch at token start whose variable is token v.
func catchTypes(t *tokens, start, v int) []string {
	open := t.next(start + 1)
	var names []string
	for i := open + 1; i < v; i++ {
		switch k := t.kind(i); {
		case isName(k):
			names = append(names, t.text(i))
		case k == token.BitwiseOr:
		default:
			return nil
		}
	}
	return names
}

// catchBodyOpen returns the `{` that opens the body of the catch at token
// start, or -1.
func catchBodyOpen(t *tokens, start int) int {
	open := t.next(start + 1)
	if open < 0 || t.kind(open) != token.LeftParen {
		return -1
	}
	close := t.matchingClose(open)
	if close < 0 {
		return -1
	}
	if body := t.next(close + 1); body >= 0 && t.kind(body) == token.LeftBrace {
		return body
	}
	return -1
}

func parseDestructureBindings(t *tokens, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	prev := t.prev(start - 1)
	if prev >= 0 {
		switch k := t.kind(prev); {
		case k == token.Equals, k == token.DoubleArrow, isMemberAccess(k), k == token.DoubleColon:
			return decls
		// A subscript, as in `$a[$key] = 1`, rather than a destructuring.
		case k == token.Variable, isName(k), k == token.RightBracket, k == token.RightParen:
			return decls
		}
	}
	close := t.matchingClose(start)
	if close < 0 {
		return decls
	}
	next := t.next(close + 1)
	if next < 0 || t.kind(next) != token.Equals {
		return decls
	}
	for i := start + 1; i < close; i++ {
		if t.kind(i) == token.Variable {
			decls[i] = ensureBinding(doc, scope, t.text(i), BindingDestructure, t.rng(i))
		}
	}
	return decls
}

func parseListDestructureBindings(t *tokens, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	open := t.next(start + 1)
	if open < 0 || t.kind(open) != token.LeftParen {
		return decls
	}
	close := t.matchingClose(open)
	if close < 0 {
		return decls
	}
	next := t.next(close + 1)
	if next < 0 || t.kind(next) != token.Equals {
		return decls
	}
	for i := open + 1; i < close; i++ {
		if t.kind(i) == token.Variable {
			decls[i] = ensureBinding(doc, scope, t.text(i), BindingDestructure, t.rng(i))
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

func isClassKeyword(t *tokens, i int) bool {
	switch t.kind(i) {
	case token.Class, token.Interface, token.Trait, token.Enum:
	default:
		return false
	}
	if prev := t.prev(i - 1); prev >= 0 {
		switch t.kind(prev) {
		case token.DoubleColon, token.Function:
			return false
		}
	}
	return true
}

// classBodyOpen returns the index of the brace opening the body of the class-like
// declared at start, or -1 if it's not a declaration. Only an anonymous class can
// have an argument list.
func classBodyOpen(t *tokens, start int) int {
	for i := t.next(start + 1); i >= 0 && i < len(t.list); i = t.next(i + 1) {
		switch t.kind(i) {
		case token.LeftBrace:
			return i
		case token.LeftParen:
			if t.kind(start) != token.Class {
				return -1
			}
			if i = t.matchingClose(i); i < 0 {
				return -1
			}
		case token.Semicolon, token.CloseTag, token.RightBrace, token.RightParen, token.Equals:
			return -1
		}
	}
	return -1
}

// isStaticPropertyAccess matches `self::$name`, but not `self::$name()`, which
// calls the method named by the variable, unless it names the class to
// instantiate: in `new static::$builder($q)` the parentheses hold the
// constructor's arguments.
func isStaticPropertyAccess(t *tokens, i int) bool {
	prev := t.prev(i - 1)
	if prev < 0 || t.kind(prev) != token.DoubleColon {
		return false
	}
	next := t.next(i + 1)
	return next < 0 || t.kind(next) != token.LeftParen || isNewClassReference(t, prev)
}

// isNewClassReference reports whether the `::` at i belongs to the class
// reference of a `new`, as in `new static::$a::$b()`, `new \Foo::$cls()`,
// `new $this->map[$k]::$cls()` or `new ($factory)::$cls()`. A call earlier in the
// chain ends the class reference, so `new Foo()::$m()` isn't one.
func isNewClassReference(t *tokens, i int) bool {
	j := t.prev(i - 1)
	for j >= 0 {
		switch k := t.kind(j); {
		case k == token.RightBracket:
			open := t.matchingOpen(j)
			if open < 0 {
				return false
			}
			j = t.prev(open - 1)
			continue
		case k == token.RightParen:
			open := t.matchingOpen(j)
			before := t.prev(open - 1)
			return open >= 0 && before >= 0 && t.kind(before) == token.New
		case k == token.Variable, k == token.Static, isName(k):
		default:
			return false
		}
		prev := t.prev(j - 1)
		if prev < 0 {
			return false
		}
		switch k := t.kind(prev); {
		case k == token.New:
			return true
		case k == token.DoubleColon, isMemberAccess(k):
			j = t.prev(prev - 1)
		default:
			return false
		}
	}
	return false
}

// parseGlobalOrStaticBindings declares the variables of `global $a, $b;` and
// `static $a, $b = 1;`. Those with an initializer are left to the assignment path.
func parseGlobalOrStaticBindings(t *tokens, start int, scope *Scope, doc *Document) map[int]*Binding {
	decls := map[int]*Binding{}
	i := t.next(start + 1)
	for i >= 0 && t.kind(i) == token.Variable {
		next := t.next(i + 1)
		if next < 0 {
			break
		}
		if t.kind(next) != token.Equals {
			decls[i] = ensureBinding(doc, scope, t.text(i), BindingVariable, t.rng(i))
		}
		depth := 0
		for next < len(t.list) && (depth > 0 || (t.kind(next) != token.Comma && !isStatementEnd(t.kind(next)))) {
			switch t.kind(next) {
			case token.LeftParen, token.LeftBracket, token.Attribute:
				depth++
			case token.RightParen, token.RightBracket:
				depth--
			}
			next++
		}
		if next >= len(t.list) || isStatementEnd(t.kind(next)) {
			break
		}
		i = t.next(next + 1)
	}
	return decls
}

// rebindRef points at a rebind whose Until is filled in when its block ends.
// Rebinds are kept by value, so it holds an index rather than a pointer.
type rebindRef struct {
	binding *Binding
	index   int
}

func (r rebindRef) get() *Rebind { return &r.binding.Rebinds[r.index] }

// newRebind returns the rebind of the write at token i. `$x ??= …` may keep
// the binding's value, so it's conditional.
func newRebind(t *tokens, i int, foreachVars map[int]int) (r Rebind, ok bool) {
	r.At = t.rng(i).Start
	if body, isForeach := foreachVars[i]; isForeach {
		// A braceless or `foreach (): … endforeach;` body: where it ends isn't
		// tracked, so the value is never taken as certain.
		r.Conditional = body < 0
		return r, true
	}
	if op := assignmentOperator(t, i); op >= 0 && t.kind(op) == token.CoalesceEqual {
		r.Conditional = true
		return r, true
	}
	r.Conditional = isBracelessBody(t, i) || isShortCircuitOperand(t, i)
	return r, true
}

// isBracelessBody reports whether the write at token i is the body of a
// control structure without braces, `if ($a) $x = 1;` or `else $x = 2;`, or
// follows a `case`, alternative-syntax or ternary colon, so it may not run.
func isBracelessBody(t *tokens, i int) bool {
	prev := t.prev(i - 1)
	if prev < 0 {
		return false
	}
	switch t.kind(prev) {
	case token.RightParen, token.Colon, token.Else, token.Do:
		return true
	}
	return false
}

// foreachBodyOpen returns the `{` that opens the body of the foreach at token
// start, or -1 when the body has no braces.
func foreachBodyOpen(t *tokens, start int) int {
	open := t.next(start + 1)
	if open < 0 || t.kind(open) != token.LeftParen {
		return -1
	}
	close := t.matchingClose(open)
	if close < 0 {
		return -1
	}
	body := t.next(close + 1)
	if body < 0 || t.kind(body) != token.LeftBrace {
		return -1
	}
	return body
}

func assignmentOperator(t *tokens, i int) int {
	next := t.next(i + 1)
	if next < 0 {
		return -1
	}
	if k := t.kind(next); k == token.Equals || k == token.CoalesceEqual {
		return next
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

func isPropertyHook(t *tokens, i int) bool {
	if t.kind(i) != token.Identifier || (!strings.EqualFold(t.text(i), "get") && !strings.EqualFold(t.text(i), "set")) {
		return false
	}
	next := t.next(i + 1)
	if next < 0 {
		return false
	}
	switch t.kind(next) {
	case token.LeftBrace, token.DoubleArrow, token.LeftParen:
		return true
	}
	return false
}

// parsePropertyHook gives a PHP 8.4 `get` or `set` hook its own scope. A `set`
// hook has its parameter, which is an implicit $value unless it names its own.
func parsePropertyHook(t *tokens, start int, parent *Scope, doc *Document) (*pendingScope, map[int]*Binding) {
	scope := &Scope{Kind: ScopeMethod, Name: t.text(start), Parent: parent, Range: textpos.Range{Start: t.rng(start).Start}}
	doc.Scopes = append(doc.Scopes, scope)
	decls := map[int]*Binding{}

	j := t.next(start + 1)
	if j >= 0 && t.kind(j) == token.LeftParen {
		closeParen := t.matchingClose(j)
		if closeParen < 0 {
			return nil, nil
		}
		bindParameters(t, j, closeParen, scope, doc, decls)
		j = t.next(closeParen + 1)
	} else if strings.EqualFold(t.text(start), "set") {
		doc.addBinding(scope, "$value", BindingParameter, t.rng(start), nil)
	}
	if j < 0 {
		return nil, nil
	}
	switch t.kind(j) {
	case token.LeftBrace:
		return &pendingScope{scope: scope, bodyOpenIndex: j}, decls
	case token.DoubleArrow:
		bodyStart := t.next(j + 1)
		bodyEnd := t.arrowExpressionEnd(bodyStart)
		if bodyStart < 0 || bodyEnd < 0 {
			return nil, nil
		}
		scope.ExprBody = true
		scope.Range.End = t.rng(bodyEnd).End
		return &pendingScope{scope: scope, bodyOpenIndex: bodyStart, bodyCloseIndex: bodyEnd, expressionBody: true}, decls
	}
	return nil, nil
}

func isSubscriptWrite(t *tokens, i int) bool {
	next := t.next(i + 1)
	if next < 0 || t.kind(next) != token.LeftBracket {
		return false
	}
	for next >= 0 && t.kind(next) == token.LeftBracket {
		end := t.matchingClose(next)
		if end < 0 {
			return false
		}
		next = t.next(end + 1)
	}
	return next >= 0 && t.kind(next) == token.Equals
}

// isOutArgument reports whether the variable at i is a whole argument of a call
// that may write to it.
func isOutArgument(t *tokens, i int, outArg func(string, int) bool) bool {
	if outArg == nil {
		return false
	}
	prev, next := t.prev(i-1), t.next(i+1)
	if prev < 0 || next < 0 {
		return false
	}
	if t.kind(prev) != token.LeftParen && t.kind(prev) != token.Comma {
		return false
	}
	if t.kind(next) != token.RightParen && t.kind(next) != token.Comma {
		return false
	}

	arg, depth := 0, 0
	open := prev
	for ; open >= 0; open = t.prev(open - 1) {
		kind := t.kind(open)
		if depth == 0 && kind == token.LeftParen {
			break
		}
		switch {
		case closesBracket(kind):
			depth++
		case opensBracket(kind):
			if depth == 0 {
				return false
			}
			depth--
		case kind == token.Comma:
			if depth == 0 {
				arg++
			}
		case isStatementEnd(kind):
			if depth == 0 {
				return false
			}
		}
	}
	if open < 0 {
		return false
	}

	callee := t.prev(open - 1)
	if callee < 0 {
		return false
	}
	switch k := t.kind(callee); {
	case k == token.Variable, k == token.RightParen, k == token.RightBracket:
		return outArg("", arg)
	case k == token.Static:
		before := t.prev(callee - 1)
		return before >= 0 && t.kind(before) == token.New && outArg("", arg)
	case !isName(k):
		return false
	}
	if before := t.prev(callee - 1); before >= 0 {
		switch k := t.kind(before); {
		case isMemberAccess(k), k == token.DoubleColon, k == token.New:
			return outArg("", arg)
		case k == token.Function:
			return false
		}
	}
	return outArg(t.text(callee), arg)
}

func parseUseBindings(t *tokens, doc *Document, scope *Scope) map[int]*Binding {
	decls := map[int]*Binding{}
	braceDepth := 0
	for i := 0; i < len(t.list); i++ {
		switch t.kind(i) {
		case token.LeftBrace:
			braceDepth++
		case token.RightBrace:
			if braceDepth > 0 {
				braceDepth--
			}
		case token.Use:
			if braceDepth != 0 {
				continue
			}
			for idx, binding := range parseSingleUseBinding(t, i, doc, scope) {
				decls[idx] = binding
			}
		}
	}
	return decls
}

func parseSingleUseBinding(t *tokens, start int, doc *Document, scope *Scope) map[int]*Binding {
	decls := map[int]*Binding{}
	semi := statementEnd(t, start+1)
	if semi < 0 {
		return decls
	}
	openGroup := -1
	for i := start + 1; i < semi; i++ {
		if t.kind(i) == token.LeftBrace {
			openGroup = i
			break
		}
	}
	if openGroup < 0 {
		if idx := useAliasTokenIndex(t, start+1, semi); idx >= 0 {
			decls[idx] = addImport(t, doc, scope, idx)
		}
		return decls
	}
	closeGroup := t.matchingClose(openGroup)
	if closeGroup < 0 || closeGroup > semi {
		return decls
	}
	itemStart := openGroup + 1
	for i := openGroup + 1; i <= closeGroup; i++ {
		if i < closeGroup && t.kind(i) != token.Comma {
			continue
		}
		if idx := useAliasTokenIndex(t, itemStart, i); idx >= 0 {
			decls[idx] = addImport(t, doc, scope, idx)
		}
		itemStart = i + 1
	}
	return decls
}

func addImport(t *tokens, doc *Document, scope *Scope, idx int) *Binding {
	name, at := t.lastSegment(idx)
	return doc.addBinding(scope, name, BindingImport, at, nil)
}

func useAliasTokenIndex(t *tokens, start, end int) int {
	last := -1
	for i := start; i < end; i++ {
		switch k := t.kind(i); {
		case k == token.As:
			alias := t.next(i + 1)
			if alias >= end || t.kind(alias) != token.Identifier {
				return -1
			}
			return alias
		case isName(k):
			last = i
		}
	}
	return last
}

func statementEnd(t *tokens, start int) int {
	for i := start; i < len(t.list); i++ {
		if isStatementEnd(t.kind(i)) {
			return i
		}
	}
	return -1
}

func bindArrowCaptures(t *tokens, bodyStart, bodyEnd int, parent, scope *Scope, doc *Document, decls map[int]*Binding) {
	active := []scopeFrame{
		{scope: parent, exprEnd: -1},
		{scope: scope, exprEnd: bodyEnd},
	}
	for i := bodyStart; i <= bodyEnd; i++ {
		if t.kind(i) != token.Variable {
			continue
		}
		if _, ok := decls[i]; ok {
			continue
		}
		binding := resolveVariable(active, t.text(i))
		if binding == nil {
			continue
		}
		if binding.Scope != scope {
			binding = ensureArrowCapture(doc, scope, binding, t.rng(i))
			decls[i] = binding
			continue
		}
	}
}

func ensureArrowCapture(doc *Document, scope *Scope, origin *Binding, ref textpos.Range) *Binding {
	for _, binding := range scope.Bindings {
		if binding.Name == origin.Name && binding.Kind == BindingClosureUse && binding.Origin == origin {
			binding.References = append(binding.References, ref)
			return binding
		}
	}
	return doc.addBinding(scope, origin.Name, BindingClosureUse, ref, origin)
}

func buildAssignment(t *tokens, variableIdx, equalsIdx, relativeBraceDepth int) *Assignment {
	stmtStart := lexer.StatementStart(t.list, variableIdx)
	stmtEnd := findStatementEnd(t, equalsIdx+1)
	exprStart := t.next(equalsIdx + 1)
	if stmtEnd < 0 || exprStart < 0 {
		return nil
	}

	exprEnd := t.prev(stmtEnd - 1)
	if exprEnd < exprStart {
		return nil
	}

	return &Assignment{
		Variable:           t.rng(variableIdx),
		Expression:         textpos.Range{Start: t.rng(exprStart).Start, End: t.rng(exprEnd).End},
		Statement:          textpos.Range{Start: t.rng(stmtStart).Start, End: t.rng(stmtEnd).End},
		RelativeBraceDepth: relativeBraceDepth,
		DirectStatement:    stmtStart == variableIdx && isStatementEnd(t.kind(stmtEnd)),
	}
}

func findStatementEnd(t *tokens, start int) int {
	depthParen := 0
	depthBracket := 0
	depthBrace := 0
	for i := start; i < len(t.list); i++ {
		switch k := t.kind(i); {
		case k == token.LeftParen:
			depthParen++
		case k == token.RightParen:
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return -1
			}
			if depthParen > 0 {
				depthParen--
			}
		case opensSquare(k):
			depthBracket++
		case k == token.RightBracket:
			if depthBracket == 0 && depthParen == 0 && depthBrace == 0 {
				return -1
			}
			if depthBracket > 0 {
				depthBracket--
			}
		case k == token.LeftBrace:
			depthBrace++
		case k == token.RightBrace:
			if depthBrace == 0 && depthParen == 0 && depthBracket == 0 {
				return -1
			}
			if depthBrace > 0 {
				depthBrace--
			}
		case isStatementEnd(k):
			if depthParen == 0 && depthBracket == 0 && depthBrace == 0 {
				return i
			}
		}
	}
	return -1
}

func containsPos(r textpos.Range, pos textpos.Position) bool {
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

func dedupeRanges(in []textpos.Range) []textpos.Range {
	type key struct {
		line int
		col  int
		end  int
	}
	seen := make(map[key]bool, len(in))
	var out []textpos.Range
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

func scopeContains(outer, inner textpos.Range) bool {
	if comparePos(outer.Start, inner.Start) > 0 {
		return false
	}
	return comparePos(outer.End, inner.End) >= 0
}

func comparePos(a, b textpos.Position) int {
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
