package lexer

import (
	"errors"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type Token struct {
	Kind  token.Kind
	Start uint32
	End   uint32
}

func (t Token) Text(src string) string {
	return src[t.Start:t.End]
}

const haltCompilerTokens = 3

type lexer struct {
	version    token.Version
	src        string
	pos        int
	tokens     []Token
	afterArrow bool
	haltLeft   int
	halted     bool
}

func Lex(src string, version token.Version) []Token {
	return newLexer(src, version).run()
}

func LexCode(src string, version token.Version) []Token {
	l := newLexer(src, version)
	l.scripting(false)
	return l.run()
}

func newLexer(src string, version token.Version) *lexer {
	return &lexer{version: version, src: src, tokens: make([]Token, 0, len(src)/5+1)}
}

func (l *lexer) run() []Token {
	for l.pos < len(l.src) {
		l.html()
		l.scripting(false)
	}
	return l.tokens
}

func (l *lexer) emit(kind token.Kind, end int) {
	if l.halted {
		return
	}
	l.tokens = append(l.tokens, Token{Kind: kind, Start: uint32(l.pos), End: uint32(end)})
	l.pos = end
	if l.haltLeft == 0 || kind.IsTrivia() || kind == token.OpenTag {
		return
	}
	l.haltLeft--
	if l.haltLeft > 0 {
		return
	}
	if end < len(l.src) {
		l.tokens = append(l.tokens, Token{Kind: token.InlineHTML, Start: uint32(end), End: uint32(len(l.src))})
	}
	l.pos, l.halted = len(l.src), true
}

func (l *lexer) html() {
	src := l.src
	for i := l.pos; i < len(src); {
		j := strings.Index(src[i:], "<?")
		if j < 0 {
			break
		}
		at := i + j
		if kind, n := openTag(src[at:]); n > 0 {
			if at > l.pos {
				l.emit(token.InlineHTML, at)
			}
			l.emit(kind, at+n)
			return
		}
		i = at + 2
	}
	if l.pos < len(src) {
		l.emit(token.InlineHTML, len(src))
	}
}

func openTag(s string) (token.Kind, int) {
	if strings.HasPrefix(s, "<?=") {
		return token.OpenTagWithEcho, 3
	}
	if len(s) < 5 || !strings.EqualFold(s[:5], "<?php") {
		return token.Invalid, 0
	}
	if len(s) == 5 {
		return token.OpenTag, 5
	}
	switch s[5] {
	case ' ', '\t', '\n':
		return token.OpenTag, 6
	case '\r':
		if len(s) > 6 && s[6] == '\n' {
			return token.OpenTag, 7
		}
		return token.OpenTag, 6
	}
	return token.Invalid, 0
}

func (l *lexer) scripting(insideBraces bool) {
	depth := 0
	for l.pos < len(l.src) {
		src, start := l.src, l.pos
		c := src[start]
		if isWhitespace(c) {
			end := start + 1
			for end < len(src) && isWhitespace(src[end]) {
				end++
			}
			l.emit(token.Whitespace, end)
			continue
		}
		arrow := l.afterArrow
		l.afterArrow = false
		switch {
		case c == '?' && strings.HasPrefix(src[start:], "?>"):
			l.emit(token.CloseTag, closeTagEnd(src, start+2))
			return
		case c == '$' && start+1 < len(src) && isLabelStart(src[start+1]):
			l.emit(token.Variable, labelEnd(src, start+2))
		case isLabelStart(c):
			if arrow {
				l.emit(token.Identifier, labelEnd(src, start+1))
			} else {
				l.word()
			}
		case c == '\\' && start+1 < len(src) && isLabelStart(src[start+1]):
			l.emit(token.NameFullyQualified, qualifiedEnd(src, labelEnd(src, start+2)))
		case isDigit(c) || c == '.' && start+1 < len(src) && isDigit(src[start+1]):
			l.number()
		case c == '\'':
			l.singleQuoted(start + 1)
		case c == '"':
			l.doubleQuoted(start + 1)
		case c == '`':
			l.emit(token.Backtick, start+1)
			l.interpolated('`', 0, "")
			if l.pos < len(l.src) {
				l.emit(token.Backtick, l.pos+1)
			}
		case c == '#' && strings.HasPrefix(src[start:], "#["):
			l.emit(token.Attribute, start+2)
		case c == '#' || strings.HasPrefix(src[start:], "//"):
			l.emit(token.Comment, lineCommentEnd(src, start))
		case strings.HasPrefix(src[start:], "/*"):
			l.blockComment()
		case c == '(' && l.cast():
		case strings.HasPrefix(src[start:], "<<<") && l.heredoc():
		case strings.HasPrefix(src[start:], "<>"):
			l.emit(token.IsNotEqual, start+2)
		case c == '&' && !strings.HasPrefix(src[start:], "&&") && !strings.HasPrefix(src[start:], "&="):
			l.emit(l.ampersand(), start+1)
		case c == '{':
			depth++
			l.emit(token.LeftBrace, start+1)
		case c == '}':
			l.emit(token.RightBrace, start+1)
			if insideBraces && depth == 0 {
				return
			}
			depth--
		default:
			kind, n := token.Punctuator(src[start:], l.version)
			if n == 0 {
				kind, n = token.BadCharacter, 1
			}
			l.emit(kind, start+n)
			l.afterArrow = kind == token.ObjectOperator || kind == token.NullsafeObjectOperator
		}
	}
}

func (l *lexer) word() {
	src, start := l.src, l.pos
	end := labelEnd(src, start+1)
	if end-start == 1 && (src[start] == 'b' || src[start] == 'B') && end < len(src) {
		switch {
		case src[end] == '\'':
			l.singleQuoted(end + 1)
			return
		case src[end] == '"':
			l.doubleQuoted(end + 1)
			return
		case strings.HasPrefix(src[end:], "<<<"):
			l.pos = end
			first := len(l.tokens)
			if l.heredoc() {
				l.tokens[first].Start = uint32(start)
				return
			}
			l.pos = start
		}
	}
	if end < len(src) && src[end] == '\\' && end+1 < len(src) && isLabelStart(src[end+1]) {
		kind := token.NameQualified
		if strings.EqualFold(src[start:end], "namespace") {
			kind = token.NameRelative
		}
		l.emit(kind, qualifiedEnd(src, end))
		return
	}
	kind := keyword(src[start:end])
	if !kind.In(l.version) {
		kind = token.Identifier
	}
	switch kind {
	case token.Enum:
		kind = l.enumKind(end)
	case token.Yield:
		if fromEnd, ok := l.yieldFrom(end); ok {
			l.emit(token.YieldFrom, fromEnd)
			return
		}
	case token.Private, token.Protected, token.Public:
		if len(src)-end >= 5 {
			if set, ok := token.ModifierSet(strings.ToLower(src[start : end+5])); ok && set.In(l.version) {
				l.emit(set, end+5)
				return
			}
		}
	case token.HaltCompiler:
		if l.haltLeft == 0 {
			l.emit(kind, end)
			l.haltLeft = haltCompilerTokens
			return
		}
	case token.Readonly:
		if next := l.wordGap(end); l.version == token.PHP81 && next < len(src) && src[next] == '(' {
			kind = token.Identifier
		}
	}
	l.emit(kind, end)
}

func keyword(label string) token.Kind {
	const longest = len("__halt_compiler")
	if len(label) > longest {
		return token.Identifier
	}
	var buf [longest]byte
	for i := 0; i < len(label); i++ {
		c := label[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	if string(buf[:len(label)]) == "enum" {
		return token.Enum
	}
	if kind, ok := token.Keyword(string(buf[:len(label)])); ok {
		return kind
	}
	return token.Identifier
}

func (l *lexer) wordGap(i int) int {
	if l.version < token.PHP83 {
		for i < len(l.src) && isWhitespace(l.src[i]) {
			i++
		}
		return i
	}
	return skipTrivia(l.src, i)
}

func (l *lexer) enumKind(end int) token.Kind {
	next := l.wordGap(end)
	if next == end || next >= len(l.src) || !isLabelStart(l.src[next]) {
		return token.Identifier
	}
	word := l.src[next:labelEnd(l.src, next+1)]
	if strings.EqualFold(word, "extends") || strings.EqualFold(word, "implements") {
		return token.Identifier
	}
	return token.Enum
}

func (l *lexer) yieldFrom(end int) (int, bool) {
	next := l.wordGap(end)
	if next == end || len(l.src)-next < 4 || !strings.EqualFold(l.src[next:next+4], "from") {
		return 0, false
	}
	if next+4 < len(l.src) && isLabelChar(l.src[next+4]) {
		return 0, false
	}
	return next + 4, true
}

func (l *lexer) ampersand() token.Kind {
	if !token.AmpersandFollowedByVarOrVararg.In(l.version) {
		return token.Ampersand
	}
	next := l.wordGap(l.pos + 1)
	if next < len(l.src) && (l.src[next] == '$' || strings.HasPrefix(l.src[next:], "...")) {
		return token.AmpersandFollowedByVarOrVararg
	}
	return token.AmpersandNotFollowedByVarOrVararg
}

func (l *lexer) number() {
	src, start := l.src, l.pos
	if len(src)-start > 2 && src[start] == '0' {
		var digit func(byte) bool
		base := 0
		switch src[start+1] {
		case 'x', 'X':
			digit, base = isHexDigit, 16
		case 'b', 'B':
			digit, base = isBinaryDigit, 2
		case 'o', 'O':
			if l.version >= token.PHP81 {
				digit, base = isOctalDigit, 8
			}
		}
		if base != 0 && digit(src[start+2]) {
			end := digitsEnd(src, start+2, digit)
			l.emit(integerKind(src[start+2:end], base), end)
			return
		}
	}
	end := digitsEnd(src, start, isDigit)
	float := false
	if end < len(src) && src[end] == '.' {
		float = true
		end = digitsEnd(src, end+1, isDigit)
	}
	if end < len(src) && (src[end] == 'e' || src[end] == 'E') {
		exp := end + 1
		if exp < len(src) && (src[exp] == '+' || src[exp] == '-') {
			exp++
		}
		if exp < len(src) && isDigit(src[exp]) {
			float = true
			end = digitsEnd(src, exp, isDigit)
		}
	}
	if float {
		l.emit(token.FloatLiteral, end)
		return
	}
	text := src[start:end]
	if len(text) > 1 && text[0] == '0' {
		l.emit(integerKind(text[1:], 8), end)
		return
	}
	l.emit(integerKind(text, 10), end)
}

func integerKind(digits string, base int) token.Kind {
	if strings.IndexByte(digits, '_') >= 0 {
		digits = strings.ReplaceAll(digits, "_", "")
	}
	if _, err := strconv.ParseInt(digits, base, 64); errors.Is(err, strconv.ErrRange) {
		return token.FloatLiteral
	}
	return token.IntegerLiteral
}

func digitsEnd(src string, i int, digit func(byte) bool) int {
	for i < len(src) {
		if digit(src[i]) {
			i++
			continue
		}
		if src[i] == '_' && i+1 < len(src) && digit(src[i+1]) {
			i += 2
			continue
		}
		break
	}
	return i
}

func (l *lexer) singleQuoted(i int) {
	src := l.src
	for i < len(src) {
		switch src[i] {
		case '\\':
			i += 2
		case '\'':
			l.emit(token.ConstantString, i+1)
			return
		default:
			i++
		}
	}
	l.emit(token.EncapsedAndWhitespace, len(src))
}

func (l *lexer) doubleQuoted(i int) {
	src := l.src
	for j := i; j < len(src); j++ {
		switch src[j] {
		case '\\':
			j++
		case '"':
			l.emit(token.ConstantString, j+1)
			return
		case '$':
			if j+1 < len(src) && (isLabelStart(src[j+1]) || src[j+1] == '{') {
				l.interpolatedString(i)
				return
			}
		case '{':
			if j+1 < len(src) && src[j+1] == '$' {
				l.interpolatedString(i)
				return
			}
		}
	}
	l.emit(token.DoubleQuote, i)
	if l.pos < len(src) {
		l.emit(token.EncapsedAndWhitespace, len(src))
	}
}

func (l *lexer) interpolatedString(bodyStart int) {
	l.emit(token.DoubleQuote, bodyStart)
	l.interpolated('"', 0, "")
	if l.pos < len(l.src) {
		l.emit(token.DoubleQuote, l.pos+1)
	}
}

func (l *lexer) interpolated(terminator byte, bodyStart int, label string) {
	src := l.src
	i := l.pos
	for i < len(src) {
		if label != "" && (i == bodyStart || src[i-1] == '\n' || src[i-1] == '\r') {
			if end, ok := closingLabel(src, i, label); ok {
				l.text(i)
				l.emit(token.EndHeredoc, end)
				return
			}
		}
		c := src[i]
		switch {
		case c == '\\':
			i += 2
			continue
		case terminator != 0 && c == terminator:
			l.text(i)
			return
		case c == '$' && i+1 < len(src) && isLabelStart(src[i+1]):
			l.text(i)
			l.variableInString()
			i = l.pos
			continue
		case c == '$' && i+1 < len(src) && src[i+1] == '{':
			l.text(i)
			l.emit(token.DollarOpenCurlyBraces, i+2)
			if end := labelEndAt(src, l.pos); end > l.pos && end < len(src) && (src[end] == '[' || src[end] == '}') {
				l.emit(token.StringVarname, end)
			}
			l.scripting(true)
			i = l.pos
			continue
		case c == '{' && i+1 < len(src) && src[i+1] == '$':
			l.text(i)
			l.emit(token.CurlyOpen, i+1)
			l.scripting(true)
			i = l.pos
			continue
		}
		i++
	}
	l.text(min(i, len(src)))
}

func (l *lexer) text(end int) {
	if end > l.pos {
		l.emit(token.EncapsedAndWhitespace, end)
	}
}

func (l *lexer) variableInString() {
	src := l.src
	l.emit(token.Variable, labelEnd(src, l.pos+2))
	i := l.pos
	switch {
	case i < len(src) && src[i] == '[':
		l.emit(token.LeftBracket, i+1)
		l.offsetInString()
	case strings.HasPrefix(src[i:], "->") && i+2 < len(src) && isLabelStart(src[i+2]):
		l.emit(token.ObjectOperator, i+2)
		l.emit(token.Identifier, labelEnd(src, i+3))
	case strings.HasPrefix(src[i:], "?->") && i+3 < len(src) && isLabelStart(src[i+3]):
		l.emit(token.NullsafeObjectOperator, i+3)
		l.emit(token.Identifier, labelEnd(src, i+4))
	}
}

func (l *lexer) offsetInString() {
	src := l.src
	for l.pos < len(src) {
		i := l.pos
		switch c := src[i]; {
		case c == ']':
			l.emit(token.RightBracket, i+1)
			return
		case c == '$' && i+1 < len(src) && isLabelStart(src[i+1]):
			l.emit(token.Variable, labelEnd(src, i+2))
		case isLabelStart(c):
			l.emit(token.Identifier, labelEnd(src, i+1))
		case isDigit(c):
			l.emit(token.NumString, l.offsetNumberEnd(i))
		case c == '-' || c == '[':
			kind, n := token.Punctuator(src[i:i+1], l.version)
			l.emit(kind, i+n)
		default:
			return
		}
	}
}

func (l *lexer) offsetNumberEnd(i int) int {
	src := l.src
	if len(src)-i > 2 && src[i] == '0' {
		switch src[i+1] {
		case 'x', 'X':
			return digitsEnd(src, i+2, isHexDigit)
		case 'b', 'B':
			return digitsEnd(src, i+2, isBinaryDigit)
		case 'o', 'O':
			if l.version >= token.PHP81 {
				return digitsEnd(src, i+2, isOctalDigit)
			}
		}
	}
	return digitsEnd(src, i, isDigit)
}

func (l *lexer) heredoc() bool {
	src := l.src
	i := l.pos + 3
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	quote := byte(0)
	if i < len(src) && (src[i] == '\'' || src[i] == '"') {
		quote = src[i]
		i++
	}
	if i >= len(src) || !isLabelStart(src[i]) {
		return false
	}
	labelStart := i
	i = labelEnd(src, i+1)
	label := src[labelStart:i]
	if quote != 0 {
		if i >= len(src) || src[i] != quote {
			return false
		}
		i++
	}
	bodyStart, ok := newlineEnd(src, i)
	if !ok {
		return false
	}
	l.emit(token.StartHeredoc, bodyStart)
	if quote == '\'' {
		l.nowdoc(bodyStart, label)
		return true
	}
	l.interpolated(0, bodyStart, label)
	return true
}

func (l *lexer) nowdoc(bodyStart int, label string) {
	src := l.src
	for i := bodyStart; i < len(src); {
		if end, ok := closingLabel(src, i, label); ok {
			l.text(i)
			l.emit(token.EndHeredoc, end)
			return
		}
		nl := strings.IndexAny(src[i:], "\r\n")
		if nl < 0 {
			break
		}
		i, _ = newlineEnd(src, i+nl)
	}
	l.text(len(src))
}

func closingLabel(src string, i int, label string) (int, bool) {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	if !strings.HasPrefix(src[i:], label) {
		return 0, false
	}
	end := i + len(label)
	if end < len(src) && isLabelChar(src[end]) {
		return 0, false
	}
	return end, true
}

func newlineEnd(src string, i int) (int, bool) {
	switch {
	case strings.HasPrefix(src[i:], "\r\n"):
		return i + 2, true
	case i < len(src) && (src[i] == '\n' || src[i] == '\r'):
		return i + 1, true
	}
	return i, false
}

var castKinds = map[string]token.Kind{
	"int": token.IntCast, "integer": token.IntCast,
	"bool": token.BoolCast, "boolean": token.BoolCast,
	"float": token.FloatCast, "double": token.FloatCast, "real": token.FloatCast,
	"string": token.StringCast, "binary": token.StringCast,
	"array": token.ArrayCast, "object": token.ObjectCast,
	"unset": token.UnsetCast, "void": token.VoidCast,
}

func (l *lexer) cast() bool {
	src := l.src
	i := skipSpaces(src, l.pos+1)
	nameStart := i
	for i < len(src) && isLetter(src[i]) {
		i++
	}
	if i == nameStart || i-nameStart > len("integer") {
		return false
	}
	kind, ok := castKinds[strings.ToLower(src[nameStart:i])]
	if !ok || !kind.In(l.version) {
		return false
	}
	i = skipSpaces(src, i)
	if i >= len(src) || src[i] != ')' {
		return false
	}
	l.emit(kind, i+1)
	return true
}

func (l *lexer) blockComment() {
	src, start := l.src, l.pos
	kind := token.Comment
	if len(src)-start > 3 && src[start+2] == '*' && isWhitespace(src[start+3]) {
		kind = token.DocComment
	}
	end := strings.Index(src[start+2:], "*/")
	if end < 0 {
		l.emit(kind, len(src))
		return
	}
	l.emit(kind, start+2+end+2)
}

func lineCommentEnd(src string, i int) int {
	for ; i < len(src); i++ {
		switch src[i] {
		case '\n', '\r':
			return i
		case '?':
			if i+1 < len(src) && src[i+1] == '>' {
				return i
			}
		}
	}
	return i
}

func closeTagEnd(src string, i int) int {
	end, _ := newlineEnd(src, i)
	return end
}

func skipTrivia(src string, i int) int {
	for i < len(src) {
		switch {
		case isWhitespace(src[i]):
			i++
		case src[i] == '#' && !strings.HasPrefix(src[i:], "#[") || strings.HasPrefix(src[i:], "//"):
			i = lineCommentEnd(src, i)
		case strings.HasPrefix(src[i:], "/*"):
			end := strings.Index(src[i+2:], "*/")
			if end < 0 {
				return len(src)
			}
			i += 2 + end + 2
		default:
			return i
		}
	}
	return i
}

func skipSpaces(src string, i int) int {
	for i < len(src) && (src[i] == ' ' || src[i] == '\t') {
		i++
	}
	return i
}

func labelEnd(src string, i int) int {
	for i < len(src) && isLabelChar(src[i]) {
		i++
	}
	return i
}

func labelEndAt(src string, i int) int {
	if i >= len(src) || !isLabelStart(src[i]) {
		return i
	}
	return labelEnd(src, i+1)
}

func qualifiedEnd(src string, i int) int {
	for i+1 < len(src) && src[i] == '\\' && isLabelStart(src[i+1]) {
		i = labelEnd(src, i+2)
	}
	return i
}

func isWhitespace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

func isLetter(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

func isLabelStart(c byte) bool {
	return isLetter(c) || c == '_' || c >= 0x80
}

func isLabelChar(c byte) bool {
	return isLabelStart(c) || isDigit(c)
}

func isDigit(c byte) bool       { return '0' <= c && c <= '9' }
func isBinaryDigit(c byte) bool { return c == '0' || c == '1' }
func isOctalDigit(c byte) bool  { return '0' <= c && c <= '7' }
func isHexDigit(c byte) bool    { return isDigit(c) || 'a' <= c && c <= 'f' || 'A' <= c && c <= 'F' }
