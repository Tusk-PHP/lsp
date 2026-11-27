package checks

import (
	"cmp"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
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

var includes = map[string]bool{
	"include":      true,
	"include_once": true,
	"require":      true,
	"require_once": true,
}

var constantLookupFunctions = map[string]bool{
	"constant": true,
	"defined":  true,
}

func classTokens(tokens []parser.Token, startLine, endLine int) []parser.Token {
	byLine := func(t parser.Token, line int) int { return cmp.Compare(t.Line, line) }
	first, _ := slices.BinarySearchFunc(tokens, startLine, byLine)
	last, _ := slices.BinarySearchFunc(tokens, endLine+1, byLine)
	return tokens[first:last]
}

// collect reads the uses in the tokens of class, or of a trait it uses.
func (u *privateMemberUses) collect(tokens []parser.Token, decl declarations, class string) {
	c := &useCollector{u: u, scope: tokens, class: class, methodVars: map[string]bool{}}
	c.collect(tokens, decl)
	for name := range c.methodVars {
		u.collectMethodVariable(tokens, name)
	}
}

// useCollector reads one class's or trait's tokens. A dynamic call on the
// class's own methods through a variable, `$this->$m()`, is resolved from the
// variable's assignments in scope once every token is read.
type useCollector struct {
	u          *privateMemberUses
	scope      []parser.Token
	class      string
	methodVars map[string]bool
}

func (c *useCollector) collect(tokens []parser.Token, decl declarations) {
	u := c.u
	for i, tok := range tokens {
		switch tok.Kind {
		case parser.TokenStringLiteral:
			u.collectLiteral(tok.Value)
			c.collect(parser.InterpolatedTokens(tok), decl)
		case parser.TokenIdentifier:
			c.collectCall(tokens, i)
		case parser.TokenOpenBracket:
			c.collectCallable(tokens, i)
		case parser.TokenArrow:
			c.collectInstanceAccess(tokens, i)
		case parser.TokenDoubleColon:
			c.collectStaticAccess(tokens, i)
		case parser.TokenFunction:
			if name := significantAt(tokens, i+1); decl == declarationsAreUses && isWordToken(name) {
				u.methods[strings.ToLower(name.Value)] = true
			}
		}
	}
}

// dynamicMethod records a call on one of the class's own methods whose name
// starts at tokens[at]: a variable, a string with a literal prefix, or a
// concatenation that starts with one. Any other name may be any method.
func (c *useCollector) dynamicMethod(tokens []parser.Token, at int) {
	tok := tokenAt(tokens, at)
	if tok.Kind == parser.TokenVariable && endsName(tokens, at) {
		c.methodVars[tok.Value] = true
		return
	}
	if prefix, ok := literalPrefix(tokens, at); ok {
		c.u.methodPrefixes = append(c.u.methodPrefixes, strings.ToLower(prefix))
		return
	}
	c.u.anyMethod = true
}

// isMemberName reports whether the name at tokens[i] follows `->`, `::` or
// `function`, so it names a member rather than a language construct.
func isMemberName(tokens []parser.Token, i int) bool {
	prev := parser.PrevSignificant(tokens, i-1)
	if prev < 0 {
		return false
	}
	switch tokens[prev].Kind {
	case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenFunction:
		return true
	}
	return false
}

// collectMethodVariable records what a variable used as a method name may
// hold: the literal prefix of every value assigned to it, or any method when
// one of them has none or it's never assigned.
func (u *privateMemberUses) collectMethodVariable(tokens []parser.Token, name string) {
	assigned := false
	for i, tok := range tokens {
		if tok.Kind != parser.TokenVariable || tok.Value != name {
			continue
		}
		if boundOtherwise(tokens, i) {
			u.anyMethod = true
			return
		}
		if tokenAt(tokens, i+1).Kind != parser.TokenEquals || tokenAt(tokens, i+2).Kind == parser.TokenEquals {
			continue
		}
		prefix, ok := literalPrefix(tokens, nextSignificant(tokens, i+2))
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

// boundOtherwise reports whether the variable at tokens[i] may get a value
// other than by a plain assignment: as a parameter, a foreach target, a
// reference, a global or static, or inside a destructuring.
func boundOtherwise(tokens []parser.Token, i int) bool {
	prev := parser.PrevSignificant(tokens, i-1)
	if prev >= 0 {
		p := tokens[prev]
		if p.Kind == parser.TokenAmpersand || p.Kind == parser.TokenStatic || (p.Kind == parser.TokenIdentifier && (strings.EqualFold(p.Value, "as") || strings.EqualFold(p.Value, "global"))) {
			return true
		}
	}
	open := enclosingOpen(tokens, i)
	if open < 0 {
		return false
	}
	switch tokens[open].Kind {
	case parser.TokenOpenBracket:
		return significantAt(tokens, matchingClose(tokens, open)+1).Kind == parser.TokenEquals
	case parser.TokenOpenParen:
		before := parser.PrevSignificant(tokens, open-1)
		if before < 0 {
			return false
		}
		if strings.EqualFold(tokens[before].Value, "list") {
			return true
		}
		if tokens[before].Kind == parser.TokenFunction || strings.EqualFold(tokens[before].Value, "fn") || tokens[before].Kind == parser.TokenUse {
			return true
		}
		fn := parser.PrevSignificant(tokens, before-1)
		return fn >= 0 && tokens[fn].Kind == parser.TokenFunction
	}
	return false
}

// enclosingOpen returns the `(` or `[` that the token at tokens[i] is inside,
// or -1 when it isn't inside either.
func enclosingOpen(tokens []parser.Token, i int) int {
	depth := 0
	for j := i - 1; j >= 0; j-- {
		switch tokens[j].Kind {
		case parser.TokenCloseParen, parser.TokenCloseBracket:
			depth++
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			if depth == 0 {
				return j
			}
			depth--
		case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
			if depth == 0 {
				return -1
			}
		}
	}
	return -1
}

func matchingClose(tokens []parser.Token, open int) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(tokens)
}

// literalPrefix returns the literal start of the method name that starts at
// tokens[at]: a whole literal, the literal before `.`, or the text before the
// first variable of an interpolated string. ok is false when the name doesn't
// start with a literal.
func literalPrefix(tokens []parser.Token, at int) (string, bool) {
	tok := tokenAt(tokens, at)
	if tok.Kind != parser.TokenStringLiteral {
		return "", false
	}
	if value, ok := plainStringLiteral(tok.Value); ok {
		if endsName(tokens, at) || significantAt(tokens, at+1).Value == "." {
			return value, value != ""
		}
		return "", false
	}
	if !endsName(tokens, at) || len(tok.Value) < 2 || tok.Value[0] != '"' {
		return "", false
	}
	body := tok.Value[1 : len(tok.Value)-1]
	end := strings.IndexByte(body, '$')
	if end <= 0 || strings.ContainsRune(body[:end], '\\') {
		return "", false
	}
	return strings.TrimSuffix(body[:end], "{"), true
}

// endsName reports whether the name at tokens[at] is all of it: what follows
// closes the call's name, its argument or the statement.
func endsName(tokens []parser.Token, at int) bool {
	switch significantAt(tokens, at+1).Kind {
	case parser.TokenOpenParen, parser.TokenCloseBrace, parser.TokenCloseBracket, parser.TokenCloseParen, parser.TokenComma, parser.TokenSemicolon:
		return true
	}
	return false
}

// ownReceiver reports whether the access at tokens[arrow] is on $this.
func ownReceiver(tokens []parser.Token, arrow int) bool {
	before := parser.PrevSignificant(tokens, arrow-1)
	if before >= 0 && tokens[before].Kind == parser.TokenQuestion {
		before = parser.PrevSignificant(tokens, before-1)
	}
	if before < 0 || !isThis(tokens[before]) {
		return false
	}
	prev := parser.PrevSignificant(tokens, before-1)
	return prev < 0 || (tokens[prev].Kind != parser.TokenArrow && tokens[prev].Kind != parser.TokenDoubleColon)
}

// ownClassName reports whether tok names the class itself: self, static or
// its own name.
func (c *useCollector) ownClassName(tok parser.Token) bool {
	if !isWordToken(tok) {
		return false
	}
	name := tok.Value[strings.LastIndexByte(tok.Value, '\\')+1:]
	return strings.EqualFold(name, "self") || strings.EqualFold(name, "static") || strings.EqualFold(name, c.class)
}

func nextSignificant(tokens []parser.Token, from int) int {
	if i := parser.NextSignificant(tokens, from); i >= 0 {
		return i
	}
	return len(tokens)
}

func tokenAt(tokens []parser.Token, i int) parser.Token {
	if i < len(tokens) {
		return tokens[i]
	}
	return parser.Token{Kind: parser.TokenEOF}
}

func significantAt(tokens []parser.Token, from int) parser.Token {
	return tokenAt(tokens, nextSignificant(tokens, from))
}

func isWordToken(tok parser.Token) bool {
	if tok.Kind == parser.TokenVariable || tok.Kind == parser.TokenNumber || tok.Value == "" {
		return false
	}
	return parser.IsIdentByte(tok.Value[0])
}

func (c *useCollector) collectInstanceAccess(tokens []parser.Token, arrow int) {
	u := c.u
	at := nextSignificant(tokens, arrow+1)
	name := tokenAt(tokens, at)
	if isWordToken(name) {
		u.methods[strings.ToLower(name.Value)] = true
		u.properties[name.Value] = true
		return
	}
	u.anyProperty = true
	if !ownReceiver(tokens, arrow) {
		return
	}
	switch {
	case name.Kind == parser.TokenVariable && significantAt(tokens, at+1).Kind == parser.TokenOpenParen:
		c.dynamicMethod(tokens, at)
	case name.Kind == parser.TokenOpenBrace:
		closing := matchingBrace(tokens, at)
		if significantAt(tokens, closing+1).Kind == parser.TokenOpenParen {
			c.dynamicMethod(tokens, nextSignificant(tokens, at+1))
		}
	}
}

func matchingBrace(tokens []parser.Token, open int) int {
	depth := 0
	for i := open; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseBrace:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return len(tokens)
}

func (c *useCollector) collectStaticAccess(tokens []parser.Token, colons int) {
	u := c.u
	at := nextSignificant(tokens, colons+1)
	name := tokenAt(tokens, at)
	own := false
	if class := parser.PrevSignificant(tokens, colons-1); class >= 0 {
		own = c.ownClassName(tokens[class])
	}
	switch {
	case isWordToken(name):
		u.methods[strings.ToLower(name.Value)] = true
		u.constants[name.Value] = true
	case name.Kind == parser.TokenVariable && significantAt(tokens, at+1).Kind == parser.TokenOpenParen:
		if own {
			c.dynamicMethod(tokens, at)
		}
	case name.Kind == parser.TokenVariable:
		u.properties[name.Value[1:]] = true
	default:
		u.anyProperty, u.anyConstant = true, true
		if own {
			u.anyMethod = true
		}
	}
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

func (c *useCollector) collectCall(tokens []parser.Token, i int) {
	u := c.u
	name := strings.ToLower(tokens[i].Value[strings.LastIndexByte(tokens[i].Value, '\\')+1:])
	switch {
	case includes[name] && !isMemberName(tokens, i):
		u.anyMethod, u.anyProperty, u.anyConstant = true, true, true
	case name == "array":
		if open := nextSignificant(tokens, i+1); tokenAt(tokens, open).Kind == parser.TokenOpenParen {
			c.collectCallable(tokens, open)
		} else if isArrayCastOfThis(tokens, i) {
			u.anyProperty = true
		}
	case name == "foreach":
		if isForeachOverThis(tokens, i) {
			u.anyProperty = true
		}
	case propertyListings[name]:
		if ownClassArgument(tokens, i+1) {
			u.anyProperty = true
		}
	case name == "get_class_methods":
		if ownClassArgument(tokens, i+1) {
			u.anyMethod = true
		}
	case reflectionClasses[name]:
		if isInstantiation(tokens, i) && ownClassArgument(tokens, i+1) {
			u.anyMethod, u.anyProperty, u.anyConstant = true, true, true
		}
	case name == "property_exists":
		u.collectPropertyExists(tokens, nextSignificant(tokens, i+1))
	case reflectiveConstantReads[name]:
		u.anyConstant = true
	case constantLookupFunctions[name]:
		constant, lookup := constantLookupArgument(tokens, i+1)
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

func (u *privateMemberUses) collectPropertyExists(tokens []parser.Token, open int) {
	if tokenAt(tokens, open).Kind != parser.TokenOpenParen {
		return
	}
	depth := 0
	for i := open + 1; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			if depth == 0 {
				return
			}
			depth--
		case parser.TokenComma:
			if depth > 0 {
				continue
			}
			name := nextSignificant(tokens, i+1)
			value, ok := plainStringLiteral(tokenAt(tokens, name).Value)
			if after := significantAt(tokens, name+1).Kind; !ok || (after != parser.TokenCloseParen && after != parser.TokenComma) {
				u.anyProperty = true
				return
			}
			u.properties[value] = true
			return
		}
	}
}

func (c *useCollector) collectCallable(tokens []parser.Token, open int) {
	target := nextSignificant(tokens, open+1)
	comma := c.ownClassReferenceEnd(tokens, target)
	if tokenAt(tokens, comma).Kind != parser.TokenComma {
		return
	}
	method := nextSignificant(tokens, comma+1)
	if _, ok := plainStringLiteral(tokenAt(tokens, method).Value); ok && endsName(tokens, method) {
		return
	}
	c.dynamicMethod(tokens, method)
}

// ownClassReferenceEnd returns where a reference to the class itself that
// starts at tokens[target] ends: $this, __CLASS__, or self::class,
// static::class or its own name with ::class. It's len(tokens) for anything
// else.
func (c *useCollector) ownClassReferenceEnd(tokens []parser.Token, target int) int {
	tok := tokenAt(tokens, target)
	if isWordToken(tok) && tok.Value != "__CLASS__" && !c.ownClassName(tok) {
		return len(tokens)
	}
	return classReferenceEnd(tokens, target)
}

func classReferenceEnd(tokens []parser.Token, target int) int {
	tok := tokenAt(tokens, target)
	switch {
	case isThis(tok), tok.Value == "__CLASS__":
		return nextSignificant(tokens, target+1)
	case !isWordToken(tok):
		return len(tokens)
	}
	colons := nextSignificant(tokens, target+1)
	class := nextSignificant(tokens, colons+1)
	if tokenAt(tokens, colons).Kind != parser.TokenDoubleColon || tokenAt(tokens, class).Kind != parser.TokenClass {
		return len(tokens)
	}
	return nextSignificant(tokens, class+1)
}

func ownClassReferenceEnd(tokens []parser.Token, target int) int {
	tok := tokenAt(tokens, target)
	if isWordToken(tok) && !strings.EqualFold(tok.Value, "self") && !strings.EqualFold(tok.Value, "static") && tok.Value != "__CLASS__" {
		return len(tokens)
	}
	return classReferenceEnd(tokens, target)
}

func ownClassArgument(tokens []parser.Token, from int) bool {
	open := nextSignificant(tokens, from)
	if tokenAt(tokens, open).Kind != parser.TokenOpenParen {
		return false
	}
	return tokenAt(tokens, ownClassReferenceEnd(tokens, nextSignificant(tokens, open+1))).Kind == parser.TokenCloseParen
}

func isInstantiation(tokens []parser.Token, class int) bool {
	before := parser.PrevSignificant(tokens, class-1)
	if before >= 0 && tokens[before].Kind == parser.TokenBackslash {
		before = parser.PrevSignificant(tokens, before-1)
	}
	return before >= 0 && tokens[before].Kind == parser.TokenNew
}

func isArrayCastOfThis(tokens []parser.Token, array int) bool {
	open := parser.PrevSignificant(tokens, array-1)
	if open < 0 || tokens[open].Kind != parser.TokenOpenParen {
		return false
	}
	closing := nextSignificant(tokens, array+1)
	if tokenAt(tokens, closing).Kind != parser.TokenCloseParen {
		return false
	}
	this := nextSignificant(tokens, closing+1)
	return isThis(tokenAt(tokens, this)) && !startsMemberAccess(tokens, nextSignificant(tokens, this+1))
}

func isForeachOverThis(tokens []parser.Token, foreach int) bool {
	open := nextSignificant(tokens, foreach+1)
	if tokenAt(tokens, open).Kind != parser.TokenOpenParen {
		return false
	}
	this := nextSignificant(tokens, open+1)
	as := significantAt(tokens, this+1)
	return isThis(tokenAt(tokens, this)) && as.Kind == parser.TokenIdentifier && strings.EqualFold(as.Value, "as")
}

func isThis(tok parser.Token) bool {
	return tok.Kind == parser.TokenVariable && tok.Value == "$this"
}

func startsMemberAccess(tokens []parser.Token, at int) bool {
	switch tokenAt(tokens, at).Kind {
	case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenOpenBracket:
		return true
	case parser.TokenQuestion:
		return significantAt(tokens, at+1).Kind == parser.TokenArrow
	}
	return false
}

type constantLookup int

const (
	lookupNotACall constantLookup = iota
	lookupComputed
	lookupLiteral
)

func constantLookupArgument(tokens []parser.Token, from int) (string, constantLookup) {
	open := nextSignificant(tokens, from)
	if tokenAt(tokens, open).Kind != parser.TokenOpenParen {
		return "", lookupNotACall
	}
	arg := nextSignificant(tokens, open+1)
	value, ok := plainStringLiteral(tokenAt(tokens, arg).Value)
	if !ok {
		return "", lookupComputed
	}
	if after := significantAt(tokens, arg+1).Kind; after != parser.TokenCloseParen && after != parser.TokenComma {
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
