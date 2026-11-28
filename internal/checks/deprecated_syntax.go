package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// DeprecatedSyntaxRule flags language constructs, names and arguments PHP
// deprecates, as opposed to deprecated functions (DeprecatedFunctionRule):
// `return` in a `finally` block, a constructor or destructor that returns a
// value or yields, a nullable `__debugInfo()` return type, names reserved for
// future keywords (`let`, `is`, `_`, a function called `readonly`, a class
// constant called `namespace`), `define()`'s third argument and
// `SORT_LOCALE_STRING`. What PHP reports and doesn't was checked against PHP
// 8.6's own compiler: methods and class constants may still be called `let`,
// and a closure in a constructor or `finally` block is a function of its own.
type DeprecatedSyntaxRule struct {
	// PHPVersion is the project's lowest supported PHP major.minor; a
	// deprecation from a later version isn't reported. Empty reports all.
	PHPVersion string
	// Shared, when set, shares the file's tokens with the other rules.
	Shared *Shared
}

func (r *DeprecatedSyntaxRule) Codes() []rule.Code { return []rule.Code{rule.Deprecated} }

func (r *DeprecatedSyntaxRule) Check(file *parser.FileNode, source string, _ *symbols.Index) []Finding {
	if file == nil || r.PHPVersion != "" && compareBuiltinVersionsStrings(r.PHPVersion, "8.4") < 0 {
		return nil
	}
	s := &syntaxScan{rule: r, toks: r.Shared.significantTokens(source)}
	s.run()
	return s.findings
}

type syntaxFrameKind uint8

const (
	frameBlock    syntaxFrameKind = iota // any other `{ … }`
	frameClass                           // a class, interface, trait or enum body
	frameFunction                        // a function, method or closure body
	frameArrow                           // an arrow function's expression
	frameFinally                         // a `finally` block
)

// Special methods whose body the rule looks into.
const (
	methodOther = iota
	methodConstructor
	methodDestructor
	methodDebugInfo
)

type syntaxFrame struct {
	kind syntaxFrameKind
	// depth is the nesting depth inside the frame's `{`, or, for an arrow
	// function, the depth its `=>` is at.
	depth int
	// classKind is "class", "interface", "trait" or "enum" for a class body.
	classKind string
	method    int
	yielded   bool
}

// pendingDecl is a class-like or function seen whose body hasn't opened yet.
type pendingDecl struct {
	kind      syntaxFrameKind // frameClass or frameFunction; zero when none
	depth     int
	classKind string
	method    int
	arrow     bool
}

type syntaxScan struct {
	rule     *DeprecatedSyntaxRule
	toks     []parser.Token
	frames   []syntaxFrame
	pending  pendingDecl
	depth    int
	findings []Finding
	// args is reused by every call to arguments.
	args []argRange
}

func (s *syntaxScan) report(from, to int, since, message string) {
	if s.rule.PHPVersion != "" && compareBuiltinVersionsStrings(s.rule.PHPVersion, since) < 0 {
		return
	}
	start, end := s.toks[from], s.toks[to]
	s.findings = append(s.findings, Finding{
		StartLine: start.Line,
		StartCol:  start.Column,
		EndLine:   end.Line,
		EndCol:    end.Column + len(end.Value),
		Severity:  rule.SeverityWarning,
		Code:      rule.Deprecated,
		Message:   message,
		Tags:      []Tag{TagDeprecated},
	})
}

func (s *syntaxScan) top() *syntaxFrame {
	if len(s.frames) == 0 {
		return nil
	}
	return &s.frames[len(s.frames)-1]
}

func (s *syntaxScan) inClassBody() bool {
	f := s.top()
	return f != nil && f.kind == frameClass && f.depth == s.depth
}

// inDeclarationScope reports whether the code is at the top level of the
// file or of a namespace block, where `use` imports and `const` declares a
// global constant.
func (s *syntaxScan) inDeclarationScope() bool {
	for _, f := range s.frames {
		if f.kind != frameBlock {
			return false
		}
	}
	return true
}

// enclosingFunction returns the innermost function or arrow function frame.
func (s *syntaxScan) enclosingFunction() *syntaxFrame {
	for i := len(s.frames) - 1; i >= 0; i-- {
		if k := s.frames[i].kind; k == frameFunction || k == frameArrow {
			return &s.frames[i]
		}
	}
	return nil
}

// inFinally reports whether the code is in a `finally` block of the current
// function; one of an enclosing function doesn't count.
func (s *syntaxScan) inFinally() bool {
	for i := len(s.frames) - 1; i >= 0; i-- {
		switch s.frames[i].kind {
		case frameFinally:
			return true
		case frameFunction, frameArrow, frameClass:
			return false
		}
	}
	return false
}

func (s *syntaxScan) kind(i int) parser.TokenKind {
	if i < 0 || i >= len(s.toks) {
		return parser.TokenEOF
	}
	return s.toks[i].Kind
}

// is reports whether the token at i is word, ignoring case.
func (s *syntaxScan) is(i int, word string) bool {
	return i >= 0 && i < len(s.toks) && strings.EqualFold(s.toks[i].Value, word)
}

func (s *syntaxScan) lower(i int) string {
	if i < 0 || i >= len(s.toks) {
		return ""
	}
	return strings.ToLower(s.toks[i].Value)
}

// afterMember reports whether the token at i follows `->`, `?->` or `::`, or
// is the last part of a namespace-qualified name (`Foo\define`).
func (s *syntaxScan) afterMember(i int) bool {
	switch s.kind(i - 1) {
	case parser.TokenArrow, parser.TokenDoubleColon:
		return true
	case parser.TokenBackslash:
		return s.kind(i-2) == parser.TokenIdentifier
	}
	return false
}

// endArrows closes the arrow functions the token at i ends: a `;` or `,` at
// their own depth, or a closing bracket that leaves it.
func (s *syntaxScan) endArrows(i int) {
	for f := s.top(); f != nil && f.kind == frameArrow; f = s.top() {
		switch s.kind(i) {
		case parser.TokenSemicolon, parser.TokenComma:
			if s.depth > f.depth {
				return
			}
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			if s.depth > f.depth {
				return
			}
		default:
			return
		}
		s.frames = s.frames[:len(s.frames)-1]
	}
}

func (s *syntaxScan) run() {
	for i := 0; i < len(s.toks); i++ {
		s.endArrows(i)
		tok := s.toks[i]
		switch tok.Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			s.depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket:
			s.depth--
			s.dropPending()
			if tok.Kind == parser.TokenCloseParen && s.pending.kind == frameFunction && !s.pending.arrow &&
				s.depth == s.pending.depth && s.pending.method == methodDebugInfo {
				s.checkDebugInfoType(i + 1)
			}
		case parser.TokenOpenBrace:
			frame := syntaxFrame{kind: frameBlock}
			if s.pending.kind != 0 && !s.pending.arrow && s.depth == s.pending.depth {
				frame = syntaxFrame{kind: s.pending.kind, classKind: s.pending.classKind, method: s.pending.method}
				s.pending = pendingDecl{}
			} else if s.is(i-1, "finally") {
				frame.kind = frameFinally
			}
			s.depth++
			frame.depth = s.depth
			s.frames = append(s.frames, frame)
		case parser.TokenCloseBrace:
			if f := s.top(); f != nil && f.depth == s.depth {
				s.frames = s.frames[:len(s.frames)-1]
			}
			s.depth--
			s.dropPending()
		case parser.TokenSemicolon:
			if s.pending.kind != 0 && s.depth == s.pending.depth {
				s.pending = pendingDecl{} // an abstract or interface method
			}
		case parser.TokenDoubleArrow:
			if s.pending.arrow && s.depth == s.pending.depth {
				s.frames = append(s.frames, syntaxFrame{kind: frameArrow, depth: s.depth})
				s.pending = pendingDecl{}
			}
		case parser.TokenClass, parser.TokenInterface, parser.TokenTrait, parser.TokenEnum:
			s.classLike(i)
		case parser.TokenFunction:
			s.functionDecl(i)
		case parser.TokenConst:
			i = s.constants(i)
		case parser.TokenUse:
			i = s.imports(i)
		case parser.TokenReturn:
			s.returns(i)
		case parser.TokenIdentifier:
			s.identifier(i)
		}
	}
}

// dropPending forgets a declaration whose bracket closed before its body
// opened, which means it wasn't a declaration.
func (s *syntaxScan) dropPending() {
	if s.pending.kind != 0 && s.depth < s.pending.depth {
		s.pending = pendingDecl{}
	}
}

func (s *syntaxScan) classLike(i int) {
	if s.afterMember(i) {
		return // Foo::class, $object->class
	}
	switch s.kind(i + 1) {
	case parser.TokenIdentifier, parser.TokenOpenBrace, parser.TokenOpenParen, parser.TokenExtends, parser.TokenImplements:
	default:
		return // a named argument, `class: …`, or a name that isn't a declaration
	}
	kind := s.lower(i)
	if s.kind(i) == parser.TokenEnum && (s.kind(i+1) != parser.TokenIdentifier ||
		s.kind(i+2) != parser.TokenOpenBrace && s.kind(i+2) != parser.TokenColon && !s.is(i+2, "implements")) {
		return // `enum` used as a name
	}
	s.pending = pendingDecl{kind: frameClass, depth: s.depth, classKind: kind}
	prev := i - 1
	for s.kind(prev) == parser.TokenReadonly || s.kind(prev) == parser.TokenFinal || s.kind(prev) == parser.TokenAbstract {
		prev--
	}
	if s.kind(prev) == parser.TokenNew || s.kind(i+1) != parser.TokenIdentifier {
		return // an anonymous class
	}
	name := s.toks[i+1].Value
	switch lower := strings.ToLower(name); {
	case lower == "let" || lower == "is":
		s.report(i+1, i+1, "8.6", reservedNameMessage(lower, kind+" name"))
	case name == "_":
		s.report(i+1, i+1, "8.4", `Using "_" as a class name is deprecated since PHP 8.4`)
	}
}

func (s *syntaxScan) functionDecl(i int) {
	isMethod := s.inClassBody()
	name := i + 1
	if s.kind(name) == parser.TokenAmpersand {
		name++
	}
	arrow := s.is(i, "fn")
	p := pendingDecl{kind: frameFunction, depth: s.depth, arrow: arrow}
	if !arrow && s.kind(name+1) == parser.TokenOpenParen && s.kind(name) != parser.TokenOpenParen {
		lower := s.lower(name)
		switch {
		case isMethod && lower == "__construct":
			p.method = methodConstructor
		case isMethod && lower == "__destruct":
			p.method = methodDestructor
		case isMethod && lower == "__debuginfo":
			p.method = methodDebugInfo
		case !isMethod && (lower == "let" || lower == "is"):
			s.report(name, name, "8.6", reservedNameMessage(lower, "function name"))
		case !isMethod && lower == "readonly":
			s.report(name, name, "8.6", `A function called "readonly" is deprecated since PHP 8.6, readonly is a keyword`)
		}
	}
	s.pending = p
}

// checkDebugInfoType reports `?array` and `array|null` as __debugInfo()'s
// return type, starting at the token after its parameter list.
func (s *syntaxScan) checkDebugInfoType(i int) {
	if s.kind(i) != parser.TokenColon {
		return
	}
	start, end := i+1, i+1
	for end < len(s.toks) && s.kind(end) != parser.TokenOpenBrace && s.kind(end) != parser.TokenSemicolon {
		end++
	}
	var b strings.Builder
	for j := start; j < end; j++ {
		b.WriteString(strings.ToLower(strings.TrimPrefix(s.toks[j].Value, "\\")))
	}
	switch b.String() {
	case "?array", "array|null", "null|array":
		s.report(start, end-1, "8.6", "A nullable return type for __debugInfo() is deprecated since PHP 8.6, declare array and return an empty array instead of null")
	}
}

// constants reads a `const` declaration and returns the index of its last
// token. A global constant may not be called `let`, `is` or `_`; a class
// constant may not be called `namespace`.
func (s *syntaxScan) constants(i int) int {
	global := s.inDeclarationScope()
	var classKind string
	if !global {
		if !s.inClassBody() {
			return i
		}
		classKind = s.top().classKind
	}
	depth := 0
	nameAt := -1
	j := i + 1
	for ; j < len(s.toks); j++ {
		switch s.kind(j) {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
			if depth < 0 {
				return j - 1
			}
		case parser.TokenEquals:
			if depth == 0 && nameAt < 0 {
				nameAt = j - 1
				s.constantName(nameAt, global, classKind)
			}
		case parser.TokenComma:
			if depth == 0 {
				nameAt = -1
			}
		case parser.TokenSemicolon:
			if depth == 0 {
				return j
			}
		}
	}
	return j - 1
}

func (s *syntaxScan) constantName(i int, global bool, classKind string) {
	name := s.toks[i].Value
	lower := strings.ToLower(name)
	switch {
	case global && (lower == "let" || lower == "is"):
		s.report(i, i, "8.6", reservedNameMessage(lower, "constant name"))
	case global && name == "_":
		s.report(i, i, "8.6", `Using "_" as a constant name is deprecated since PHP 8.6`)
	case !global && lower == "namespace":
		s.report(i, i, "8.6", "Declaring "+article(classKind)+" "+classKind+` constant called "namespace" is deprecated since PHP 8.6`)
	}
}

// imports reads a `use` import and returns the index of its last token. The
// name it imports, its alias or else its last segment, may not be `let` or
// `is`, nor `_` for a class or constant.
func (s *syntaxScan) imports(i int) int {
	if !s.inDeclarationScope() || s.kind(i-1) == parser.TokenCloseParen || s.kind(i+1) == parser.TokenOpenParen {
		return i
	}
	kind := "class"
	j := i + 1
	if k := s.lower(j); s.kind(j) == parser.TokenFunction || k == "const" {
		kind, j = k, j+1
	}
	itemKind, last := kind, -1
	for ; j < len(s.toks); j++ {
		switch s.kind(j) {
		case parser.TokenIdentifier:
			if s.is(j, "as") && s.kind(j+1) == parser.TokenIdentifier {
				j++
			}
			last = j
		case parser.TokenFunction, parser.TokenConst:
			itemKind = s.lower(j) // inside a group: `use Foo\{function bar}`
		case parser.TokenBackslash:
			last = -1
		case parser.TokenOpenBrace:
			last = -1
		case parser.TokenComma, parser.TokenCloseBrace, parser.TokenSemicolon:
			if last >= 0 {
				s.importedName(last, itemKind)
			}
			last, itemKind = -1, kind
			if s.kind(j) == parser.TokenSemicolon {
				return j
			}
		default:
			return j - 1
		}
	}
	return j - 1
}

func (s *syntaxScan) importedName(i int, kind string) {
	name := s.toks[i].Value
	lower := strings.ToLower(name)
	switch {
	case lower == "let" || lower == "is":
		s.report(i, i, "8.6", reservedNameMessage(lower, importNoun[kind]))
	case name == "_" && kind == "class":
		s.report(i, i, "8.4", `Using "_" as a class name is deprecated since PHP 8.4`)
	case name == "_" && kind == "const":
		s.report(i, i, "8.6", `Using "_" as a constant name is deprecated since PHP 8.6`)
	}
}

// returns reports a `return` in a `finally` block, and one with a value in a
// constructor or destructor.
func (s *syntaxScan) returns(i int) {
	if s.afterMember(i) {
		return // a property or constant called `return`: $this->return, Foo::RETURN
	}
	if s.inFinally() {
		s.report(i, i, "8.6", "Returning from a finally block is deprecated since PHP 8.6, it discards the exception or the value returned by the try block")
	}
	f := s.enclosingFunction()
	if f == nil || f.kind != frameFunction || f.method == methodOther || f.method == methodDebugInfo ||
		s.kind(i+1) == parser.TokenSemicolon {
		return
	}
	end := s.statementEnd(i + 1)
	what := "constructor"
	if f.method == methodDestructor {
		what = "destructor"
	}
	s.report(i, end, "8.6", "Returning a value from a "+what+" is deprecated since PHP 8.6")
}

// statementEnd returns the index of the last token before the `;` that ends
// the statement starting at i.
func (s *syntaxScan) statementEnd(i int) int {
	depth := 0
	for j := i; j < len(s.toks); j++ {
		switch s.kind(j) {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
			if depth < 0 {
				return j - 1
			}
		case parser.TokenSemicolon, parser.TokenEOF:
			if depth == 0 {
				return j - 1
			}
		}
	}
	return len(s.toks) - 1
}

func (s *syntaxScan) identifier(i int) {
	tok := s.toks[i]
	if len(tok.Value) < 4 || len(tok.Value) > 18 {
		return // shorter or longer than every name below
	}
	switch {
	case s.is(i, "case"):
		// An enum case is one of the enum's constants to PHP.
		if s.inClassBody() && s.top().classKind == "enum" && s.kind(i+1) == parser.TokenIdentifier {
			s.constantName(i+1, false, "enum")
		}
	case s.is(i, "yield"):
		if f := s.enclosingFunction(); f != nil && f.kind == frameFunction && !f.yielded &&
			(f.method == methodConstructor || f.method == methodDestructor) {
			f.yielded = true
			what := "constructor"
			if f.method == methodDestructor {
				what = "destructor"
			}
			s.report(i, i, "8.6", "Making a "+what+" a generator with yield is deprecated since PHP 8.6")
		}
	case s.is(i, "define"):
		if s.globalCall(i) {
			s.defineCall(i)
		}
	case s.is(i, "class_alias"):
		if s.globalCall(i) {
			if args := s.arguments(i + 1); len(args) >= 2 {
				s.reservedStringName(args[1], "class alias")
			}
		}
	case tok.Value == "SORT_LOCALE_STRING":
		if !s.afterMember(i) &&
			s.kind(i+1) != parser.TokenOpenParen && s.kind(i-1) != parser.TokenFunction &&
			!s.is(i-1, "case") {
			s.report(i, i, "8.6", "SORT_LOCALE_STRING is deprecated since PHP 8.6, use one of Collator's sort methods instead")
		}
	}
}

// globalCall reports whether the name at i is called as a global function.
func (s *syntaxScan) globalCall(i int) bool {
	return s.kind(i+1) == parser.TokenOpenParen && !s.afterMember(i) &&
		s.kind(i-1) != parser.TokenFunction && s.kind(i-1) != parser.TokenNew
}

// argRange is the first and last token of one call argument.
type argRange struct{ from, to int }

// arguments splits the call arguments in the parentheses opening at open. It
// returns nil for a first-class callable or partial application, `f(...)`.
// The slice is valid until the next call.
func (s *syntaxScan) arguments(open int) []argRange {
	args := s.args[:0]
	depth, from := 0, open+1
	for j := open + 1; j < len(s.toks); j++ {
		switch s.kind(j) {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			if depth == 0 {
				if j > from {
					args = append(args, argRange{from, j - 1})
				}
				s.args = args
				return args
			}
			depth--
		case parser.TokenComma:
			if depth == 0 {
				args = append(args, argRange{from, j - 1})
				from = j + 1
			}
		case parser.TokenSemicolon:
			return nil
		}
	}
	return nil
}

func (s *syntaxScan) defineCall(i int) {
	args := s.arguments(i + 1)
	if len(args) == 0 {
		return
	}
	s.reservedStringName(args[0], "constant name")
	if a := args[0]; a.from == a.to && s.unquoted(a.from) == "_" {
		s.report(a.from, a.to, "8.6", `Using "_" as a constant name is deprecated since PHP 8.6`)
	}
	if len(args) >= 3 {
		s.report(args[2].from, args[2].to, "8.6", "define()'s third argument is deprecated since PHP 8.6: case-insensitive constants are no longer supported, so it is ignored; remove it")
	}
}

// reservedStringName reports a string literal argument naming `let` or `is`.
func (s *syntaxScan) reservedStringName(a argRange, what string) {
	if a.from != a.to {
		return
	}
	if lower := strings.ToLower(s.unquoted(a.from)); lower == "let" || lower == "is" {
		s.report(a.from, a.to, "8.6", reservedNameMessage(lower, what))
	}
}

// unquoted returns a plain string literal's value, or "" for anything else.
func (s *syntaxScan) unquoted(i int) string {
	if s.kind(i) != parser.TokenStringLiteral {
		return ""
	}
	v := s.toks[i].Value
	if len(v) < 2 || (v[0] != '\'' && v[0] != '"') || v[len(v)-1] != v[0] {
		return ""
	}
	return v[1 : len(v)-1]
}

// importNoun is what a `use` import of each kind names.
var importNoun = map[string]string{"class": "class name", "function": "function name", "const": "constant name"}

// reservedNameMessage says that `let` or `is` is used as what, such as
// "class name".
func reservedNameMessage(name, what string) string {
	return `Using "` + name + `" as ` + article(what) + " " + what + " is deprecated since PHP 8.6, it is reserved for a future keyword"
}

func article(word string) string {
	if word != "" && strings.ContainsRune("aeiou", rune(word[0])) {
		return "an"
	}
	return "a"
}
