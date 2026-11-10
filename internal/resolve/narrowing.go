package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Narrowing: what calls between a variable's last assignment and the cursor
// say about it, following PHPStan (https://phpstan.org/writing-php-code/narrowing-types):
//
//   - @phpstan-assert Type $param after a call that passes the variable:
//     PHPUnit's assertInstanceOf() and webmozart/assert work this way;
//   - @phpstan-assert-if-true and -if-false inside an if on the call, and
//     after an early return guarded by it;
//   - @param-out Type $param after a call that passes the variable by
//     reference;
//   - @phpstan-self-out and @phpstan-this-out on the object a method is
//     called on.
//
// `!Type` removes a type, `=Type` sets it exactly.

// maxNarrowingStatements bounds how many calls before the cursor are read
// for one variable.
const maxNarrowingStatements = 24

// narrowingFact is what one call says about the variable.
type narrowingFact struct {
	typ      ResolvedType
	negated  bool
	equality bool
}

// narrowVariable applies to t what the code between the variable's last
// assignment and pos says about varName.
func (r *Resolver) narrowVariable(varName string, t ResolvedType, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	if file == nil || r.Index == nil || pos.Line >= len(lines) {
		return t
	}
	from := r.lastWriteLine(varName, lines, pos)
	if from < 0 || from > pos.Line || !r.mayNarrow(lines, from, pos, varName) {
		return t
	}
	// Narrowing doesn't nest: the calls it reads type their receivers and
	// arguments without narrowing them in turn, which keeps it linear.
	exit, ok := r.enterResolution("narrowing")
	if !ok {
		return t
	}
	defer exit()

	// Depth at the start of each line from `from` to pos, and the lowest it
	// gets after each line, to tell which statements still hold at pos.
	n := pos.Line - from + 1
	depth := make([]int, n+1)
	for i := 0; i < n; i++ {
		line := lines[from+i]
		if i == n-1 && pos.Character <= len(line) {
			line = line[:pos.Character]
		}
		depth[i+1] = depth[i] + braceDelta(line)
	}
	minAfter := make([]int, n+1)
	minAfter[n] = depth[n]
	for i := n - 1; i >= 0; i-- {
		minAfter[i] = min(depth[i+1], minAfter[i+1])
	}

	statements := 0
	for i := from; i <= pos.Line && statements < maxNarrowingStatements; i++ {
		line := strings.TrimSpace(lines[i])
		if i == pos.Line && pos.Character <= len(lines[i]) {
			line = strings.TrimSpace(lines[i][:pos.Character])
		}
		if line == "" || !strings.Contains(line, varName) || !r.callsNarrowing(line) && !strings.Contains(line, "instanceof") {
			continue
		}
		rel := i - from
		lineDepth := depth[rel]
		// Inside a block that has closed again before pos: doesn't hold.
		if minAfter[rel] < lineDepth {
			continue
		}
		if strings.HasPrefix(line, "if") {
			// An if whose block only leaves (return, throw…): what follows runs
			// only when its condition was false.
			if cond, ok := guardCondition(lines, i); ok && i < pos.Line {
				statements++
				t = r.applyCondition(varName, t, cond, true, lines, protocol.Position{Line: i}, file)
			}
			continue
		}
		if !strings.HasSuffix(line, ";") || strings.HasPrefix(line, "return") {
			continue
		}
		// A statement that is a call: assertSomething($x); $c->add($x);
		stmt := strings.TrimSpace(strings.TrimSuffix(line, ";"))
		if strings.HasPrefix(stmt, varName+" =") || strings.HasPrefix(stmt, varName+"=") {
			continue
		}
		statements++
		if cond, ok := assertedCondition(stmt); ok {
			t = r.applyCondition(varName, t, cond, false, lines, protocol.Position{Line: i, Character: 0}, file)
			continue
		}
		t = r.applyStatementCall(varName, t, stmt, lines, protocol.Position{Line: i, Character: 0}, file)
	}

	// The if blocks pos is inside of.
	for _, h := range enclosingIfConditions(lines, from, pos) {
		t = r.applyCondition(varName, t, h.cond, h.inElse, lines, protocol.Position{Line: h.line}, file)
	}
	// A switch (true) case the read is under.
	t = r.switchCaseInstanceof(varName, t, lines, from, pos, file)
	// An instanceof test before && or || in the read's own statement.
	t = r.statementInstanceof(varName, t, statementPrefix(lines, pos), file)
	return t
}

// mayNarrow reports whether any line after the assignment on line from, up
// to pos, mentions varName and calls something that narrows types (see
// symbols.Index.MayNarrow), which is all that can narrow it. It reads the
// text and does lookups only, so most variables stop here.
func (r *Resolver) mayNarrow(lines []string, from int, pos protocol.Position, varName string) bool {
	for i := pos.Line; i >= from; i-- {
		line := lines[i]
		if i == pos.Line && pos.Character <= len(line) {
			line = line[:pos.Character]
		}
		if !strings.Contains(line, varName) {
			continue
		}
		if strings.Contains(line, "instanceof") {
			return true
		}
		if strings.IndexByte(line, '(') < 0 {
			continue
		}
		if r.callsNarrowing(line) {
			return true
		}
	}
	return false
}

// callsNarrowing reports whether a line calls a method or function whose
// PHPDoc narrows types, or PHPUnit's assertInstanceOf(), which is read by
// name when PHPUnit isn't installed where it can be read.
func (r *Resolver) callsNarrowing(line string) bool {
	for i := 0; i < len(line); i++ {
		if line[i] != '(' {
			continue
		}
		j := i
		for j > 0 && (line[j-1] == ' ' || line[j-1] == '\t') {
			j--
		}
		k := j
		for k > 0 && (isIdentByte(line[k-1]) || line[k-1] >= '0' && line[k-1] <= '9') {
			k--
		}
		if k == j {
			continue
		}
		name := line[k:j]
		if strings.EqualFold(name, "assertInstanceOf") || r.Index != nil && r.Index.MayNarrow(name) {
			return true
		}
	}
	return false
}

// lastWriteLine returns the line of the last assignment to varName before
// pos, or the start of its scope when it's a parameter never written.
func (r *Resolver) lastWriteLine(varName string, lines []string, pos protocol.Position) int {
	s := r.scopeDocument(lines).ScopeAt(pos)
	if s == nil {
		return -1
	}
	for ; s != nil; s = s.Parent {
		for _, b := range s.Bindings {
			if b.Name != varName {
				continue
			}
			last := b.Decl.Start.Line
			if b.Kind == scope.BindingParameter {
				last = s.Range.Start.Line
			}
			for _, w := range b.Writes {
				// A write on the read's own line is the one whose right-hand
				// side it's in, as `$x = $x->inner()` inside an instanceof test.
				if w.Start.Line < pos.Line && w.Start.Line > last {
					last = w.Start.Line
				}
			}
			if last > pos.Line {
				return -1
			}
			return last
		}
		if s.Kind != scope.ScopeClosure || !s.ExprBody {
			return s.Range.Start.Line
		}
	}
	return -1
}

// braceDelta counts how a line changes the brace depth, skipping strings.
func braceDelta(line string) int {
	d := 0
	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' {
				i++
			} else if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '\'', '"':
			quote = c
		case '{':
			d++
		case '}':
			d--
		case '/':
			if i+1 < len(line) && line[i+1] == '/' {
				return d
			}
		case '#':
			return d
		}
	}
	return d
}

// applyStatementCall applies what a call statement says: asserts on the
// argument passed for a parameter, @param-out, and self-out on the receiver.
func (r *Resolver) applyStatementCall(varName string, t ResolvedType, stmt string, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	// Only a call the variable is passed to, or called on, can say anything
	// about it. Checking that first keeps from resolving every call.
	if !passesOrCallsOn(stmt, varName) {
		return t
	}
	callee, receiverExpr, args, ok := r.statementCalleeWith(stmt, lines, pos, file, map[string]ResolvedType{varName: t})
	if !ok {
		return r.applyUnresolvedAssertion(varName, t, stmt, lines, pos, file)
	}
	if callee.member.DocComment == "" {
		return t
	}
	doc := r.parseDoc(callee.member.DocComment)
	if doc == nil || len(doc.Asserts)+len(doc.ParamOuts) == 0 && doc.SelfOut == "" {
		return t
	}
	if strings.TrimSpace(receiverExpr) == varName {
		// Self-out builds on what the object is by now.
		callee.receiver = t
	}
	env := r.calleeEnv(callee, args, lines, pos, file)
	byParam := argumentsByParam(callee.params, args)
	argIs := func(paramName string) bool {
		for i, p := range callee.params {
			if p.name == paramName && i < len(byParam) {
				for _, a := range byParam[i] {
					if strings.TrimSpace(a.Expr) == varName {
						return true
					}
				}
			}
		}
		return false
	}
	for _, a := range doc.Asserts {
		if a.Kind != parser.AssertAlways {
			continue
		}
		if a.Target == "$this" && strings.TrimSpace(receiverExpr) == varName || argIs(a.Target) {
			t = r.applyFact(t, narrowingFact{typ: r.applyEnv(r.parseType(a.Type), env), negated: a.Negated, equality: a.Equality})
		}
	}
	for _, out := range doc.ParamOuts {
		if argIs(out.Name) {
			t = r.applyEnv(r.parseType(out.Type), env)
		}
	}
	if doc.SelfOut != "" && strings.TrimSpace(receiverExpr) == varName {
		t = r.applyEnv(r.parseType(doc.SelfOut), env)
	}
	return t
}

// resolvedCallee is the method or function a call statement calls.
type resolvedCallee struct {
	member   *symbols.Symbol
	receiver ResolvedType
	params   []paramPattern
}

// statementCallee works out what the outermost call of a statement calls:
// foo($x), $obj->foo($x), Foo::bar($x), $this->assertX($x).
func (r *Resolver) statementCallee(stmt string, lines []string, pos protocol.Position, file *parser.FileNode) (callee resolvedCallee, receiverExpr string, args []CallArg, ok bool) {
	return r.statementCalleeWith(stmt, lines, pos, file, nil)
}

// statementCalleeWith is statementCallee with the types of some variables
// already known, so they aren't worked out again.
func (r *Resolver) statementCalleeWith(stmt string, lines []string, pos protocol.Position, file *parser.FileNode, known map[string]ResolvedType) (callee resolvedCallee, receiverExpr string, args []CallArg, ok bool) {
	steps, origin, parsed := parseChain(stmt)
	if !parsed {
		return callee, "", nil, false
	}
	ctx := ExprContext{Lines: lines, Pos: pos, File: file, Locals: known}
	if len(steps) == 0 {
		if origin.kind != 'f' {
			return callee, "", nil, false
		}
		fn := r.lookupFunction(origin.text, file)
		if fn == nil {
			return callee, "", nil, false
		}
		callee = resolvedCallee{member: fn, params: r.paramPatterns(fn, r.parseDoc(fn.DocComment))}
		return callee, "", ParseCallArgs(origin.args), true
	}
	last := steps[len(steps)-1]
	if !last.call || last.op == "[" || last.op == "()" {
		return callee, "", nil, false
	}
	receiverExpr = chainPrefix(stmt, origin, steps[:len(steps)-1])
	recvType := r.chainOriginType(origin, ctx)
	for _, st := range steps[:len(steps)-1] {
		if recvType.IsEmpty() {
			return callee, "", nil, false
		}
		recvType = r.chainStepType(recvType, st, ctx)
	}
	if last.op == "::" {
		recvType = ClassStringTarget(recvType)
	}
	m, owner := r.FindMemberOn(recvType, last.name, true)
	if m == nil {
		return callee, "", nil, false
	}
	callee = resolvedCallee{member: m, receiver: owner, params: r.paramPatterns(m, r.parseDoc(m.DocComment))}
	return callee, receiverExpr, ParseCallArgs(last.args), true
}

// chainPrefix rebuilds the text of a chain's origin and steps, which is what
// a receiver is written as.
func chainPrefix(stmt string, origin chainOrigin, steps []chainStep) string {
	if len(steps) == 0 && origin.kind == '$' {
		return origin.text
	}
	var sb strings.Builder
	switch origin.kind {
	case '$', 'c':
		sb.WriteString(origin.text)
	default:
		return ""
	}
	for _, st := range steps {
		switch st.op {
		case "[":
			sb.WriteString("[" + st.offset + "]")
		case "()":
			sb.WriteString("(" + st.args + ")")
		default:
			sb.WriteString(st.op + st.name)
			if st.call {
				sb.WriteString("(" + st.args + ")")
			}
		}
	}
	return sb.String()
}

// calleeEnv is what a callee's PHPDoc types mean at a call: its templates
// bound from the receiver and the arguments.
func (r *Resolver) calleeEnv(c resolvedCallee, args []CallArg, lines []string, pos protocol.Position, file *parser.FileNode) typeEnv {
	owner := c.member.ParentFQN
	if c.member.Kind == symbols.KindFunction {
		owner = c.member.FQN
	}
	var subst map[string]ResolvedType
	if c.member.Kind == symbols.KindMethod {
		subst = r.classSubst(c.member.ParentFQN, r.ReceiverFor(c.receiver, c.member), c.receiver.Params)
	}
	if len(c.member.Templates) > 0 {
		ctx := ExprContext{Lines: lines, Pos: pos, File: file}
		bound := r.bindCallTemplates(append([]paramPattern(nil), c.params...), &CallSite{Args: args, ArgType: func(expr string, expected *ResolvedType) ResolvedType {
			return r.ExpressionTypeExpecting(expr, expected, ctx)
		}}, templateNames(c.member.Templates), subst, owner, file)
		if subst == nil {
			subst = make(map[string]ResolvedType, len(bound))
		}
		for k, v := range bound {
			subst[k] = v
		}
	}
	subst = r.fillUnbound(subst, c.member.Templates, owner, file)
	static := c.receiver
	return typeEnv{subst: subst, static: static, owner: owner, file: file}
}

// applyFact narrows t by one assertion.
func (r *Resolver) applyFact(t ResolvedType, f narrowingFact) ResolvedType {
	if f.typ.IsEmpty() && !f.typ.Nullable {
		return t
	}
	if f.equality && !f.negated {
		return f.typ
	}
	if f.negated {
		return r.removeType(t, f.typ)
	}
	// What t already holds of the asserted type, or the asserted type.
	var kept []ResolvedType
	for _, p := range t.Parts() {
		if r.typeMatches(p, f.typ) == matchYes {
			kept = append(kept, p)
		}
	}
	if len(kept) > 0 && !(len(t.Parts()) == 1 && f.typ.FQN != "" && !strings.EqualFold(kept[0].FQN, f.typ.FQN) && !IsBuiltinTypeName(f.typ.FQN)) {
		return MakeUnion(kept, false)
	}
	return f.typ
}

// removeType takes the parts matching remove out of t: !null drops null.
func (r *Resolver) removeType(t, remove ResolvedType) ResolvedType {
	if remove.FQN == "" && remove.Nullable || remove.FQN == "null" {
		out := t
		if len(out.Union) > 0 {
			out = MakeUnion(out.Union, false)
		}
		out.Nullable = false
		return out
	}
	var kept []ResolvedType
	for _, p := range t.Parts() {
		if r.typeMatches(p, remove) != matchYes {
			kept = append(kept, p)
		}
	}
	if len(kept) == 0 {
		return t
	}
	return MakeUnion(kept, t.Nullable && !remove.Nullable)
}

// ifCondition is the condition of an if block that holds where it's read.
type ifCondition struct {
	cond   string
	line   int
	inElse bool // the condition was false: pos is in the else block
}

// enclosingIfConditions returns the conditions of the if (and else) blocks
// that pos is inside, from `from` on.
func enclosingIfConditions(lines []string, from int, pos protocol.Position) []ifCondition {
	var out []ifCondition
	depth := 0
	for i := pos.Line; i >= from && i >= 0; i-- {
		line := lines[i]
		if i == pos.Line && pos.Character <= len(line) {
			line = line[:pos.Character]
		}
		for k := len(line) - 1; k >= 0; k-- {
			switch line[k] {
			case '}':
				depth++
			case '{':
				if depth > 0 {
					depth--
					continue
				}
				// This brace opens a block pos is in.
				header := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[:k]), "}"))
				switch {
				case strings.HasPrefix(header, "if") || strings.HasPrefix(header, "elseif") || strings.HasPrefix(header, "else if") || strings.HasPrefix(header, "while"):
					// An elseif's own condition holds in its block; the ones
					// before it were false, which isn't read here.
					if c, ok := conditionOf(header); ok {
						out = append(out, ifCondition{cond: c, line: i})
					}
				case header == "else":
					if c, l, ok := matchingIfCondition(lines, i, from); ok {
						out = append(out, ifCondition{cond: c, line: l, inElse: true})
					}
				case strings.Contains(header, "function") || strings.Contains(header, "fn") || strings.HasPrefix(header, "class"):
					// A function or closure body ends what ifs can say.
					return out
				}
				line = line[:k]
			}
		}
	}
	return out
}

// conditionOf returns the condition of `if (…) {` or `elseif (…) {`.
func conditionOf(header string) (string, bool) {
	open := strings.IndexByte(header, '(')
	if open < 0 {
		return "", false
	}
	close := matchingParen(header, open)
	if close < 0 {
		return "", false
	}
	return strings.TrimSpace(header[open+1 : close]), true
}

// matchingIfCondition finds the condition of the plain if (no elseif) whose
// else block starts on line elseLine.
func matchingIfCondition(lines []string, elseLine, from int) (string, int, bool) {
	depth := 0
	for i := elseLine; i >= from; i-- {
		line := lines[i]
		if i == elseLine {
			k := strings.Index(line, "else")
			if k < 0 {
				return "", 0, false
			}
			line = line[:k]
		}
		for k := len(line) - 1; k >= 0; k-- {
			switch line[k] {
			case '}':
				depth++
			case '{':
				depth--
				if depth == 0 {
					header := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(line[:k]), "}"))
					if strings.HasPrefix(header, "if") {
						c, ok := conditionOf(header)
						return c, i, ok
					}
					return "", 0, false // elseif chains: nothing simple holds
				}
			}
		}
	}
	return "", 0, false
}

// guardCondition returns the condition of an if at line i whose block does
// nothing but leave (return, throw, continue, break, exit), so whatever
// follows it runs only when the condition was false.
func guardCondition(lines []string, i int) (string, bool) {
	line := strings.TrimSpace(lines[i])
	if !strings.HasPrefix(line, "if") {
		return "", false
	}
	cond, ok := conditionOf(line)
	if !ok {
		return "", false
	}
	open := strings.IndexByte(line, '{')
	if open < 0 {
		return "", false
	}
	body := strings.TrimSpace(line[open+1:])
	for j := i + 1; !strings.Contains(body, "}") && j < len(lines) && j <= i+maxGuardLines; j++ {
		body += " " + strings.TrimSpace(lines[j])
	}
	close := strings.IndexByte(body, '}')
	if close < 0 || strings.IndexByte(body[:close], '{') >= 0 {
		return "", false // nested blocks may leave only sometimes
	}
	// The block's last statement leaves; what comes before it doesn't matter.
	stmts := splitTopLevel(strings.TrimSpace(body[:close]), ";")
	last := ""
	for k := len(stmts) - 1; k >= 0; k-- {
		if s := strings.TrimSpace(stmts[k]); s != "" {
			last = s
			break
		}
	}
	for _, kw := range []string{"return", "throw", "continue", "break", "exit", "die"} {
		if hasLeaveKeyword(last, kw) {
			return cond, true
		}
	}
	return "", false
}

// maxGuardLines bounds how far a guard's block is read.
const maxGuardLines = 12

func hasLeaveKeyword(stmt, kw string) bool {
	if !strings.HasPrefix(stmt, kw) {
		return false
	}
	if len(stmt) == len(kw) {
		return true
	}
	c := stmt[len(kw)]
	return c == ' ' || c == '\t' || c == '(' || c == ';'
}

// applyCondition applies what holds of varName when cond is true, or false
// when negate is set.
func (r *Resolver) applyCondition(varName string, t ResolvedType, cond string, negate bool, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	conjuncts := splitTopLevel(cond, "&&")
	if negate && len(conjuncts) > 1 {
		return t // !(a && b) says nothing about either alone
	}
	if negate {
		// !(a || b) is !a && !b: each of them was false.
		if disjuncts := splitTopLevel(cond, "||"); len(disjuncts) > 1 {
			conjuncts = disjuncts
		}
	} else if len(splitTopLevel(cond, "||")) > 1 {
		return t // a || b says nothing about either alone
	}
	for _, c := range conjuncts {
		c = strings.TrimSpace(c)
		value := !negate
		for strings.HasPrefix(c, "!") {
			value = !value
			c = strings.TrimSpace(c[1:])
		}
		if inner, ok := unwrapParens(c); ok {
			c = inner
			for strings.HasPrefix(c, "!") {
				value = !value
				c = strings.TrimSpace(c[1:])
			}
		}
		if name, ok := instanceofTest(c, varName); ok {
			t = r.narrowInstanceof(t, name, value, file)
			continue
		}
		if !strings.Contains(c, varName) || !strings.HasSuffix(c, ")") {
			continue
		}
		callee, receiverExpr, args, ok := r.statementCallee(c, lines, pos, file)
		if !ok || !strings.Contains(callee.member.DocComment, "assert") {
			continue
		}
		doc := r.parseDoc(callee.member.DocComment)
		if doc == nil {
			continue
		}
		env := r.calleeEnv(callee, args, lines, pos, file)
		byParam := argumentsByParam(callee.params, args)
		for _, a := range doc.Asserts {
			holds := a.Kind == parser.AssertIfTrue && value || a.Kind == parser.AssertIfFalse && !value
			opposite := a.Kind == parser.AssertIfTrue && !value || a.Kind == parser.AssertIfFalse && value
			if !holds && !opposite {
				continue
			}
			target := false
			if a.Target == "$this" {
				target = strings.TrimSpace(receiverExpr) == varName
			} else {
				for i, p := range callee.params {
					if p.name == a.Target && i < len(byParam) && len(byParam[i]) > 0 && strings.TrimSpace(byParam[i][0].Expr) == varName {
						target = true
					}
				}
			}
			if !target {
				continue
			}
			fact := narrowingFact{typ: r.applyEnv(r.parseType(a.Type), env), negated: a.Negated, equality: a.Equality}
			if opposite {
				// The other outcome: the type is ruled out. =Type says nothing then.
				if a.Equality {
					continue
				}
				fact.negated = !fact.negated
			}
			t = r.applyFact(t, fact)
		}
	}
	return t
}

// applyUnresolvedAssertion handles PHPUnit's assertInstanceOf(Foo::class, $x)
// when PHPUnit itself isn't installed where it can be read (Symfony runs it
// through simple-phpunit, outside vendor/): the call is what it says.
func (r *Resolver) applyUnresolvedAssertion(varName string, t ResolvedType, stmt string, lines []string, pos protocol.Position, file *parser.FileNode) ResolvedType {
	steps, origin, ok := parseChain(stmt)
	if !ok {
		return t
	}
	name, args := origin.text, origin.args
	if len(steps) > 0 {
		last := steps[len(steps)-1]
		name, args = last.name, last.args
	}
	if !strings.EqualFold(name, "assertInstanceOf") {
		return t
	}
	list := ParseCallArgs(args)
	if len(list) < 2 || strings.TrimSpace(list[1].Expr) != varName {
		return t
	}
	cls := ClassStringTarget(r.ExpressionType(list[0].Expr, ExprContext{Lines: lines, Pos: pos, File: file}))
	if cls.FQN == "" || cls.FQN == "class-string" || IsBuiltinTypeName(cls.FQN) {
		return t
	}
	return cls
}

// passesOrCallsOn reports whether a call statement passes varName as one of
// its arguments or calls a method on it: foo($x), $this->assert($x),
// $x->add(1). It reads the text only.
func passesOrCallsOn(stmt, varName string) bool {
	if strings.HasPrefix(stmt, varName+"->") || strings.HasPrefix(stmt, varName+"?->") {
		return true
	}
	for i := strings.Index(stmt, varName); i >= 0; {
		end := i + len(varName)
		if end < len(stmt) && (isIdentByte(stmt[end]) || stmt[end] >= '0' && stmt[end] <= '9') {
			// $xs, not $x
		} else {
			before := strings.TrimRight(stmt[:i], " \t&")
			after := strings.TrimLeft(stmt[end:], " \t")
			if (strings.HasSuffix(before, "(") || strings.HasSuffix(before, ",") || strings.HasSuffix(before, ":")) &&
				(strings.HasPrefix(after, ")") || strings.HasPrefix(after, ",")) {
				return true
			}
		}
		next := strings.Index(stmt[end:], varName)
		if next < 0 {
			break
		}
		i = end + next
	}
	return false
}
