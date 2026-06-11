package parser

import "strings"

// ---------------------------------------------------------------------------
// Match expression structures
// ---------------------------------------------------------------------------

// MatchDef represents a PHP 8.0 match expression found in a file.
// Only the structural information needed by downstream analysis is captured;
// full expression AST is deliberately out of scope.
type MatchDef struct {
	// SubjectText is the raw text of the subject expression, e.g. "$status".
	// Captured as-is from the token stream; may be empty on malformed input.
	SubjectText string
	// Arms are the match arms collected from the match body.
	Arms []MatchArm
	// Line is the 0-based line of the 'match' keyword.
	Line int
	// EndLine is the 0-based line of the closing brace (or the last token
	// consumed before a recovery bail-out on malformed input).
	EndLine int
}

// MatchArm represents one arm of a match expression:
//
//	cond1, cond2 => expr
//	default      => expr
type MatchArm struct {
	// Conditions holds the raw text of each comma-separated condition.
	// Empty (len == 0) when IsDefault is true.
	Conditions []string
	// IsDefault is true when the arm's only condition is the 'default' keyword.
	IsDefault bool
	// Line is the 0-based line where this arm starts.
	Line int
}

// ---------------------------------------------------------------------------
// Call-site structures
// ---------------------------------------------------------------------------

// maxCallSitesPerFile caps the number of CallSite records collected from a
// single file. This prevents unbounded memory growth in very large generated
// files (e.g. IDE helper files with thousands of delegating calls). Once the
// cap is hit the remainder of the file is still parsed for declarations; only
// new call sites stop being recorded.
const maxCallSitesPerFile = 5000

// CallSite represents one call expression encountered inside a function or
// method body. Only calls with recognisable callees are collected (plain
// identifiers, $var->method, Type::method, new Type). Exotic call targets
// (closures, array-derefs, etc.) are skipped silently to keep the collector
// simple and fast.
type CallSite struct {
	// CalleeText is the raw text of the callee as it appears in source, e.g.
	// "doSomething", "$this->logger->info", "Cache::get", "new Request".
	CalleeText string
	// Args is the argument list parsed from the call. Each element is one
	// positional or named argument.
	Args []CallArg
	// IsFirstClassCallable is true when the argument list consists solely of
	// the literal "..." token — i.e. foo(...) syntax introduced in PHP 8.1.
	// In that case Args is empty and the call site must not be mistaken for a
	// variadic call or a zero-argument call.
	IsFirstClassCallable bool
	// Line is the 0-based line of the opening parenthesis of the argument list.
	Line int
}

// CallArg represents one argument in a call site.
type CallArg struct {
	// Name is the named-argument label (e.g. "timeout" for timeout: 30).
	// Empty for positional arguments.
	Name string
	// ValueText is the raw source text of the argument value expression.
	// For a spread argument such as ...$items, ValueText is "$items" and
	// IsSpread is true.
	ValueText string
	// IsSpread is true when the argument is prefixed with "...".
	IsSpread bool
}

// ---------------------------------------------------------------------------
// ParseResult extension — Matches and Calls
// ---------------------------------------------------------------------------

// Matches holds all match expressions found anywhere in the file (including
// inside method and function bodies). Nested matches are collected individually;
// the nesting relationship is not preserved at this level.
//
// Access via ParseResult.Matches.
type matchList = []MatchDef // type alias keeps ParseResult fields compact

// callList is the per-file slice of collected call sites.
type callList = []CallSite

// ---------------------------------------------------------------------------
// Expression scanner — wired into parseStructure
// ---------------------------------------------------------------------------

// scanExpressions walks the already-tokenised token slice looking for match
// expressions and call sites. It is called once by parseStructure after the
// declaration-level pass has populated result.Tokens, and it appends its
// findings directly into result.Matches and result.Calls.
//
// The scanner is intentionally single-pass and stays in sync with the token
// stream so it never re-reads source. Strings and comments are already
// collapsed to single tokens by the tokeniser, so keyword matching inside
// them is not possible.
func scanExpressions(tokens []Token, result *ParseResult) {
	s := &exprScanner{tokens: tokens, result: result}
	s.scan()
}

type exprScanner struct {
	tokens []Token
	pos    int
	result *ParseResult
}

func (s *exprScanner) peek() Token {
	if s.pos >= len(s.tokens) {
		return Token{Kind: TokenEOF}
	}
	return s.tokens[s.pos]
}

func (s *exprScanner) advance() Token {
	t := s.peek()
	if s.pos < len(s.tokens) {
		s.pos++
	}
	return t
}

// peekAt returns the token at pos+offset without consuming anything.
func (s *exprScanner) peekAt(offset int) Token {
	idx := s.pos + offset
	if idx < 0 || idx >= len(s.tokens) {
		return Token{Kind: TokenEOF}
	}
	return s.tokens[idx]
}

// scan performs the main single-pass scan over all tokens.
func (s *exprScanner) scan() {
	for s.pos < len(s.tokens) {
		t := s.peek()
		switch {
		case t.Kind == TokenEOF:
			return

		// Skip string literals and comments entirely — they are already
		// collapsed by the tokeniser so no keyword inside them can match.
		case t.Kind == TokenStringLiteral, t.Kind == TokenComment, t.Kind == TokenDocComment:
			s.advance()

		// 'match' keyword (it is tokenised as TokenIdentifier since it isn't in
		// the keyword list — PHP 8 reserved words handled here).
		case t.Kind == TokenIdentifier && t.Value == "match":
			startPos := s.pos
			m, ok := s.parseMatchExpression()
			if ok {
				s.result.Matches = append(s.result.Matches, m)
			}
			// Progress guard: if parseMatchExpression didn't consume anything, skip.
			if s.pos == startPos {
				s.advance()
			}

		// Call sites: identifier/new followed by '('
		case s.isCallHead(t):
			startPos := s.pos
			s.tryCollectCallSite()
			// Progress guard.
			if s.pos == startPos {
				s.advance()
			}

		default:
			s.advance()
		}
	}
}

// isCallHead returns true when the current token can start a call-site
// expression that we want to collect. We restrict to:
//   - TokenIdentifier (plain function call: foo(...))
//   - TokenNew         (constructor call: new Foo(...))
//   - TokenVariable    (method call chain: $var->method(...) — detected via
//     lookahead in tryCollectCallSite)
//
// Note: $var->method and Type::method patterns are recognised inside
// tryCollectCallSite by looking ahead from the initial token.
func (s *exprScanner) isCallHead(t Token) bool {
	if t.Kind == TokenNew {
		return true
	}
	if t.Kind == TokenIdentifier || t.Kind == TokenVariable {
		// Look ahead to see if there's eventually a '(' forming a call.
		// We do a cheap scan up to a reasonable look-ahead distance.
		return s.hasCallAhead()
	}
	return false
}

// hasCallAhead does a bounded look-ahead from the current position to decide
// if the current expression head is eventually followed by '(' in a way that
// suggests a call (rather than a bare identifier or comparison). Returns true
// when the next meaningful token after the callee expression is '('.
func (s *exprScanner) hasCallAhead() bool {
	i := 1
	for i <= 8 { // bounded look-ahead
		tok := s.peekAt(i)
		switch tok.Kind {
		case TokenOpenParen:
			return true
		case TokenArrow, TokenDoubleColon:
			// $var->method( or Type::method( — advance past the accessor
			i++
			if s.peekAt(i).Kind == TokenIdentifier || isKeywordToken(s.peekAt(i).Kind) {
				i++
			}
		default:
			return false
		}
	}
	return false
}

// tryCollectCallSite attempts to collect one call site starting at the current
// position. On success the call site is appended to result.Calls (up to the
// cap). The position is advanced past the closing ')'.
func (s *exprScanner) tryCollectCallSite() {
	startPos := s.pos

	// Build the callee text.
	callee := s.readCalleeText()
	if callee == "" {
		// Could not form a callee — reset and let the main loop advance.
		s.pos = startPos
		return
	}

	// Expect '('
	if s.peek().Kind != TokenOpenParen {
		s.pos = startPos
		return
	}
	line := s.peek().Line
	s.advance() // consume '('

	// Check for first-class callable syntax: the only token before ')' is '...'
	fcc, args, ok := s.parseArgList()
	if !ok {
		// Malformed argument list — partial collection already stopped; continue.
		return
	}

	cs := CallSite{
		CalleeText:           callee,
		Args:                 args,
		IsFirstClassCallable: fcc,
		Line:                 line,
	}

	if len(s.result.Calls) < maxCallSitesPerFile {
		s.result.Calls = append(s.result.Calls, cs)
	}
}

// readCalleeText consumes tokens forming a callee expression and returns the
// raw joined text. Recognised patterns (starting from current position):
//
//   - identifier                → "foo"
//   - identifier::identifier    → "Foo::bar"
//   - $var->identifier          → "$this->method"
//   - $var->identifier->identifier (chains up to 4 deep)
//   - new identifier            → "new Foo"
//
// On failure (no recognisable pattern) returns "" and leaves pos unchanged.
func (s *exprScanner) readCalleeText() string {
	var parts []string
	t := s.peek()

	switch t.Kind {
	case TokenNew:
		s.advance()
		if s.peek().Kind != TokenIdentifier && s.peek().Kind != TokenBackslash {
			s.pos-- // undo
			return ""
		}
		parts = append(parts, "new", s.readTypeNameText())
		return strings.Join(parts, " ")

	case TokenIdentifier:
		parts = append(parts, t.Value)
		s.advance()
		// Check for ::
		if s.peek().Kind == TokenDoubleColon {
			s.advance()
			if s.peek().Kind == TokenIdentifier || isKeywordToken(s.peek().Kind) {
				parts = append(parts, "::", s.peek().Value)
				s.advance()
			} else {
				// Static call with non-identifier after :: — not a simple callee
				s.pos -= 2
				parts = parts[:len(parts)-1]
				if len(parts) == 0 {
					return ""
				}
			}
		}
		return strings.Join(parts, "")

	case TokenVariable:
		parts = append(parts, t.Value)
		s.advance()
		// Follow -> chains up to 4 levels deep
		for range [4]struct{}{} {
			if s.peek().Kind != TokenArrow && s.peek().Kind != TokenQuestion {
				break
			}
			// Handle nullsafe ?->
			if s.peek().Kind == TokenQuestion {
				if s.peekAt(1).Kind != TokenArrow {
					break
				}
				parts = append(parts, "?->")
				s.advance() // ?
				s.advance() // ->
			} else {
				parts = append(parts, "->")
				s.advance() // ->
			}
			if s.peek().Kind == TokenIdentifier || isKeywordToken(s.peek().Kind) {
				parts = append(parts, s.peek().Value)
				s.advance()
			} else {
				// broken chain
				break
			}
		}
		return strings.Join(parts, "")

	default:
		return ""
	}
}

// readTypeNameText consumes a qualified type name (Foo or Foo\Bar\Baz) and
// returns the joined text.
func (s *exprScanner) readTypeNameText() string {
	var parts []string
	for s.peek().Kind == TokenIdentifier || s.peek().Kind == TokenBackslash {
		parts = append(parts, s.peek().Value)
		s.advance()
	}
	return strings.Join(parts, "")
}

// isThreeDots returns true when the three tokens at pos+offset, pos+offset+1,
// pos+offset+2 are all TokenUnknown "." — i.e. the PHP "..." spread/FCC token.
// The tokeniser does not produce a single "..." token; it produces three
// consecutive TokenUnknown tokens each with value ".".
func (s *exprScanner) isThreeDots(offset int) bool {
	return s.peekAt(offset).Kind == TokenUnknown && s.peekAt(offset).Value == "." &&
		s.peekAt(offset+1).Kind == TokenUnknown && s.peekAt(offset+1).Value == "." &&
		s.peekAt(offset+2).Kind == TokenUnknown && s.peekAt(offset+2).Value == "."
}

// consumeThreeDots advances past three consecutive "." tokens.
func (s *exprScanner) consumeThreeDots() {
	s.advance()
	s.advance()
	s.advance()
}

// parseArgList parses the argument list of a call, starting just after the
// opening '(' has been consumed. Returns (isFirstClassCallable, args, ok).
// ok is false only when the token stream runs out entirely (EOF inside args);
// in that case we stop collecting and leave pos where it is.
func (s *exprScanner) parseArgList() (bool, []CallArg, bool) {
	// First-class callable: the only content is '...' followed immediately by ')'
	// The tokenizer produces three separate TokenUnknown "." tokens for "...".
	if s.isThreeDots(0) && s.peekAt(3).Kind == TokenCloseParen {
		s.consumeThreeDots() // consume '...'
		s.advance()          // consume ')'
		return true, nil, true
	}

	var args []CallArg
	for {
		if s.peek().Kind == TokenEOF {
			return false, args, false // truncated input
		}
		if s.peek().Kind == TokenCloseParen {
			s.advance() // consume ')'
			return false, args, true
		}
		// Trailing comma before ')'
		if s.peek().Kind == TokenComma {
			s.advance()
			continue
		}

		arg, ok := s.parseOneArg()
		if !ok {
			// Malformed arg — skip to next comma or ')' to recover.
			s.skipToArgBoundary()
			continue
		}
		args = append(args, arg)

		// After a valid arg expect ',' or ')'
		if s.peek().Kind == TokenComma {
			s.advance()
		} else if s.peek().Kind == TokenCloseParen {
			// will be consumed at next iteration
		} else if s.peek().Kind == TokenEOF {
			return false, args, false
		}
	}
}

// skipToArgBoundary skips tokens until a comma or ')' at depth 0, to recover
// from a malformed argument expression.
func (s *exprScanner) skipToArgBoundary() {
	depth := 0
	for s.peek().Kind != TokenEOF {
		switch s.peek().Kind {
		case TokenOpenParen, TokenOpenBracket, TokenOpenBrace:
			depth++
		case TokenCloseParen:
			if depth == 0 {
				return
			}
			depth--
		case TokenCloseBracket, TokenCloseBrace:
			if depth == 0 {
				return
			}
			depth--
		case TokenComma:
			if depth == 0 {
				return
			}
		}
		s.advance()
	}
}

// parseOneArg parses one argument. It recognises:
//   - named argument:  name: value  or  name: ...value
//   - spread:          ...value
//   - positional:      value
//
// The "value" is captured as a joined raw text of the tokens making up the
// expression, up to the next comma or ')' at depth 0. This is a best-effort
// representation — complex expressions (nested calls, closures, etc.) are
// captured verbatim token-joined rather than evaluated.
func (s *exprScanner) parseOneArg() (CallArg, bool) {
	if s.peek().Kind == TokenEOF || s.peek().Kind == TokenCloseParen {
		return CallArg{}, false
	}

	arg := CallArg{}

	// Named argument: <identifier> ':'  where ':' is NOT '::'
	if s.peek().Kind == TokenIdentifier {
		name := s.peek().Value
		// Peek ahead for a ':' that is NOT '::'
		if s.peekAt(1).Kind == TokenColon {
			arg.Name = name
			s.advance() // identifier
			s.advance() // :
		}
	}

	// Spread prefix: PHP "..." is three consecutive TokenUnknown "." tokens.
	if s.isThreeDots(0) {
		arg.IsSpread = true
		s.consumeThreeDots()
	}

	// Capture value text up to the next ',' or ')' at depth 0
	var valueParts []string
	depth := 0
	for s.peek().Kind != TokenEOF {
		t := s.peek()
		switch t.Kind {
		case TokenCloseParen:
			if depth == 0 {
				goto doneValue
			}
			depth--
			valueParts = append(valueParts, t.Value)
			s.advance()
		case TokenComma:
			if depth == 0 {
				goto doneValue
			}
			valueParts = append(valueParts, t.Value)
			s.advance()
		case TokenOpenParen, TokenOpenBracket, TokenOpenBrace:
			depth++
			valueParts = append(valueParts, t.Value)
			s.advance()
		case TokenCloseBracket, TokenCloseBrace:
			if depth > 0 {
				depth--
			}
			valueParts = append(valueParts, t.Value)
			s.advance()
		default:
			valueParts = append(valueParts, t.Value)
			s.advance()
		}
	}
doneValue:
	arg.ValueText = strings.Join(valueParts, "")
	return arg, true
}

// ---------------------------------------------------------------------------
// Match expression parser
// ---------------------------------------------------------------------------

// parseMatchExpression parses a match expression starting at the current
// position (the 'match' keyword token). Returns the MatchDef and true on
// success, or a partial MatchDef and false on fatal malformation (missing '('
// for the subject, or EOF inside the match body).
//
// Recovery strategy: if an arm is malformed we record a ParseError, skip
// tokens until the next ',' or '}' at depth 0, and continue. This ensures
// we never consume tokens past the closing '}' of the match expression itself,
// so the rest of the file parses normally.
func (s *exprScanner) parseMatchExpression() (MatchDef, bool) {
	m := MatchDef{}
	matchTok := s.peek()
	m.Line = matchTok.Line
	s.advance() // consume 'match'

	// Expect '(' for the subject
	if s.peek().Kind != TokenOpenParen {
		// 'match' used as a plain method/function name, not a match expression
		return m, false
	}
	s.advance() // consume '('

	// Collect subject text until the matching ')'
	m.SubjectText = s.collectUntilCloseParen()

	// Expect '{'
	if s.peek().Kind != TokenOpenBrace {
		return m, false
	}
	s.advance() // consume '{'

	// Parse arms until '}' or EOF
	for {
		startPos := s.pos

		if s.peek().Kind == TokenEOF {
			// Truncated input — partial result
			m.EndLine = s.peek().Line
			return m, false
		}
		if s.peek().Kind == TokenCloseBrace {
			m.EndLine = s.peek().Line
			s.advance() // consume '}'
			return m, true
		}

		arm, ok := s.parseMatchArm()
		if ok {
			m.Arms = append(m.Arms, arm)
		} else {
			// Recovery: skip to next ',' or '}' at match-body depth
			s.result.Errors = append(s.result.Errors, ParseError{
				Message: "malformed match arm",
				Line:    s.peek().Line,
				Column:  s.peek().Column,
			})
			s.skipMatchArmRecovery()
		}

		// Progress guard
		if s.pos == startPos {
			s.advance()
		}
	}
}

// collectUntilCloseParen collects all tokens between the already-consumed '('
// and the matching ')' and returns them as a space-joined string. The closing
// ')' is consumed.
func (s *exprScanner) collectUntilCloseParen() string {
	var parts []string
	depth := 0
	for s.peek().Kind != TokenEOF {
		t := s.peek()
		if t.Kind == TokenCloseParen && depth == 0 {
			s.advance() // consume ')'
			break
		}
		if t.Kind == TokenOpenParen {
			depth++
		} else if t.Kind == TokenCloseParen {
			depth--
		}
		parts = append(parts, t.Value)
		s.advance()
	}
	return strings.Join(parts, " ")
}

// parseMatchArm parses one match arm:
//
//	condition1, condition2 => expr,
//	default => expr,
//
// The arm may optionally end with a trailing ',' before '}'. Returns
// (arm, true) on success, (partial, false) on fatal failure.
func (s *exprScanner) parseMatchArm() (MatchArm, bool) {
	arm := MatchArm{Line: s.peek().Line}

	// Collect conditions separated by commas, terminated by '=>'
	for {
		if s.peek().Kind == TokenEOF {
			return arm, false
		}
		if s.peek().Kind == TokenCloseBrace {
			// Trailing comma case: we hit '}' before '=>'
			return arm, false
		}
		if s.peek().Kind == TokenDoubleArrow {
			break // done collecting conditions
		}

		// 'default' keyword is tokenised as TokenIdentifier
		if s.peek().Kind == TokenIdentifier && s.peek().Value == "default" {
			arm.IsDefault = true
			arm.Conditions = nil
			s.advance() // consume 'default'
			// Skip to '=>'
			for s.peek().Kind != TokenDoubleArrow && s.peek().Kind != TokenEOF && s.peek().Kind != TokenCloseBrace {
				s.advance()
			}
			break
		}

		// Collect condition expression up to ',' or '=>' at depth 0
		condText := s.collectMatchCondition()
		if condText != "" {
			arm.Conditions = append(arm.Conditions, condText)
		}

		// After condition: ',' means another condition; '=>' means start of body
		if s.peek().Kind == TokenComma {
			s.advance()
			continue
		}
		if s.peek().Kind == TokenDoubleArrow {
			break
		}
	}

	// Consume '=>'
	if s.peek().Kind != TokenDoubleArrow {
		return arm, false
	}
	s.advance() // consume '=>'

	// Skip the arm body expression until the next ',' or '}' at depth 0.
	// We do not capture the body value — only the conditions matter for our model.
	s.skipMatchArmBody()

	// Consume trailing comma if present
	if s.peek().Kind == TokenComma {
		s.advance()
	}

	return arm, true
}

// collectMatchCondition collects one match condition expression — tokens up to
// the first ',' or '=>' at depth 0. Returns the joined text.
func (s *exprScanner) collectMatchCondition() string {
	var parts []string
	depth := 0
	for s.peek().Kind != TokenEOF {
		t := s.peek()
		switch t.Kind {
		case TokenOpenParen, TokenOpenBracket, TokenOpenBrace:
			depth++
			parts = append(parts, t.Value)
			s.advance()
		case TokenCloseParen, TokenCloseBracket, TokenCloseBrace:
			if depth == 0 {
				return strings.Join(parts, " ")
			}
			depth--
			parts = append(parts, t.Value)
			s.advance()
		case TokenComma:
			if depth == 0 {
				return strings.Join(parts, " ")
			}
			parts = append(parts, t.Value)
			s.advance()
		case TokenDoubleArrow:
			if depth == 0 {
				return strings.Join(parts, " ")
			}
			parts = append(parts, t.Value)
			s.advance()
		default:
			parts = append(parts, t.Value)
			s.advance()
		}
	}
	return strings.Join(parts, " ")
}

// skipMatchArmBody skips tokens forming the arm result expression until the
// next ',' or '}' at depth 0. If a nested match expression is encountered
// during the skip, it is recursively parsed and appended to result.Matches.
func (s *exprScanner) skipMatchArmBody() {
	depth := 0
	for s.peek().Kind != TokenEOF {
		t := s.peek()
		switch {
		case t.Kind == TokenOpenParen || t.Kind == TokenOpenBracket || t.Kind == TokenOpenBrace:
			depth++
			s.advance()
		case t.Kind == TokenCloseParen || t.Kind == TokenCloseBracket:
			if depth == 0 {
				return
			}
			depth--
			s.advance()
		case t.Kind == TokenCloseBrace:
			if depth == 0 {
				return
			}
			depth--
			s.advance()
		case t.Kind == TokenComma:
			if depth == 0 {
				s.advance() // consume trailing comma
				return
			}
			s.advance()
		// Nested match expression — recursively parse and collect it.
		case t.Kind == TokenIdentifier && t.Value == "match" && depth >= 0:
			startPos := s.pos
			nested, ok := s.parseMatchExpression()
			if ok {
				s.result.Matches = append(s.result.Matches, nested)
			} else if s.pos == startPos {
				s.advance()
			}
		default:
			s.advance()
		}
	}
}

// skipMatchArmRecovery skips tokens until the next ',' at match-body depth
// (which indicates the start of the next arm condition) or the '}' that closes
// the match expression. This is used when an arm is malformed.
func (s *exprScanner) skipMatchArmRecovery() {
	depth := 0
	for s.peek().Kind != TokenEOF {
		t := s.peek()
		switch t.Kind {
		case TokenOpenParen, TokenOpenBracket, TokenOpenBrace:
			depth++
			s.advance()
		case TokenCloseParen, TokenCloseBracket:
			if depth == 0 {
				return
			}
			depth--
			s.advance()
		case TokenCloseBrace:
			if depth == 0 {
				// '}' closes the match — do NOT consume it; the main loop will see it.
				return
			}
			depth--
			s.advance()
		case TokenComma:
			if depth == 0 {
				s.advance() // consume ',' so next iteration sees the next arm
				return
			}
			s.advance()
		default:
			s.advance()
		}
	}
}
