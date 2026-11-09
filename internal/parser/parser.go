package parser

import (
	"strings"
	"unicode"
)

// PHPParser parses PHP source into a lightweight AST for LSP features.
type PHPParser struct{}

func New() *PHPParser {
	return &PHPParser{}
}

// ParseResult holds the structured output of parsing a PHP file.
type ParseResult struct {
	Namespace string
	Uses      []UseStatement
	Classes   []ClassDef
	// AnonymousClasses are the `new class { ... }` expressions in the file,
	// kept apart from Classes, whose entries are declarations.
	AnonymousClasses []ClassDef
	Interfaces       []InterfaceDef
	Traits           []TraitDef
	Enums            []EnumDef
	Functions        []FunctionDef
	Constants        []ConstantDef
	Variables        []VariableDef
	Tokens           []Token
	Errors           []ParseError
	Lines            []string
	// Matches holds all match expressions found in the file (including those
	// inside function and method bodies). Populated by the expression scanner
	// that runs after the declaration-level pass. Nested match expressions are
	// each recorded individually; nesting depth is not tracked here.
	Matches matchList
	// Calls holds call sites collected from the file up to maxCallSitesPerFile.
	// Only calls with recognisable callees are recorded (plain identifiers,
	// $var->method chains, Type::method, new Type). The slice is populated by
	// the expression scanner that runs after the declaration-level pass.
	Calls callList
}

type UseStatement struct {
	FullName string
	Alias    string
	Kind     string // "class", "function", "const"
	Line     int
}

type ClassDef struct {
	Name       string
	FullName   string
	Extends    string
	Implements []string
	Traits     []string
	Properties []PropertyDef
	Methods    []MethodDef
	Constants  []ConstantDef
	Attributes []AttributeDef
	IsAbstract bool
	IsFinal    bool
	IsReadonly bool
	Line       int
	EndLine    int
	DocComment string
	// BodyLine and BodyCol are where an anonymous class's `{` is.
	BodyLine, BodyCol int
}

type InterfaceDef struct {
	Name       string
	FullName   string
	Extends    []string
	Methods    []MethodDef
	Properties []PropertyDef
	Constants  []ConstantDef
	Attributes []AttributeDef
	Line       int
	EndLine    int
}

type TraitDef struct {
	Name       string
	FullName   string
	Properties []PropertyDef
	Methods    []MethodDef
	Traits     []string
	Constants  []ConstantDef
	Attributes []AttributeDef
	Line       int
	EndLine    int
}

type EnumDef struct {
	Name       string
	FullName   string
	BackedType string
	Cases      []EnumCase
	Methods    []MethodDef
	Properties []PropertyDef
	Constants  []ConstantDef
	Traits     []string
	Implements []string
	Attributes []AttributeDef
	DocComment string
	Line       int
	EndLine    int
}

type EnumCase struct {
	Name       string
	Value      string
	Attributes []AttributeDef
	DocComment string
	Line       int
}

type FunctionDef struct {
	Attributes []AttributeDef
	Name       string
	FullName   string
	Params     []ParamDef
	ReturnType string
	IsVariadic bool
	Line       int
	EndLine    int
	DocComment string
}

type MethodDef struct {
	Attributes []AttributeDef
	Name       string
	Params     []ParamDef
	ReturnType string
	Visibility string
	IsStatic   bool
	IsAbstract bool
	IsFinal    bool
	Line       int
	EndLine    int
	DocComment string
}

// PropertyHook represents a single get or set accessor in a PHP 8.4 property hook block.
type PropertyHook struct {
	Kind string // "get" or "set"
}

type PropertyDef struct {
	Name          string
	Type          string
	Visibility    string
	SetVisibility string // PHP 8.4 asymmetric visibility: e.g. "private" in public private(set)
	IsStatic      bool
	IsReadonly    bool
	HasDefault    bool
	Hooks         []PropertyHook // PHP 8.4 property hooks (get/set accessors)
	Attributes    []AttributeDef
	Line          int
	DocComment    string
}

type ParamDef struct {
	Name        string
	Type        string
	HasDefault  bool
	IsVariadic  bool
	IsReference bool
	IsPromoted  bool
	Visibility  string
	IsReadonly  bool

	SetVisibility string
	DefaultValue  string
	Hooks         []PropertyHook
	Attributes    []AttributeDef
	DocComment    string
	Line          int
}

type ConstantDef struct {
	Name       string
	Value      string
	Type       string
	Visibility string
	Attributes []AttributeDef
	Line       int
}

type VariableDef struct {
	Name string
	Type string
	Line int
}

type AttributeDef struct {
	Name string
	Args []string
	Line int
}

type Token struct {
	Kind   TokenKind
	Value  string
	Line   int
	Column int
	Offset int
}

type TokenKind int

const (
	TokenPHP TokenKind = iota
	TokenNamespace
	TokenUse
	TokenClass
	TokenInterface
	TokenTrait
	TokenEnum
	TokenFunction
	TokenConst
	TokenVar
	TokenPublic
	TokenProtected
	TokenPrivate
	TokenStatic
	TokenAbstract
	TokenFinal
	TokenReadonly
	TokenExtends
	TokenImplements
	TokenReturn
	TokenNew
	TokenArrow
	TokenDoubleArrow
	TokenDoubleColon
	TokenBackslash
	TokenString
	TokenNumber
	TokenVariable
	TokenIdentifier
	TokenOpenBrace
	TokenCloseBrace
	TokenOpenParen
	TokenCloseParen
	TokenOpenBracket
	TokenCloseBracket
	TokenSemicolon
	TokenComma
	TokenEquals
	TokenColon
	TokenQuestion
	TokenPipe
	TokenPipeArrow // |> (PHP 8.5 pipe operator)
	TokenAmpersand
	TokenAt
	TokenHash
	TokenDocComment
	TokenComment
	TokenWhitespace
	TokenStringLiteral
	TokenEOF
	TokenUnknown
)

type ParseError struct {
	Message string
	Line    int
	Column  int
}

// Parse tokenizes and performs a structural parse of PHP source code.
func (p *PHPParser) Parse(source string) *ParseResult {
	result := &ParseResult{
		Lines: strings.Split(source, "\n"),
	}
	tokens, errs := tokenize(source)
	result.Tokens = tokens
	result.Errors = append(result.Errors, errs...)
	parseStructure(tokens, result)
	return result
}

// GetTokenAtPosition returns the token at the given line and character.
func (pr *ParseResult) GetTokenAtPosition(line, character int) *Token {
	for i := range pr.Tokens {
		t := &pr.Tokens[i]
		if t.Line == line {
			end := t.Column + len(t.Value)
			if character >= t.Column && character <= end {
				return t
			}
		}
	}
	return nil
}

// GetWordAtPosition extracts the word/identifier at a given position.
func (pr *ParseResult) GetWordAtPosition(line, character int) string {
	if line >= len(pr.Lines) {
		return ""
	}
	ln := pr.Lines[line]
	if character >= len(ln) {
		return ""
	}
	start := character
	for start > 0 && isIdentChar(rune(ln[start-1])) {
		start--
	}
	if start > 0 && ln[start-1] == '$' {
		start--
	}
	end := character
	for end < len(ln) && isIdentChar(rune(ln[end])) {
		end++
	}
	return ln[start:end]
}

// GetContextAtPosition returns contextual info about -> or :: access.
func (pr *ParseResult) GetContextAtPosition(line, character int) (contextType string, receiver string) {
	if line >= len(pr.Lines) {
		return "", ""
	}
	ln := pr.Lines[line]
	if character < 2 {
		return "", ""
	}

	pos := character - 1
	for pos >= 0 && pos < len(ln) && ln[pos] == ' ' {
		pos--
	}

	if pos >= 1 && pos < len(ln) && ln[pos-1] == '-' && ln[pos] == '>' {
		rEnd := pos - 1
		rStart := rEnd - 1
		for rStart >= 0 && (isIdentChar(rune(ln[rStart])) || ln[rStart] == '$') {
			rStart--
		}
		rStart++
		if rStart <= rEnd {
			return "member", ln[rStart : rEnd+1]
		}
	}

	if pos >= 1 && pos < len(ln) && ln[pos-1] == ':' && ln[pos] == ':' {
		rEnd := pos - 1
		rStart := rEnd - 1
		for rStart >= 0 && (isIdentChar(rune(ln[rStart])) || ln[rStart] == '\\') {
			rStart--
		}
		rStart++
		if rStart <= rEnd {
			return "static", ln[rStart : rEnd+1]
		}
	}

	if pos >= 0 && pos < len(ln) && ln[pos] == '\\' {
		rStart := pos - 1
		for rStart >= 0 && (isIdentChar(rune(ln[rStart])) || ln[rStart] == '\\') {
			rStart--
		}
		rStart++
		return "namespace", ln[rStart : pos+1]
	}

	return "", ""
}

func isIdentChar(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

// --- Tokenizer ---

// OpenTagLen returns the length of the PHP open tag at s[i:] (`<?php`, `<?=`,
// or a short `<?` followed by whitespace), or 0 when there is none.
func OpenTagLen(s string, i int) int {
	if i+1 >= len(s) || s[i] != '<' || s[i+1] != '?' {
		return 0
	}
	rest := s[i+2:]
	if len(rest) >= 3 && strings.EqualFold(rest[:3], "php") && (len(rest) == 3 || !isIdentByte(rest[3])) {
		return 5
	}
	if len(rest) >= 1 && rest[0] == '=' {
		return 3
	}
	if len(rest) == 0 || rest[0] == ' ' || rest[0] == '\t' || rest[0] == '\n' || rest[0] == '\r' {
		return 2
	}
	return 0
}

// HasOpenTag reports whether source contains a PHP open tag. Sources without
// one are snippets and are treated as PHP from the first byte.
func HasOpenTag(source string) bool {
	_, n := nextOpenTag(source, 0)
	return n > 0
}

// nextOpenTag finds the next open tag at or after from. It returns its offset
// and length, or len(source) and 0 when there is none.
func nextOpenTag(source string, from int) (int, int) {
	for i := from; i+1 < len(source); i++ {
		j := strings.Index(source[i:], "<?")
		if j < 0 {
			break
		}
		i += j
		if n := OpenTagLen(source, i); n > 0 {
			return i, n
		}
	}
	return len(source), 0
}

func isIdentByte(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9') || b >= 0x80
}

func tokenize(source string) ([]Token, []ParseError) {
	var errors []ParseError
	tokens := make([]Token, 0, len(source)/4)
	line, col, offset := 0, 0, 0
	// Text outside <?php … ?> is inline HTML and produces no tokens. A source
	// with no open tag at all (a snippet) is read as PHP throughout.
	inHTML := HasOpenTag(source)

	for offset < len(source) {
		if inHTML {
			next, tagLen := nextOpenTag(source, offset)
			for offset < next {
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			if tagLen == 0 {
				break
			}
			tokens = append(tokens, Token{TokenPHP, source[offset : offset+tagLen], line, col, offset})
			offset += tagLen
			col += tagLen
			inHTML = false
			continue
		}

		ch := source[offset]

		// Close tag: ends the statement and switches back to inline HTML.
		if ch == '?' && offset+1 < len(source) && source[offset+1] == '>' {
			tokens = append(tokens, Token{TokenSemicolon, "?>", line, col, offset})
			offset += 2
			col += 2
			inHTML = true
			continue
		}

		// Whitespace
		if ch == ' ' || ch == '\t' || ch == '\r' || ch == '\n' {
			for offset < len(source) && (source[offset] == ' ' || source[offset] == '\t' || source[offset] == '\r' || source[offset] == '\n') {
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			continue
		}

		// Doc comment /** ... */
		if offset+2 < len(source) && source[offset:offset+3] == "/**" {
			start := offset
			startLine, startCol := line, col
			offset += 3
			col += 3
			for offset+1 < len(source) {
				if source[offset] == '*' && source[offset+1] == '/' {
					offset += 2
					col += 2
					break
				}
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			tokens = append(tokens, Token{TokenDocComment, source[start:offset], startLine, startCol, start})
			if !strings.HasSuffix(source[start:offset], "*/") {
				errors = append(errors, ParseError{Message: "unterminated doc comment", Line: startLine, Column: startCol})
			}
			continue
		}

		// Block comment
		if offset+1 < len(source) && source[offset:offset+2] == "/*" {
			start := offset
			startLine, startCol := line, col
			offset += 2
			col += 2
			for offset+1 < len(source) {
				if source[offset] == '*' && source[offset+1] == '/' {
					offset += 2
					col += 2
					break
				}
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			tokens = append(tokens, Token{TokenComment, source[start:offset], startLine, startCol, start})
			if !strings.HasSuffix(source[start:offset], "*/") {
				errors = append(errors, ParseError{Message: "unterminated comment", Line: startLine, Column: startCol})
			}
			continue
		}

		// Line comment
		if (offset+1 < len(source) && source[offset:offset+2] == "//") || (ch == '#' && !(offset+1 < len(source) && source[offset+1] == '[')) {
			start := offset
			startCol := col
			// A line comment also ends at a close tag, which is left for the
			// next iteration.
			for offset < len(source) && source[offset] != '\n' && !(source[offset] == '?' && offset+1 < len(source) && source[offset+1] == '>') {
				offset++
				col++
			}
			tokens = append(tokens, Token{TokenComment, source[start:offset], line, startCol, start})
			continue
		}

		// Attribute #[
		if ch == '#' && offset+1 < len(source) && source[offset+1] == '[' {
			tokens = append(tokens, Token{TokenHash, "#", line, col, offset})
			offset++
			col++
			continue
		}

		// Heredoc / nowdoc: <<<LABEL, <<<"LABEL" or <<<'LABEL' through the
		// closing label, which may be indented (PHP 7.3+). Emitted as one
		// string literal token so the body is never read as code.
		if ch == '<' && strings.HasPrefix(source[offset:], "<<<") {
			if end, ok := heredocEnd(source, offset); ok {
				start := offset
				startLine, startCol := line, col
				for offset < end {
					if source[offset] == '\n' {
						line++
						col = 0
					} else {
						col++
					}
					offset++
				}
				tokens = append(tokens, Token{TokenStringLiteral, source[start:end], startLine, startCol, start})
				continue
			}
		}

		// String literals
		if ch == '"' || ch == '\'' {
			start := offset
			startLine, startCol := line, col
			quote := ch
			offset++
			col++
			for offset < len(source) {
				if source[offset] == '\\' && offset+1 < len(source) {
					offset += 2
					col += 2
					continue
				}
				if source[offset] == byte(quote) {
					offset++
					col++
					break
				}
				if source[offset] == '\n' {
					line++
					col = 0
				} else {
					col++
				}
				offset++
			}
			val := source[start:offset]
			tokens = append(tokens, Token{TokenStringLiteral, val, startLine, startCol, start})
			if len(val) < 2 || val[len(val)-1] != byte(quote) {
				errors = append(errors, ParseError{Message: "unterminated string", Line: startLine, Column: startCol})
			}
			continue
		}

		// Variable $name
		if ch == '$' && offset+1 < len(source) && isIdentChar(rune(source[offset+1])) {
			start := offset
			startCol := col
			offset++
			col++
			for offset < len(source) && isIdentChar(rune(source[offset])) {
				offset++
				col++
			}
			tokens = append(tokens, Token{TokenVariable, source[start:offset], line, startCol, start})
			continue
		}

		// Numbers
		if ch >= '0' && ch <= '9' {
			start := offset
			startCol := col
			for offset < len(source) && (source[offset] >= '0' && source[offset] <= '9' || source[offset] == '.' || source[offset] == '_') {
				offset++
				col++
			}
			tokens = append(tokens, Token{TokenNumber, source[start:offset], line, startCol, start})
			continue
		}

		// Identifiers and keywords
		if isIdentChar(rune(ch)) {
			start := offset
			startCol := col
			for offset < len(source) && isIdentChar(rune(source[offset])) {
				offset++
				col++
			}
			word := source[start:offset]
			kind := identifierToKind(word)
			tokens = append(tokens, Token{kind, word, line, startCol, start})
			continue
		}

		// Comparison operators, so that their '=' can't be read as an assignment
		if op := comparisonOperatorAt(source, offset); op != "" {
			tokens = append(tokens, Token{TokenUnknown, op, line, col, offset})
			offset += len(op)
			col += len(op)
			continue
		}

		// Multi-char operators
		if offset+1 < len(source) {
			two := source[offset : offset+2]
			switch two {
			case "->":
				tokens = append(tokens, Token{TokenArrow, "->", line, col, offset})
				offset += 2
				col += 2
				continue
			case "=>":
				tokens = append(tokens, Token{TokenDoubleArrow, "=>", line, col, offset})
				offset += 2
				col += 2
				continue
			case "::":
				tokens = append(tokens, Token{TokenDoubleColon, "::", line, col, offset})
				offset += 2
				col += 2
				continue
			case "|>":
				// PHP 8.5 pipe operator — must be checked before the single-char '|' case
				tokens = append(tokens, Token{TokenPipeArrow, "|>", line, col, offset})
				offset += 2
				col += 2
				continue
			}
		}

		// Single-char tokens
		kind := TokenUnknown
		switch ch {
		case '{':
			kind = TokenOpenBrace
		case '}':
			kind = TokenCloseBrace
		case '(':
			kind = TokenOpenParen
		case ')':
			kind = TokenCloseParen
		case '[':
			kind = TokenOpenBracket
		case ']':
			kind = TokenCloseBracket
		case ';':
			kind = TokenSemicolon
		case ',':
			kind = TokenComma
		case '=':
			kind = TokenEquals
		case ':':
			kind = TokenColon
		case '?':
			kind = TokenQuestion
		case '|':
			kind = TokenPipe
		case '&':
			kind = TokenAmpersand
		case '@':
			kind = TokenAt
		case '\\':
			kind = TokenBackslash
		}
		tokens = append(tokens, Token{kind, string(ch), line, col, offset})
		offset++
		col++
	}

	tokens = append(tokens, Token{TokenEOF, "", line, col, offset})
	return tokens, errors
}

// comparisonOperatorAt returns the comparison operator starting at offset, or "".
// heredocEnd returns the offset just past the closing label of the heredoc or
// nowdoc opening at start (which points at `<<<`). ok is false when this is
// not a heredoc opener or the closing label is missing, so the caller falls
// back to ordinary tokenizing.
func heredocEnd(source string, start int) (int, bool) {
	i := start + 3
	for i < len(source) && (source[i] == ' ' || source[i] == '\t') {
		i++
	}
	quote := byte(0)
	if i < len(source) && (source[i] == '\'' || source[i] == '"') {
		quote = source[i]
		i++
	}
	labelStart := i
	for i < len(source) && isIdentChar(rune(source[i])) {
		i++
	}
	label := source[labelStart:i]
	if label == "" || (label[0] >= '0' && label[0] <= '9') {
		return 0, false
	}
	if quote != 0 {
		if i >= len(source) || source[i] != quote {
			return 0, false
		}
		i++
	}
	if i < len(source) && source[i] == '\r' {
		i++
	}
	if i >= len(source) || source[i] != '\n' {
		return 0, false
	}
	// Scan the body line by line for the closing label.
	for pos := i + 1; pos <= len(source); {
		lineEnd := strings.IndexByte(source[pos:], '\n')
		if lineEnd < 0 {
			lineEnd = len(source)
		} else {
			lineEnd += pos
		}
		j := pos
		for j < lineEnd && (source[j] == ' ' || source[j] == '\t') {
			j++
		}
		if strings.HasPrefix(source[j:lineEnd], label) {
			after := j + len(label)
			if after >= len(source) || !isIdentChar(rune(source[after])) {
				return after, true
			}
		}
		if lineEnd >= len(source) {
			break
		}
		pos = lineEnd + 1
	}
	return 0, false
}

func comparisonOperatorAt(source string, offset int) string {
	for _, op := range []string{"===", "!==", "<=>", "==", "!=", "<=", ">="} {
		if strings.HasPrefix(source[offset:], op) {
			return op
		}
	}
	return ""
}

func identifierToKind(word string) TokenKind {
	switch word {
	case "namespace":
		return TokenNamespace
	case "use":
		return TokenUse
	case "class":
		return TokenClass
	case "interface":
		return TokenInterface
	case "trait":
		return TokenTrait
	case "enum":
		return TokenEnum
	case "function", "fn":
		return TokenFunction
	case "const":
		return TokenConst
	case "var":
		return TokenVar
	case "public":
		return TokenPublic
	case "protected":
		return TokenProtected
	case "private":
		return TokenPrivate
	case "static":
		return TokenStatic
	case "abstract":
		return TokenAbstract
	case "final":
		return TokenFinal
	case "readonly":
		return TokenReadonly
	case "extends":
		return TokenExtends
	case "implements":
		return TokenImplements
	case "return":
		return TokenReturn
	case "new":
		return TokenNew
	default:
		return TokenIdentifier
	}
}

// parseStructure extracts top-level structural info from tokens, then runs
// the expression scanner to collect match expressions and call sites.
func parseStructure(tokens []Token, result *ParseResult) {
	p := &structParser{tokens: tokens, pos: 0, result: result}
	p.parse()
	p.parseAnonymousClasses()
	// Expression scanner is a separate single-pass over the token stream.
	// It runs after declarations so it shares the same token slice without
	// re-tokenising. Strings/comments are already collapsed, so keyword
	// matches inside them cannot occur.
	scanExpressions(tokens, result)
}

type structParser struct {
	tokens     []Token
	pos        int
	result     *ParseResult
	attributes []AttributeDef // read, waiting for the declaration they belong to
	enumCases  *[]EnumCase    // where the enum body being parsed collects its cases
}

func (p *structParser) peek() Token {
	if p.pos >= len(p.tokens) {
		return Token{Kind: TokenEOF}
	}
	return p.tokens[p.pos]
}

func (p *structParser) advance() Token {
	t := p.peek()
	if p.pos < len(p.tokens) {
		p.pos++
	}
	return t
}

func (p *structParser) expect(kind TokenKind) (Token, bool) {
	t := p.peek()
	if t.Kind == kind {
		p.advance()
		return t, true
	}
	return t, false
}

func (p *structParser) skipUntil(kinds ...TokenKind) {
	depth := 0
	for p.peek().Kind != TokenEOF {
		t := p.peek()
		if t.Kind == TokenOpenBrace || t.Kind == TokenOpenParen {
			depth++
		} else if t.Kind == TokenCloseBrace || t.Kind == TokenCloseParen {
			if depth == 0 {
				return
			}
			depth--
		}
		for _, k := range kinds {
			if t.Kind == k && depth == 0 {
				return
			}
		}
		p.advance()
	}
}

func (p *structParser) skipBlock() {
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	depth := 1
	for depth > 0 && p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenOpenBrace:
			depth++
		case TokenCloseBrace:
			depth--
		}
		p.advance()
	}
}

// isRecoveryPoint returns true if the current token indicates a top-level
// declaration boundary, used to recover from missing closing braces/parens.
func (p *structParser) isRecoveryPoint() bool {
	switch p.peek().Kind {
	case TokenNamespace, TokenInterface, TokenTrait, TokenEnum:
		return true
	case TokenClass:
		// Don't recover if preceded by 'new' (anonymous class expression)
		for i := p.pos - 1; i >= 0; i-- {
			k := p.tokens[i].Kind
			if k == TokenDocComment || k == TokenComment {
				continue
			}
			return k != TokenNew
		}
		return true
	}
	return false
}

// isKeywordToken returns true for tokens that are PHP keywords but can be
// used as method names (PHP 7.0+ allows reserved words as method names).
func isKeywordToken(kind TokenKind) bool {
	switch kind {
	case TokenClass, TokenInterface, TokenTrait, TokenEnum, TokenNamespace,
		TokenFunction, TokenConst, TokenVar,
		TokenPublic, TokenProtected, TokenPrivate,
		TokenStatic, TokenAbstract, TokenFinal, TokenReadonly,
		TokenExtends, TokenImplements, TokenReturn, TokenNew:
		return true
	}
	return false
}

func (p *structParser) parse() {
	var lastDocComment string
	for p.peek().Kind != TokenEOF {
		startPos := p.pos
		t := p.peek()
		switch t.Kind {
		case TokenDocComment:
			lastDocComment = t.Value
			p.advance()
			continue
		case TokenHash:
			p.attributes = append(p.attributes, p.parseAttributeGroup()...)
			continue
		case TokenNamespace:
			p.parseNamespace()
		case TokenUse:
			p.parseUse()
		case TokenClass:
			p.parseClass(lastDocComment)
		case TokenAbstract, TokenFinal, TokenReadonly:
			// Look ahead for class
			p.parseClass(lastDocComment)
		case TokenInterface:
			p.parseInterface()
		case TokenTrait:
			p.parseTrait()
		case TokenEnum:
			p.parseEnum(lastDocComment)
		case TokenFunction:
			p.parseFunction(lastDocComment)
		case TokenConst:
			p.parseConst()
		default:
			p.advance()
		}
		lastDocComment = ""
		p.attributes = nil
		// Safety: ensure forward progress to prevent infinite loops on malformed input
		if p.pos == startPos {
			p.advance()
		}
	}
}

// parseAttributeGroup reads a PHP 8 `#[A, B(1, 'x')]` group.
func (p *structParser) parseAttributeGroup() []AttributeDef {
	if p.peek().Kind != TokenHash {
		return nil
	}
	p.advance() // #
	if p.peek().Kind != TokenOpenBracket {
		return nil
	}
	p.advance() // [
	var attrs []AttributeDef
	for p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenCloseBracket:
			p.advance()
			return attrs
		case TokenComma:
			p.advance()
			continue
		}
		attr := AttributeDef{Line: p.peek().Line, Name: p.readTypeName()}
		if attr.Name == "" {
			p.skipToAttributeGroupEnd()
			return attrs
		}
		if p.peek().Kind == TokenOpenParen {
			attr.Args = p.readAttributeArgs()
		}
		attrs = append(attrs, attr)
	}
	return attrs
}

func (p *structParser) skipToAttributeGroupEnd() {
	depth := 1
	for depth > 0 && p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenOpenBracket:
			depth++
		case TokenCloseBracket:
			depth--
		}
		p.advance()
	}
}

// readAttributeArgs returns each argument of an attribute as it's written.
func (p *structParser) readAttributeArgs() []string {
	p.advance() // (
	var args []string
	start, depth := p.pos, 0
	for p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenOpenParen, TokenOpenBracket:
			depth++
		case TokenCloseBracket:
			depth--
		case TokenCloseParen:
			if depth == 0 {
				if p.pos > start {
					args = append(args, tokensText(p.tokens[start:p.pos]))
				}
				p.advance()
				return args
			}
			depth--
		case TokenComma:
			if depth == 0 {
				args = append(args, tokensText(p.tokens[start:p.pos]))
				start = p.pos + 1
			}
		}
		p.advance()
	}
	return args
}

// tokensText joins tokens back into source text, spacing out only the words
// that would otherwise run together.
func tokensText(tokens []Token) string {
	var b strings.Builder
	prevWord := false
	for _, t := range tokens {
		isWord := t.Value != "" && (isIdentChar(rune(t.Value[0])) || t.Value[0] == '$' || t.Value[0] == '\'')
		if prevWord && isWord {
			b.WriteByte(' ')
		}
		b.WriteString(t.Value)
		prevWord = isWord
	}
	return b.String()
}

func (p *structParser) takeAttributes() []AttributeDef {
	attrs := p.attributes
	p.attributes = nil
	return attrs
}

func (p *structParser) parseNamespace() {
	p.advance()
	var parts []string
	for {
		t := p.peek()
		if t.Kind == TokenIdentifier {
			parts = append(parts, t.Value)
			p.advance()
		} else if t.Kind == TokenBackslash {
			p.advance()
		} else {
			break
		}
	}
	p.result.Namespace = strings.Join(parts, "\\")
	p.expect(TokenSemicolon)
}

func (p *structParser) parseUse() {
	p.advance()
	kind := "class"
	if p.peek().Kind == TokenFunction {
		kind = "function"
		p.advance()
	} else if p.peek().Kind == TokenConst {
		kind = "const"
		p.advance()
	}

	var parts []string
	line := p.peek().Line
	for {
		t := p.peek()
		if t.Kind == TokenIdentifier && t.Value != "as" {
			parts = append(parts, t.Value)
			p.advance()
		} else if t.Kind == TokenBackslash {
			parts = append(parts, "\\")
			p.advance()
		} else {
			break
		}
	}

	fullName := strings.Join(parts, "")
	alias := ""
	if p.peek().Value == "as" {
		p.advance()
		if t, ok := p.expect(TokenIdentifier); ok {
			alias = t.Value
		}
	}
	if alias == "" {
		segments := strings.Split(fullName, "\\")
		alias = segments[len(segments)-1]
	}
	p.result.Uses = append(p.result.Uses, UseStatement{
		FullName: fullName, Alias: alias, Kind: kind, Line: line,
	})
	p.expect(TokenSemicolon)
}

func (p *structParser) parseClass(docComment string) {
	cls := ClassDef{DocComment: docComment, Attributes: p.takeAttributes()}
	for {
		switch p.peek().Kind {
		case TokenAbstract:
			cls.IsAbstract = true
			p.advance()
		case TokenFinal:
			cls.IsFinal = true
			p.advance()
		case TokenReadonly:
			cls.IsReadonly = true
			p.advance()
		default:
			goto afterMod
		}
	}
afterMod:
	if _, ok := p.expect(TokenClass); !ok {
		return
	}
	if t, ok := p.expect(TokenIdentifier); ok {
		cls.Name = t.Value
		cls.Line = t.Line
		if p.result.Namespace != "" {
			cls.FullName = p.result.Namespace + "\\" + cls.Name
		} else {
			cls.FullName = cls.Name
		}
	}
	if p.peek().Kind == TokenExtends {
		p.advance()
		cls.Extends = p.readTypeName()
	}
	if p.peek().Kind == TokenImplements {
		p.advance()
		for {
			name := p.readTypeName()
			if name == "" {
				break
			}
			cls.Implements = append(cls.Implements, name)
			if p.peek().Kind == TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	cls.Methods, cls.Properties, cls.Constants, cls.Traits = p.parseClassBody()
	if cls.IsReadonly {
		for i := range cls.Properties {
			cls.Properties[i].IsReadonly = true
		}
	}
	cls.EndLine = p.peek().Line
	p.result.Classes = append(p.result.Classes, cls)
}

func (p *structParser) parseInterface() {
	p.advance()
	iface := InterfaceDef{Attributes: p.takeAttributes()}
	if t, ok := p.expect(TokenIdentifier); ok {
		iface.Name = t.Value
		iface.Line = t.Line
		if p.result.Namespace != "" {
			iface.FullName = p.result.Namespace + "\\" + iface.Name
		} else {
			iface.FullName = iface.Name
		}
	}
	if p.peek().Kind == TokenExtends {
		p.advance()
		for {
			name := p.readTypeName()
			if name == "" {
				break
			}
			iface.Extends = append(iface.Extends, name)
			if p.peek().Kind == TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	iface.Methods, iface.Properties, iface.Constants, _ = p.parseClassBody()
	iface.EndLine = p.peek().Line
	p.result.Interfaces = append(p.result.Interfaces, iface)
}

func (p *structParser) parseTrait() {
	p.advance()
	trait := TraitDef{Attributes: p.takeAttributes()}
	if t, ok := p.expect(TokenIdentifier); ok {
		trait.Name = t.Value
		trait.Line = t.Line
		if p.result.Namespace != "" {
			trait.FullName = p.result.Namespace + "\\" + trait.Name
		} else {
			trait.FullName = trait.Name
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	trait.Methods, trait.Properties, trait.Constants, trait.Traits = p.parseClassBody()
	trait.EndLine = p.peek().Line
	p.result.Traits = append(p.result.Traits, trait)
}

func (p *structParser) parseEnum(docComment string) {
	p.advance()
	e := EnumDef{DocComment: docComment, Attributes: p.takeAttributes()}
	if t, ok := p.expect(TokenIdentifier); ok {
		e.Name = t.Value
		e.Line = t.Line
		if p.result.Namespace != "" {
			e.FullName = p.result.Namespace + "\\" + e.Name
		} else {
			e.FullName = e.Name
		}
	}
	if p.peek().Kind == TokenColon {
		p.advance()
		e.BackedType = p.readTypeName()
	}
	if p.peek().Kind == TokenImplements {
		p.advance()
		for {
			name := p.readTypeName()
			if name == "" {
				break
			}
			e.Implements = append(e.Implements, name)
			if p.peek().Kind == TokenComma {
				p.advance()
			} else {
				break
			}
		}
	}
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return
	}
	p.enumCases = &e.Cases
	e.Methods, e.Properties, e.Constants, e.Traits = p.parseClassBody()
	p.enumCases = nil
	e.EndLine = p.peek().Line
	p.result.Enums = append(p.result.Enums, e)
}

func (p *structParser) parseFunction(docComment string) {
	p.advance()
	fn := FunctionDef{DocComment: docComment, Attributes: p.takeAttributes()}
	// By-reference function: function &get()
	if p.peek().Kind == TokenAmpersand {
		p.advance()
	}
	if t, ok := p.expect(TokenIdentifier); ok {
		fn.Name = t.Value
		fn.Line = t.Line
		if p.result.Namespace != "" {
			fn.FullName = p.result.Namespace + "\\" + fn.Name
		} else {
			fn.FullName = fn.Name
		}
	}
	fn.Params = p.parseParams()
	if p.peek().Kind == TokenColon {
		p.advance()
		fn.ReturnType = p.readTypeName()
	}
	bodyStart := p.pos
	p.skipBlock()
	bodyEnd := p.pos
	fn.EndLine = p.peek().Line
	p.result.Functions = append(p.result.Functions, fn)
	p.collectNestedFunctions(bodyStart, bodyEnd)
}

// collectNestedFunctions indexes named functions declared inside a function
// body, such as WordPress's helpers in export_wp(). PHP makes them global once
// the outer function runs. Closures and methods of anonymous classes are not
// named functions and are skipped.
func (p *structParser) collectNestedFunctions(start, end int) {
	saved := p.pos
	defer func() { p.pos = saved }()
	for i := start; i < end && i < len(p.tokens); i++ {
		switch p.tokens[i].Kind {
		case TokenClass:
			// Anonymous class: skip its body, whose functions are methods.
			j := i + 1
			for j < end && p.tokens[j].Kind != TokenOpenBrace {
				j++
			}
			depth := 0
			for ; j < end; j++ {
				if p.tokens[j].Kind == TokenOpenBrace {
					depth++
				} else if p.tokens[j].Kind == TokenCloseBrace {
					depth--
					if depth == 0 {
						break
					}
				}
			}
			i = j
		case TokenFunction:
			k := i + 1
			if k < len(p.tokens) && p.tokens[k].Kind == TokenAmpersand {
				k++ // function &name()
			}
			if k+1 >= len(p.tokens) || p.tokens[k].Kind != TokenIdentifier || p.tokens[k+1].Kind != TokenOpenParen {
				continue
			}
			doc := ""
			if i > 0 && p.tokens[i-1].Kind == TokenDocComment {
				doc = p.tokens[i-1].Value
			}
			p.pos = i
			p.parseFunction(doc)
			if p.pos <= i {
				continue // no progress; keep scanning
			}
			i = p.pos - 1
		}
	}
}

func (p *structParser) parseConst() {
	p.advance()
	c := ConstantDef{Line: p.peek().Line}
	if t, ok := p.expect(TokenIdentifier); ok {
		c.Name = t.Value
	}
	if p.peek().Kind == TokenEquals {
		p.advance()
		c.Value = p.peek().Value
		p.advance()
	}
	p.expect(TokenSemicolon)
	p.result.Constants = append(p.result.Constants, c)
}

func (p *structParser) parseClassBody() (methods []MethodDef, props []PropertyDef, consts []ConstantDef, traits []string) {
	depth := 1
	var docComment string
	var attributes []AttributeDef
	enumCases := p.enumCases
	for depth > 0 && p.peek().Kind != TokenEOF {
		// Recovery: at the outermost class level, bail on top-level keywords (missing closing brace)
		if depth == 1 && p.isRecoveryPoint() {
			p.result.Errors = append(p.result.Errors, ParseError{Message: "expected '}'", Line: p.peek().Line, Column: p.peek().Column})
			return
		}
		t := p.peek()
		if t.Kind == TokenDocComment {
			docComment = t.Value
			p.advance()
			continue
		}
		if t.Kind == TokenHash {
			attributes = append(attributes, p.parseAttributeGroup()...)
			continue
		}
		if enumCases != nil && depth == 1 && t.Kind == TokenIdentifier && strings.EqualFold(t.Value, "case") {
			p.advance()
			c := EnumCase{Line: p.peek().Line, DocComment: docComment, Attributes: attributes}
			if isNameToken(p.peek()) {
				c.Name = p.peek().Value
				p.advance()
			}
			if p.peek().Kind == TokenEquals {
				p.advance()
				c.Value = p.peek().Value
				p.skipUntil(TokenSemicolon)
			}
			*enumCases = append(*enumCases, c)
			p.expect(TokenSemicolon)
			docComment, attributes = "", nil
			continue
		}
		if t.Kind == TokenCloseBrace {
			depth--
			p.advance()
			continue
		}
		if t.Kind == TokenOpenBrace {
			depth++
			p.advance()
			continue
		}

		visibility := "public"
		setVisibility := ""
		visibilitySet := false // tracks whether we already have a primary visibility
		isStatic, isAbstract, isFinal, isReadonly := false, false, false, false
		for {
			switch p.peek().Kind {
			case TokenPublic, TokenProtected, TokenPrivate:
				vis := "public"
				if p.peek().Kind == TokenProtected {
					vis = "protected"
				} else if p.peek().Kind == TokenPrivate {
					vis = "private"
				}
				p.advance()
				// Check for PHP 8.4 asymmetric-visibility qualifier: vis(set) or vis(get).
				// When a qualifier is present this modifier describes set/write access only;
				// the previously set visibility is the read/primary access level.
				qual := p.tryConsumeAsymmetricQualifier()
				if qual != "" {
					// This is the secondary (write) visibility — record it and keep the
					// primary visibility unchanged.
					setVisibility = vis
				} else {
					// Primary visibility — only accept it if we haven't set one yet.
					if !visibilitySet {
						visibility = vis
						visibilitySet = true
					}
				}
			case TokenStatic:
				isStatic = true
				p.advance()
			case TokenAbstract:
				isAbstract = true
				p.advance()
			case TokenFinal:
				isFinal = true
				p.advance()
			case TokenReadonly:
				isReadonly = true
				p.advance()
			default:
				goto doneMod
			}
		}
	doneMod:
		// Recovery after modifiers: check if we hit a top-level keyword (e.g., "abstract class Foo" after missing brace)
		if depth == 1 && p.isRecoveryPoint() {
			p.result.Errors = append(p.result.Errors, ParseError{Message: "expected '}'", Line: p.peek().Line, Column: p.peek().Column})
			return
		}
		switch p.peek().Kind {
		case TokenUse:
			// Trait use statement inside class body: use Foo, Bar { ... };
			p.advance()
			var traitNames []string
			for {
				if p.peek().Kind == TokenIdentifier || p.peek().Kind == TokenBackslash {
					name := p.readTypeName()
					traitNames = append(traitNames, name)
				}
				if p.peek().Kind == TokenComma {
					p.advance()
					continue
				}
				break
			}
			// Handle optional conflict resolution block: { ... }
			if p.peek().Kind == TokenOpenBrace {
				innerDepth := 1
				p.advance()
				for innerDepth > 0 && p.peek().Kind != TokenEOF {
					if p.peek().Kind == TokenOpenBrace {
						innerDepth++
					} else if p.peek().Kind == TokenCloseBrace {
						innerDepth--
					}
					p.advance()
				}
			} else {
				p.expect(TokenSemicolon)
			}
			traits = append(traits, traitNames...)
		case TokenFunction:
			p.advance()
			m := MethodDef{
				Attributes: attributes,
				Visibility: visibility, IsStatic: isStatic,
				IsAbstract: isAbstract, IsFinal: isFinal,
				DocComment: docComment, Line: p.peek().Line,
			}
			// By-reference method: function &get()
			if p.peek().Kind == TokenAmpersand {
				p.advance()
			}
			// PHP allows reserved keywords as method names (e.g. class(), use(), match())
			if isNameToken(p.peek()) && p.peek().Kind != TokenOpenParen {
				m.Name = p.peek().Value
				p.advance()
			}
			m.Params = p.parseParams()
			if p.peek().Kind == TokenColon {
				p.advance()
				m.ReturnType = p.readTypeName()
			}
			if p.peek().Kind == TokenOpenBrace {
				innerDepth := 1
				p.advance()
				for innerDepth > 0 && p.peek().Kind != TokenEOF {
					if p.peek().Kind == TokenOpenBrace {
						innerDepth++
					} else if p.peek().Kind == TokenCloseBrace {
						innerDepth--
					}
					p.advance()
				}
			} else {
				p.expect(TokenSemicolon)
			}
			m.EndLine = p.peek().Line
			methods = append(methods, m)
			if strings.EqualFold(m.Name, "__construct") {
				props = append(props, promotedProperties(m.Params)...)
			}

		case TokenConst:
			p.advance()
			consts = append(consts, p.parseClassConstants(visibility, attributes)...)

		case TokenVariable:
			prop := PropertyDef{
				Attributes: attributes,
				Visibility: visibility, SetVisibility: setVisibility,
				IsStatic: isStatic, IsReadonly: isReadonly,
				DocComment: docComment, Line: p.peek().Line,
				Name: p.peek().Value,
			}
			p.advance()
			if p.peek().Kind == TokenEquals {
				prop.HasDefault = true
				p.advance()
				p.skipUntil(TokenSemicolon)
			}
			// PHP 8.4 property hooks: { get => ...; set => ...; }
			if p.peek().Kind == TokenOpenBrace {
				prop.Hooks = p.parsePropertyHooks()
			} else {
				p.expect(TokenSemicolon)
			}
			props = append(props, prop)

		case TokenIdentifier, TokenQuestion, TokenOpenParen:
			isNullable := false
			if p.peek().Kind == TokenQuestion {
				isNullable = true
				p.advance()
			}
			typeName := p.readTypeName()
			if isNullable {
				typeName = "?" + typeName
			}
			// Union/intersection types (TokenPipeArrow must NOT be consumed here)
			for p.peek().Kind == TokenPipe || p.peek().Kind == TokenAmpersand {
				op := p.peek().Value
				p.advance()
				typeName += op + p.readTypeName()
			}
			if p.peek().Kind == TokenVariable {
				prop := PropertyDef{
					Attributes: attributes,
					Visibility: visibility, SetVisibility: setVisibility,
					IsStatic: isStatic, IsReadonly: isReadonly,
					Type: typeName, DocComment: docComment,
					Line: p.peek().Line, Name: p.peek().Value,
				}
				p.advance()
				if p.peek().Kind == TokenEquals {
					prop.HasDefault = true
					p.advance()
					p.skipUntil(TokenSemicolon)
				}
				// PHP 8.4 property hooks: { get => ...; set => ...; }
				if p.peek().Kind == TokenOpenBrace {
					prop.Hooks = p.parsePropertyHooks()
				} else {
					p.expect(TokenSemicolon)
				}
				props = append(props, prop)
			} else {
				p.advance()
			}

		default:
			p.advance()
		}
		docComment, attributes = "", nil
	}
	return
}

// parseClassConstants reads the rest of a `const` statement, which may give a
// PHP 8.3 type and declare several constants: `const string A = 'a', B = 'b';`.
func (p *structParser) parseClassConstants(visibility string, attributes []AttributeDef) []ConstantDef {
	typeName := ""
	if !isNameToken(p.peek()) || (p.pos+1 < len(p.tokens) && p.tokens[p.pos+1].Kind != TokenEquals) {
		typeName = p.readTypeName()
	}
	var consts []ConstantDef
	for isNameToken(p.peek()) {
		c := ConstantDef{Name: p.peek().Value, Type: typeName, Visibility: visibility, Attributes: attributes, Line: p.peek().Line}
		p.advance()
		if p.peek().Kind == TokenEquals {
			p.advance()
			// Record the immediate value token for simple constants.
			if p.peek().Kind != TokenSemicolon && p.peek().Kind != TokenEOF {
				c.Value = p.peek().Value
			}
			// Consume the full value expression, including PHP 8.3 dynamic const
			// fetches like Colors::{$name} where '{' must not escape to the outer
			// class-body depth counter.
			p.skipConstantValue()
		}
		consts = append(consts, c)
		if p.peek().Kind != TokenComma {
			break
		}
		p.advance()
	}
	p.expect(TokenSemicolon)
	return consts
}

// skipConstantValue stops at the `,` or `;` ending a constant's value.
func (p *structParser) skipConstantValue() {
	depth := 0
	for p.peek().Kind != TokenEOF {
		switch p.peek().Kind {
		case TokenOpenBrace, TokenOpenParen, TokenOpenBracket:
			depth++
		case TokenCloseBrace, TokenCloseParen, TokenCloseBracket:
			if depth == 0 {
				return
			}
			depth--
		case TokenComma, TokenSemicolon:
			if depth == 0 {
				return
			}
		}
		p.advance()
	}
}

// isNameToken accepts any word, as members and constants can be named after
// reserved keywords: `case Default;`, `const LIST = 1;`.
func isNameToken(t Token) bool {
	if t.Kind == TokenVariable || t.Kind == TokenStringLiteral || t.Value == "" {
		return false
	}
	c := t.Value[0]
	return c == '_' || (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c >= 0x80
}

// promotedProperties returns the properties a constructor declares in its
// parameter list.
func promotedProperties(params []ParamDef) []PropertyDef {
	var props []PropertyDef
	for _, param := range params {
		if !param.IsPromoted || param.Name == "" {
			continue
		}
		props = append(props, PropertyDef{
			Name: param.Name, Type: param.Type, Visibility: param.Visibility, SetVisibility: param.SetVisibility,
			IsReadonly: param.IsReadonly, HasDefault: param.HasDefault, Hooks: param.Hooks,
			Attributes: param.Attributes, DocComment: param.DocComment, Line: param.Line,
		})
	}
	return props
}

func (p *structParser) parseParams() []ParamDef {
	if _, ok := p.expect(TokenOpenParen); !ok {
		return nil
	}
	var params []ParamDef
	for p.peek().Kind != TokenCloseParen && p.peek().Kind != TokenEOF {
		// Recovery: bail on braces (missing closing paren)
		if p.peek().Kind == TokenOpenBrace || p.peek().Kind == TokenCloseBrace {
			p.result.Errors = append(p.result.Errors, ParseError{Message: "expected ')'", Line: p.peek().Line, Column: p.peek().Column})
			return params
		}
		startPos := p.pos
		param := ParamDef{Visibility: "public"}
		for {
			switch p.peek().Kind {
			case TokenDocComment:
				param.DocComment = p.peek().Value
				p.advance()
			case TokenHash:
				param.Attributes = append(param.Attributes, p.parseAttributeGroup()...)
			case TokenPublic, TokenProtected, TokenPrivate:
				vis := strings.ToLower(p.peek().Value)
				p.advance()
				if p.tryConsumeAsymmetricQualifier() != "" {
					param.SetVisibility = vis
				} else {
					param.Visibility = vis
				}
				param.IsPromoted = true
			case TokenReadonly:
				param.IsPromoted = true
				param.IsReadonly = true
				p.advance()
			case TokenFinal:
				param.IsPromoted = true
				p.advance()
			default:
				goto doneParamMod
			}
		}
	doneParamMod:
		if p.peek().Kind == TokenQuestion {
			p.advance()
			param.Type = "?" + p.readTypeName()
		} else if p.peek().Kind == TokenIdentifier || p.peek().Kind == TokenBackslash || p.peek().Kind == TokenOpenParen {
			param.Type = p.readTypeName()
			for p.peek().Kind == TokenPipe || p.peek().Kind == TokenAmpersand {
				op := p.peek().Value
				p.advance()
				param.Type += op + p.readTypeName()
			}
		}
		// A by-reference marker reads like the start of an intersection type, so
		// it may already be on the end of the type.
		if strings.HasSuffix(param.Type, "&") {
			param.Type = strings.TrimSuffix(param.Type, "&")
			param.IsReference = true
		}
		if p.peek().Kind == TokenAmpersand {
			param.IsReference = true
			p.advance()
		}
		// The tokenizer emits "..." as three consecutive TokenUnknown "." tokens,
		// never as a single three-character token. Detect the triple-dot sequence
		// and consume all three tokens so that IsVariadic is correctly set for
		// both "...$x" and "Type ...$x" forms.
		if p.pos+2 < len(p.tokens) &&
			p.tokens[p.pos].Kind == TokenUnknown && p.tokens[p.pos].Value == "." &&
			p.tokens[p.pos+1].Kind == TokenUnknown && p.tokens[p.pos+1].Value == "." &&
			p.tokens[p.pos+2].Kind == TokenUnknown && p.tokens[p.pos+2].Value == "." {
			param.IsVariadic = true
			p.advance()
			p.advance()
			p.advance()
		}
		if p.peek().Kind == TokenVariable {
			param.Name = p.peek().Value
			param.Line = p.peek().Line
			p.advance()
		}
		if p.peek().Kind == TokenEquals {
			param.HasDefault = true
			p.advance()
			start := p.pos
			depth := 0
			for p.peek().Kind != TokenEOF {
				if k := p.peek().Kind; k == TokenOpenParen || k == TokenOpenBracket {
					depth++
				} else if k == TokenCloseParen || k == TokenCloseBracket {
					if depth == 0 {
						break
					}
					depth--
				} else if k == TokenComma && depth == 0 {
					break
				}
				p.advance()
			}
			param.DefaultValue = tokensText(p.tokens[start:p.pos])
		}
		// PHP 8.4 hooks on a promoted property.
		if p.peek().Kind == TokenOpenBrace && param.IsPromoted {
			param.Hooks = p.parsePropertyHooks()
		}
		params = append(params, param)
		if p.peek().Kind == TokenComma {
			p.advance()
		}
		// Safety: ensure forward progress
		if p.pos == startPos {
			p.advance()
		}
	}
	p.expect(TokenCloseParen)
	return params
}

func (p *structParser) readTypeName() string {
	// Handle nullable prefix
	nullable := ""
	if p.peek().Kind == TokenQuestion {
		nullable = "?"
		p.advance()
	}
	var parts []string
	for {
		t := p.peek()
		// Two names in a row are a type and what it types, as in `const string NAME`.
		afterName := false
		if len(parts) > 0 {
			last := parts[len(parts)-1]
			afterName = last != "\\" && last != "|" && last != "&"
		}
		if t.Kind == TokenIdentifier || isTypeKeyword(t.Kind) {
			if afterName {
				break
			}
			parts = append(parts, t.Value)
			p.advance()
		} else if t.Kind == TokenBackslash {
			parts = append(parts, "\\")
			p.advance()
		} else if t.Kind == TokenPipe || t.Kind == TokenAmpersand {
			// Union (A|B) or intersection (A&B) types
			parts = append(parts, t.Value)
			p.advance()
		} else if group := p.readDNFGroup(afterName); group != "" {
			parts = append(parts, group)
		} else {
			break
		}
	}
	return nullable + strings.Join(parts, "")
}

// readDNFGroup consumes the `(A&B)` of a PHP 8.2 DNF type. Anything else in
// parentheses is left alone, as it's not a type.
func (p *structParser) readDNFGroup(afterName bool) string {
	if afterName || p.peek().Kind != TokenOpenParen {
		return ""
	}
	group := "("
	hasIntersection := false
	for i := p.pos + 1; i < len(p.tokens); i++ {
		t := p.tokens[i]
		switch t.Kind {
		case TokenIdentifier, TokenBackslash:
			group += t.Value
		case TokenAmpersand:
			hasIntersection = true
			group += t.Value
		case TokenCloseParen:
			if !hasIntersection {
				return ""
			}
			p.pos = i + 1
			return group + ")"
		default:
			return ""
		}
	}
	return ""
}

// isTypeKeyword returns true for keyword tokens that are valid as type names
// in PHP (self, static, parent, array, callable, etc.) but NOT structural
// keywords like class, extends, implements, function, etc.
func isTypeKeyword(kind TokenKind) bool {
	switch kind {
	case TokenStatic, TokenConst:
		return true
	}
	return false
}

// tryConsumeAsymmetricQualifier checks for a PHP 8.4 asymmetric-visibility
// qualifier immediately after a visibility keyword: e.g. private(set) or
// public(get). If found, it consumes the "(set)"/"(get)" tokens and returns
// the qualifier name ("set" or "get"). Otherwise it returns "" and leaves the
// token stream unchanged.
func (p *structParser) tryConsumeAsymmetricQualifier() string {
	// We need: TokenOpenParen, then an identifier whose value is "set" or "get",
	// then TokenCloseParen. If any of those are missing we leave the stream as-is.
	if p.peek().Kind != TokenOpenParen {
		return ""
	}
	// Peek two more tokens ahead to confirm the pattern ( "set"|"get" )
	nextPos := p.pos + 1
	if nextPos >= len(p.tokens) {
		return ""
	}
	qualifier := p.tokens[nextPos]
	if qualifier.Kind != TokenIdentifier || (qualifier.Value != "set" && qualifier.Value != "get") {
		return ""
	}
	closePos := nextPos + 1
	if closePos >= len(p.tokens) || p.tokens[closePos].Kind != TokenCloseParen {
		return ""
	}
	// Consume '(' qualifier ')'
	p.advance()               // (
	name := p.advance().Value // set | get
	p.advance()               // )
	return name
}

// parsePropertyHooks parses a PHP 8.4 property-hook block of the form:
//
//	{ get => <expr>; set => <expr>; }
//	{ get { <body> } set { <body> } }
//	{ get; }  (interface abstract hook)
//
// It consumes the entire block, records the hook kinds in the returned slice,
// and always makes forward progress. The implementation is brace-balanced so
// nested braces inside hook bodies do not confuse the outer class-body parser.
func (p *structParser) parsePropertyHooks() []PropertyHook {
	var hooks []PropertyHook
	if _, ok := p.expect(TokenOpenBrace); !ok {
		return hooks
	}
	depth := 1
	for depth > 0 && p.peek().Kind != TokenEOF {
		t := p.peek()
		switch t.Kind {
		case TokenOpenBrace:
			depth++
			p.advance()
		case TokenCloseBrace:
			depth--
			p.advance()
		case TokenIdentifier:
			if depth == 1 && (t.Value == "get" || t.Value == "set") {
				// Record the hook kind then consume the rest of this hook's body.
				hooks = append(hooks, PropertyHook{Kind: t.Value})
				p.advance() // consume "get" or "set"
				// Abstract hook in interface: get; or set;
				if p.peek().Kind == TokenSemicolon {
					p.advance()
					continue
				}
				// Short hook: get => <expr>; or set => <expr>;
				if p.peek().Kind == TokenDoubleArrow {
					p.advance() // =>
					// Skip the expression until semicolon, respecting nested braces
					exprDepth := 0
					for p.peek().Kind != TokenEOF {
						switch p.peek().Kind {
						case TokenOpenBrace:
							exprDepth++
						case TokenCloseBrace:
							if exprDepth == 0 {
								// End of hook block — don't consume, the outer depth-- will fire
								goto hookExprDone
							}
							exprDepth--
						case TokenSemicolon:
							if exprDepth == 0 {
								p.advance() // consume ;
								goto hookExprDone
							}
						}
						p.advance()
					}
				hookExprDone:
				} else if p.peek().Kind == TokenOpenBrace {
					// Long hook: set { <body> }
					hookDepth := 1
					p.advance() // {
					for hookDepth > 0 && p.peek().Kind != TokenEOF {
						switch p.peek().Kind {
						case TokenOpenBrace:
							hookDepth++
						case TokenCloseBrace:
							hookDepth--
						}
						p.advance()
					}
				}
			} else {
				p.advance()
			}
		default:
			p.advance()
		}
	}
	return hooks
}
