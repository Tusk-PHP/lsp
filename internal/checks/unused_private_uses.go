package checks

import (
	"cmp"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

type privateMemberUses struct {
	methods     map[string]bool
	properties  map[string]bool
	constants   map[string]bool
	anyMethod   bool
	anyProperty bool
	anyConstant bool
}

func newPrivateMemberUses() *privateMemberUses {
	return &privateMemberUses{methods: map[string]bool{}, properties: map[string]bool{}, constants: map[string]bool{}}
}

func (u *privateMemberUses) usesMethod(name string) bool {
	return u.anyMethod || u.methods[strings.ToLower(name)]
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

func (u *privateMemberUses) collect(tokens []parser.Token, decl declarations) {
	for i, tok := range tokens {
		switch tok.Kind {
		case parser.TokenStringLiteral:
			u.collectLiteral(tok.Value)
			u.collect(parser.InterpolatedTokens(tok), decl)
		case parser.TokenIdentifier:
			u.collectCall(tokens, i)
		case parser.TokenOpenBracket:
			u.collectCallable(tokens, i)
		case parser.TokenArrow:
			u.collectInstanceAccess(significantAt(tokens, i+1))
		case parser.TokenDoubleColon:
			u.collectStaticAccess(tokens, i)
		case parser.TokenFunction:
			if name := significantAt(tokens, i+1); decl == declarationsAreUses && isWordToken(name) {
				u.methods[strings.ToLower(name.Value)] = true
			}
		}
	}
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

func (u *privateMemberUses) collectInstanceAccess(name parser.Token) {
	if !isWordToken(name) {
		u.anyMethod, u.anyProperty = true, true
		return
	}
	u.methods[strings.ToLower(name.Value)] = true
	u.properties[name.Value] = true
}

func (u *privateMemberUses) collectStaticAccess(tokens []parser.Token, colons int) {
	at := nextSignificant(tokens, colons+1)
	name := tokenAt(tokens, at)
	switch {
	case isWordToken(name):
		u.methods[strings.ToLower(name.Value)] = true
		u.constants[name.Value] = true
	case name.Kind == parser.TokenVariable && significantAt(tokens, at+1).Kind == parser.TokenOpenParen:
		u.anyMethod = true
	case name.Kind == parser.TokenVariable:
		u.properties[name.Value[1:]] = true
	default:
		u.anyMethod, u.anyProperty, u.anyConstant = true, true, true
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

func (u *privateMemberUses) collectCall(tokens []parser.Token, i int) {
	name := strings.ToLower(tokens[i].Value[strings.LastIndexByte(tokens[i].Value, '\\')+1:])
	switch {
	case name == "array":
		if open := nextSignificant(tokens, i+1); tokenAt(tokens, open).Kind == parser.TokenOpenParen {
			u.collectCallable(tokens, open)
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

func (u *privateMemberUses) collectCallable(tokens []parser.Token, open int) {
	comma := classReferenceEnd(tokens, nextSignificant(tokens, open+1))
	if tokenAt(tokens, comma).Kind != parser.TokenComma {
		return
	}
	method := nextSignificant(tokens, comma+1)
	if _, ok := plainStringLiteral(tokenAt(tokens, method).Value); !ok {
		u.anyMethod = true
		return
	}
	switch significantAt(tokens, method+1).Kind {
	case parser.TokenCloseBracket, parser.TokenCloseParen, parser.TokenComma:
	default:
		u.anyMethod = true
	}
}

func classReferenceEnd(tokens []parser.Token, target int) int {
	tok := tokenAt(tokens, target)
	switch {
	case tok.Kind == parser.TokenVariable && tok.Value == "$this", tok.Value == "__CLASS__":
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
