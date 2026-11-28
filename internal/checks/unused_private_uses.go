package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type privateMemberUses struct {
	methods        map[string]bool
	methodPrefixes []string
	properties     map[string]bool
	constants      map[string]bool
	anyMethod      bool
	anyProperty    bool
	anyConstant    bool
}

func newPrivateMemberUses() *privateMemberUses {
	return &privateMemberUses{methods: map[string]bool{}, properties: map[string]bool{}, constants: map[string]bool{}}
}

func (u *privateMemberUses) usesMethod(name string) bool {
	lower := strings.ToLower(name)
	if u.anyMethod || u.methods[lower] {
		return true
	}
	for _, prefix := range u.methodPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func (u *privateMemberUses) usesProperty(name string) bool {
	return u.anyProperty || u.properties[name]
}

func (u *privateMemberUses) usesConstant(name string) bool {
	return u.anyConstant || u.constants[name]
}

type declarations bool

const (
	declarationsAreNotUses declarations = false
	declarationsAreUses    declarations = true
)

var reflectiveConstantReads = map[string]bool{
	"getconstants":            true,
	"getconstant":             true,
	"hasconstant":             true,
	"getreflectionconstants":  true,
	"getreflectionconstant":   true,
	"reflectionclassconstant": true,
}

var propertyListings = map[string]bool{
	"get_object_vars": true,
	"get_class_vars":  true,
}

var reflectionClasses = map[string]bool{
	"reflectionclass":  true,
	"reflectionobject": true,
}

var constantLookupFunctions = map[string]bool{
	"constant": true,
	"defined":  true,
}

type codeTokens struct {
	src    string
	tokens []lexer.Token
}

func (c codeTokens) at(i int) lexer.Token {
	if i >= 0 && i < len(c.tokens) {
		return c.tokens[i]
	}
	return lexer.Token{Kind: token.Invalid}
}

func (c codeTokens) kind(i int) token.Kind {
	return c.at(i).Kind
}

func (c codeTokens) text(i int) string {
	return c.at(i).Text(c.src)
}

func (c codeTokens) isName(i int) bool {
	return c.kind(i).IsSemiReserved()
}

func (c codeTokens) isThis(i int) bool {
	return c.kind(i) == token.Variable && c.text(i) == "$this"
}

// collect reads the uses in the tokens of class, or of a trait it uses. A
// dynamic call on the class's own methods through a variable, `$this->$m()`,
// is resolved from the variable's assignments once every token is read.
func (u *privateMemberUses) collect(code codeTokens, decl declarations, class string) {
	c := &useCollector{u: u, class: class, methodVars: map[string]bool{}}
	c.collect(code, decl)
	for name := range c.methodVars {
		u.collectMethodVariable(code, name)
	}
}

type useCollector struct {
	u          *privateMemberUses
	class      string
	methodVars map[string]bool
}

func (c *useCollector) collect(code codeTokens, decl declarations) {
	u := c.u
	for i, tok := range code.tokens {
		switch tok.Kind {
		case token.ConstantString:
			u.collectLiteral(code.text(i))
		case token.Include, token.IncludeOnce, token.Require, token.RequireOnce:
			u.anyMethod, u.anyProperty, u.anyConstant = true, true, true
		case token.Identifier, token.NameFullyQualified, token.NameQualified:
			u.collectCall(code, i)
		case token.Array:
			if code.kind(i+1) == token.LeftParen {
				c.collectCallable(code, i+1)
			}
		case token.ArrayCast:
			if code.isThis(i+1) && !startsMemberAccess(code, i+2) {
				u.anyProperty = true
			}
		case token.Foreach:
			if code.kind(i+1) == token.LeftParen && code.isThis(i+2) && code.kind(i+3) == token.As {
				u.anyProperty = true
			}
		case token.LeftBracket:
			c.collectCallable(code, i)
		case token.ObjectOperator, token.NullsafeObjectOperator:
			c.collectInstanceAccess(code, i+1)
		case token.DoubleColon:
			c.collectStaticAccess(code, i+1)
		case token.Function:
			if decl == declarationsAreUses && code.isName(i+1) {
				u.methods[strings.ToLower(code.text(i+1))] = true
			}
		}
	}
}

func (c *useCollector) collectInstanceAccess(code codeTokens, name int) {
	u := c.u
	if code.isName(name) {
		u.methods[strings.ToLower(code.text(name))] = true
		u.properties[code.text(name)] = true
		return
	}
	u.anyProperty = true
	if !ownReceiver(code, name-1) {
		return
	}
	switch code.kind(name) {
	case token.Variable:
		if code.kind(name+1) == token.LeftParen {
			c.dynamicMethod(code, name)
		}
	case token.LeftBrace:
		if code.kind(matchingBrace(code, name)+1) == token.LeftParen {
			c.dynamicMethod(code, name+1)
		}
	}
}

// ownReceiver reports whether the member access at code.tokens[arrow] is on
// $this itself.
func ownReceiver(code codeTokens, arrow int) bool {
	return code.isThis(arrow-1) && !isMemberAccess(code.kind(arrow-2))
}

func matchingBrace(code codeTokens, open int) int {
	depth := 0
	for i := open; i < len(code.tokens); i++ {
		switch code.kind(i) {
		case token.LeftBrace, token.CurlyOpen, token.DollarOpenCurlyBraces:
			depth++
		case token.RightBrace:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(code.tokens)
}

func (c *useCollector) collectStaticAccess(code codeTokens, name int) {
	u := c.u
	own := c.ownClassName(code, name-2)
	switch {
	case code.isName(name):
		u.methods[strings.ToLower(code.text(name))] = true
		u.constants[code.text(name)] = true
	case code.kind(name) == token.Variable && code.kind(name+1) == token.LeftParen:
		if own {
			c.dynamicMethod(code, name)
		}
	case code.kind(name) == token.Variable:
		u.properties[code.text(name)[1:]] = true
	default:
		u.anyProperty, u.anyConstant = true, true
		if own {
			u.anyMethod = true
		}
	}
}

// ownClassName reports whether code.tokens[i] names the class itself: self,
// static, $this or its own name, qualified or not.
func (c *useCollector) ownClassName(code codeTokens, i int) bool {
	switch code.kind(i) {
	case token.Static:
		return true
	case token.Variable:
		return code.isThis(i)
	case token.Identifier, token.NameQualified, token.NameFullyQualified:
		text := code.text(i)
		name := text[strings.LastIndexByte(text, '\\')+1:]
		return strings.EqualFold(name, "self") || strings.EqualFold(name, c.class)
	}
	return false
}

// dynamicMethod records a call on one of the class's own methods whose name
// starts at code.tokens[at]: a variable, a string with a literal prefix, or a
// concatenation that starts with one. Any other name may be any method.
func (c *useCollector) dynamicMethod(code codeTokens, at int) {
	if code.kind(at) == token.Variable && endsName(code, at+1) {
		c.methodVars[code.text(at)] = true
		return
	}
	if prefix, ok := literalPrefix(code, at); ok {
		c.u.methodPrefixes = append(c.u.methodPrefixes, strings.ToLower(prefix))
		return
	}
	c.u.anyMethod = true
}

// collectMethodVariable records what a variable used as a method name may
// hold: the literal prefix of every value assigned to it, or any method when
// one of them has none or it may get a value otherwise.
func (u *privateMemberUses) collectMethodVariable(code codeTokens, name string) {
	assigned := false
	for i, tok := range code.tokens {
		if tok.Kind != token.Variable || code.text(i) != name {
			continue
		}
		if boundOtherwise(code, i) {
			u.anyMethod = true
			return
		}
		if code.kind(i+1) != token.Equals {
			continue
		}
		prefix, ok := literalPrefix(code, i+2)
		if !ok {
			u.anyMethod = true
			return
		}
		u.methodPrefixes = append(u.methodPrefixes, strings.ToLower(prefix))
		assigned = true
	}
	if !assigned {
		u.anyMethod = true
	}
}

// boundOtherwise reports whether the variable at code.tokens[i] may get a
// value other than by a plain assignment: as a parameter, a foreach target,
// a reference, a global or static, or inside a destructuring.
func boundOtherwise(code codeTokens, i int) bool {
	switch code.kind(i - 1) {
	case token.Ampersand, token.AmpersandFollowedByVarOrVararg, token.AmpersandNotFollowedByVarOrVararg, token.As, token.Global, token.Static:
		return true
	}
	open := enclosingOpen(code, i)
	if open < 0 {
		return false
	}
	if code.kind(open) == token.LeftBracket {
		return code.kind(matchingClose(code, open)+1) == token.Equals
	}
	switch code.kind(open - 1) {
	case token.List, token.Function, token.Fn, token.Use:
		return true
	}
	return code.kind(open-2) == token.Function
}

// enclosingOpen returns the `(` or `[` that code.tokens[i] is inside, or -1.
func enclosingOpen(code codeTokens, i int) int {
	depth := 0
	for j := i - 1; j >= 0; j-- {
		switch code.kind(j) {
		case token.RightParen, token.RightBracket:
			depth++
		case token.LeftParen, token.LeftBracket:
			if depth == 0 {
				return j
			}
			depth--
		case token.Semicolon, token.LeftBrace, token.RightBrace:
			if depth == 0 {
				return -1
			}
		}
	}
	return -1
}

func matchingClose(code codeTokens, open int) int {
	depth := 0
	for i := open; i < len(code.tokens); i++ {
		switch code.kind(i) {
		case token.LeftParen, token.LeftBracket:
			depth++
		case token.RightParen, token.RightBracket:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(code.tokens)
}

// literalPrefix returns the literal start of the method name that starts at
// code.tokens[at]: a whole literal, the literal before `.`, or the text
// before the first variable of an interpolated string. ok is false when the
// name doesn't start with a literal.
func literalPrefix(code codeTokens, at int) (string, bool) {
	switch code.kind(at) {
	case token.ConstantString:
		value, ok := plainStringLiteral(code.text(at))
		if !ok || value == "" || !endsName(code, at+1) && code.kind(at+1) != token.Dot {
			return "", false
		}
		return value, true
	case token.DoubleQuote:
		if code.kind(at+1) != token.EncapsedAndWhitespace {
			return "", false
		}
		prefix := code.text(at + 1)
		if prefix == "" || strings.ContainsRune(prefix, '\\') {
			return "", false
		}
		return prefix, true
	}
	return "", false
}

// endsName reports whether code.tokens[at] closes a call's name, its
// argument or the statement.
func endsName(code codeTokens, at int) bool {
	switch code.kind(at) {
	case token.LeftParen, token.RightBrace, token.RightBracket, token.RightParen, token.Comma, token.Semicolon:
		return true
	}
	return false
}

func (u *privateMemberUses) collectLiteral(lit string) {
	value, ok := plainStringLiteral(lit)
	if !ok {
		return
	}
	if i := strings.LastIndex(value, "::"); i >= 0 {
		value = value[i+2:]
	}
	if !isIdentifierName(value) {
		return
	}
	u.methods[strings.ToLower(value)] = true
}

func isIdentifierName(s string) bool {
	if s == "" || (s[0] >= '0' && s[0] <= '9') {
		return false
	}
	for i := 0; i < len(s); i++ {
		if !parser.IsIdentByte(s[i]) {
			return false
		}
	}
	return true
}

func (u *privateMemberUses) collectCall(code codeTokens, i int) {
	text := code.text(i)
	name := strings.ToLower(text[strings.LastIndexByte(text, '\\')+1:])
	switch {
	case name == "property_exists":
		u.collectPropertyExists(code, i+1)
	case propertyListings[name]:
		if ownClassArgument(code, i+1) {
			u.anyProperty = true
		}
	case name == "get_class_methods":
		if ownClassArgument(code, i+1) {
			u.anyMethod = true
		}
	case reflectionClasses[name]:
		if code.kind(i-1) == token.New && ownClassArgument(code, i+1) {
			u.anyMethod, u.anyProperty, u.anyConstant = true, true, true
		}
	case reflectiveConstantReads[name]:
		u.anyConstant = true
	case constantLookupFunctions[name]:
		constant, lookup := constantLookupArgument(code, i+1)
		switch lookup {
		case lookupComputed:
			u.anyConstant = true
		case lookupLiteral:
			if _, member, ok := strings.Cut(constant, "::"); ok {
				u.constants[member] = true
			}
		}
	}
}

func (u *privateMemberUses) collectPropertyExists(code codeTokens, open int) {
	if code.kind(open) != token.LeftParen {
		return
	}
	depth := 0
	for i := open + 1; i < len(code.tokens); i++ {
		switch code.kind(i) {
		case token.LeftParen, token.LeftBracket, token.LeftBrace:
			depth++
		case token.RightParen, token.RightBracket, token.RightBrace:
			if depth == 0 {
				return
			}
			depth--
		case token.Comma:
			if depth > 0 {
				continue
			}
			value, ok := plainStringLiteral(code.text(i + 1))
			if after := code.kind(i + 2); code.kind(i+1) != token.ConstantString || !ok || (after != token.RightParen && after != token.Comma) {
				u.anyProperty = true
				return
			}
			u.properties[value] = true
			return
		}
	}
}

func (c *useCollector) collectCallable(code codeTokens, open int) {
	target := open + 1
	if kind := code.kind(target); kind != token.ClassConst && kind != token.Static && (code.isName(target) || kind == token.NameQualified || kind == token.NameFullyQualified) {
		if !c.ownClassName(code, target) {
			return
		}
	}
	comma := classReferenceEnd(code, target)
	if code.kind(comma) != token.Comma {
		return
	}
	method := comma + 1
	if _, ok := plainStringLiteral(code.text(method)); ok && code.kind(method) == token.ConstantString && endsName(code, method+1) {
		return
	}
	c.dynamicMethod(code, method)
}

func classReferenceEnd(code codeTokens, target int) int {
	switch kind := code.kind(target); {
	case code.isThis(target), kind == token.ClassConst:
		return target + 1
	case !code.isName(target) && kind != token.NameQualified && kind != token.NameFullyQualified && kind != token.NameRelative:
		return len(code.tokens)
	}
	if code.kind(target+1) != token.DoubleColon || code.kind(target+2) != token.Class {
		return len(code.tokens)
	}
	return target + 3
}

func ownClassReferenceEnd(code codeTokens, target int) int {
	if code.kind(target) == token.Identifier && !strings.EqualFold(code.text(target), "self") {
		return len(code.tokens)
	}
	if kind := code.kind(target); kind != token.Identifier && kind != token.Static && kind != token.Variable && kind != token.ClassConst {
		return len(code.tokens)
	}
	return classReferenceEnd(code, target)
}

func ownClassArgument(code codeTokens, open int) bool {
	return code.kind(open) == token.LeftParen && code.kind(ownClassReferenceEnd(code, open+1)) == token.RightParen
}

func startsMemberAccess(code codeTokens, at int) bool {
	kind := code.kind(at)
	return isMemberAccess(kind) || kind == token.LeftBracket
}

type constantLookup int

const (
	lookupNotACall constantLookup = iota
	lookupComputed
	lookupLiteral
)

func constantLookupArgument(code codeTokens, open int) (string, constantLookup) {
	if code.kind(open) != token.LeftParen {
		return "", lookupNotACall
	}
	value, ok := plainStringLiteral(code.text(open + 1))
	if code.kind(open+1) != token.ConstantString || !ok {
		return "", lookupComputed
	}
	if after := code.kind(open + 2); after != token.RightParen && after != token.Comma {
		return "", lookupComputed
	}
	return value, lookupLiteral
}

func plainStringLiteral(lit string) (string, bool) {
	if len(lit) < 2 {
		return "", false
	}
	quote := lit[0]
	if (quote != '\'' && quote != '"') || lit[len(lit)-1] != quote {
		return "", false
	}
	body := lit[1 : len(lit)-1]
	if strings.ContainsAny(body, "\\$") {
		return "", false
	}
	return body, true
}
