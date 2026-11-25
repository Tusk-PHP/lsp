package sourcectx

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
)

type ContextSymbolKind string

const (
	ContextSymbolUnknown    ContextSymbolKind = "unknown"
	ContextSymbolIdentifier ContextSymbolKind = "identifier"
	ContextSymbolVariable   ContextSymbolKind = "variable"
)

type AccessKind string

const (
	AccessNone     AccessKind = "none"
	AccessInstance AccessKind = "instance"
	AccessNullsafe AccessKind = "nullsafe"
	AccessStatic   AccessKind = "static"
)

type ScopeKind string

const (
	ScopeUnknown  ScopeKind = "unknown"
	ScopeFunction ScopeKind = "function"
	ScopeMethod   ScopeKind = "method"
	ScopeClosure  ScopeKind = "closure"
)

type Scope struct {
	Kind       ScopeKind
	Name       string
	ClassFQN   string
	MethodName string
	StartLine  int
	EndLine    int
}

type NameContext struct {
	URI             string
	Position        protocol.Position
	SymbolText      string
	SymbolKind      ContextSymbolKind
	AccessKind      AccessKind
	SubjectExpr     string
	EnclosingFQN    string
	WordRange       protocol.Range
	Line            string
	JoinedLine      string
	JoinedWordStart int
	File            *parser.FileNode
	Called          bool
	FunctionCall    bool
}

type CursorContext struct {
	NameContext
	Namespace string
	Uses      []parser.UseNode
	Scope     *Scope
}

func (c *CursorContext) Name() *NameContext {
	if c == nil {
		return nil
	}
	return &c.NameContext
}

type MemberContext struct {
	Prefix     string
	Filter     string
	AccessKind AccessKind
}

type Document struct {
	uri    string
	lines  []string
	result *parser.ParseResult
	file   *parser.FileNode
}

func NewDocument(uri, source string) *Document {
	return &Document{uri: uri, lines: strings.Split(source, "\n"), result: parser.New().Parse(source), file: parser.ParseFile(source)}
}

func (d *Document) Tokens() []parser.Token {
	return d.result.Tokens
}

func (d *Document) File() *parser.FileNode {
	return d.file
}

func Analyze(uri, source string, pos protocol.Position) *CursorContext {
	return NewDocument(uri, source).At(pos)
}

func (d *Document) At(pos protocol.Position) *CursorContext {
	name := d.NameAt(pos)
	if name == nil {
		return nil
	}
	ctx := &CursorContext{NameContext: *name}
	ctx.Namespace, ctx.Uses = namespaceUsesAt(d.result, pos)
	ctx.Scope = scopeAt(d.result, pos, ctx.EnclosingFQN)
	return ctx
}

func (d *Document) NameAt(pos protocol.Position) *NameContext {
	if pos.Line < 0 || pos.Line >= len(d.lines) {
		return nil
	}
	line := d.lines[pos.Line]
	word, wordRange := wordAtLine(line, pos)
	joinedLine, joinedWordStart := resolve.JoinChainLines(d.lines, pos.Line, wordRange.Start.Character)
	accessKind, subjectExpr := detectAccess(joinedLine, joinedWordStart)
	called := isCalled(d.result.Tokens, wordRange)
	return &NameContext{
		URI:             d.uri,
		Position:        pos,
		SymbolText:      word,
		SymbolKind:      classifySymbolKind(word),
		AccessKind:      accessKind,
		SubjectExpr:     subjectExpr,
		EnclosingFQN:    enclosingClassAt(d.result, d.file, pos),
		WordRange:       wordRange,
		Line:            line,
		JoinedLine:      joinedLine,
		JoinedWordStart: joinedWordStart,
		File:            d.file,
		Called:          called,
		FunctionCall:    called && accessKind == AccessNone,
	}
}

func WordAt(source string, pos protocol.Position) (string, protocol.Range) {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return "", protocol.Range{}
	}
	return wordAtLine(lines[pos.Line], pos)
}

func wordAtLine(line string, pos protocol.Position) (string, protocol.Range) {
	if pos.Character > len(line) {
		return "", protocol.Range{}
	}

	ch := pos.Character
	if ch < len(line) && line[ch] == '$' {
		start := ch
		end := ch + 1
		for end < len(line) && resolve.IsWordChar(line[end]) {
			end++
		}
		if end > start+1 {
			return line[start:end], protocol.Range{
				Start: protocol.Position{Line: pos.Line, Character: start},
				End:   protocol.Position{Line: pos.Line, Character: end},
			}
		}
		return "", protocol.Range{}
	}

	start := pos.Character
	for start > 0 && resolve.IsWordChar(line[start-1]) {
		start--
	}
	if start > 0 && line[start-1] == '$' {
		start--
	}
	end := pos.Character
	for end < len(line) && resolve.IsWordChar(line[end]) {
		end++
	}
	if start >= end {
		return "", protocol.Range{}
	}
	return line[start:end], protocol.Range{
		Start: protocol.Position{Line: pos.Line, Character: start},
		End:   protocol.Position{Line: pos.Line, Character: end},
	}
}

func DetectMemberContext(trimmed string) (MemberContext, bool) {
	for i := len(trimmed) - 1; i >= 2; i-- {
		if i >= 2 && trimmed[i-2] == '?' && trimmed[i-1] == '-' && trimmed[i] == '>' {
			filter := trimmed[i+1:]
			if isMemberFilter(filter) {
				return MemberContext{Prefix: trimmed[:i+1], Filter: filter, AccessKind: AccessNullsafe}, true
			}
		}
		if trimmed[i-1] == '-' && trimmed[i] == '>' {
			filter := trimmed[i+1:]
			if isMemberFilter(filter) {
				return MemberContext{Prefix: trimmed[:i+1], Filter: filter, AccessKind: AccessInstance}, true
			}
		}
		if trimmed[i-1] == ':' && trimmed[i] == ':' {
			filter := trimmed[i+1:]
			if isMemberFilter(filter) {
				return MemberContext{Prefix: trimmed[:i+1], Filter: filter, AccessKind: AccessStatic}, true
			}
		}
	}
	return MemberContext{}, false
}

func detectAccess(line string, wordStart int) (AccessKind, string) {
	i := wordStart
	for i > 0 && (line[i-1] == ' ' || line[i-1] == '\t') {
		i--
	}
	if i >= 3 && line[i-3] == '?' && line[i-2] == '-' && line[i-1] == '>' {
		return AccessNullsafe, strings.TrimSpace(line[:i-3])
	}
	if i >= 2 && line[i-2] == '-' && line[i-1] == '>' {
		return AccessInstance, strings.TrimSpace(line[:i-2])
	}
	if i >= 2 && line[i-2] == ':' && line[i-1] == ':' {
		return AccessStatic, strings.TrimSpace(line[:i-2])
	}
	return AccessNone, ""
}

func classifySymbolKind(word string) ContextSymbolKind {
	if strings.HasPrefix(word, "$") {
		return ContextSymbolVariable
	}
	if word != "" {
		return ContextSymbolIdentifier
	}
	return ContextSymbolUnknown
}

func isMemberFilter(filter string) bool {
	return filter != "" && !strings.ContainsAny(filter, " \t(=;,")
}

func scopeAt(result *parser.ParseResult, pos protocol.Position, classFQN string) *Scope {
	if result == nil {
		return nil
	}

	scope := closureScopeAt(result, pos, classFQN)
	if methodScope := methodScopeAt(result, pos, classFQN); methodScope != nil {
		if scope == nil || methodScope.StartLine >= scope.StartLine {
			scope = methodScope
		}
	}
	if functionScope := functionScopeAt(result, pos); functionScope != nil {
		if scope == nil || functionScope.StartLine >= scope.StartLine {
			scope = functionScope
		}
	}

	return scope
}

func methodScopeAt(result *parser.ParseResult, pos protocol.Position, classFQN string) *Scope {
	var best *parser.MethodDef
	consider := func(line, endLine int, methods []parser.MethodDef) {
		if !lineInRange(pos.Line, line, endLine) {
			return
		}
		for i := range methods {
			m := &methods[i]
			if lineInRange(pos.Line, m.Line, m.EndLine) && (best == nil || m.Line > best.Line) {
				best = m
			}
		}
	}
	for _, cls := range result.Classes {
		consider(cls.Line, cls.EndLine, cls.Methods)
	}
	for _, cls := range result.AnonymousClasses {
		consider(cls.Line, cls.EndLine, cls.Methods)
	}
	for _, trait := range result.Traits {
		consider(trait.Line, trait.EndLine, trait.Methods)
	}
	for _, enum := range result.Enums {
		consider(enum.Line, enum.EndLine, enum.Methods)
	}
	if best == nil {
		return nil
	}
	return &Scope{
		Kind:       ScopeMethod,
		Name:       best.Name,
		ClassFQN:   classFQN,
		MethodName: best.Name,
		StartLine:  best.Line,
		EndLine:    best.EndLine,
	}
}

func functionScopeAt(result *parser.ParseResult, pos protocol.Position) *Scope {
	for _, fn := range result.Functions {
		if !lineInRange(pos.Line, fn.Line, fn.EndLine) {
			continue
		}
		return &Scope{
			Kind:      ScopeFunction,
			Name:      fn.Name,
			StartLine: fn.Line,
			EndLine:   fn.EndLine,
		}
	}
	return nil
}

func closureScopeAt(result *parser.ParseResult, pos protocol.Position, classFQN string) *Scope {
	var best *Scope
	for i := 0; i < len(result.Tokens); i++ {
		if result.Tokens[i].Kind != parser.TokenFunction {
			continue
		}
		next := parser.NextSignificant(result.Tokens, i+1)
		if next < 0 || result.Tokens[next].Kind == parser.TokenIdentifier {
			continue
		}
		if result.Tokens[next].Kind != parser.TokenOpenParen {
			continue
		}

		closeParen := parser.MatchingClose(result.Tokens, next)
		if closeParen < 0 {
			continue
		}

		body := parser.NextSignificant(result.Tokens, closeParen+1)
		if body >= 0 && result.Tokens[body].Kind == parser.TokenUse {
			useOpen := parser.NextSignificant(result.Tokens, body+1)
			if useOpen >= 0 && result.Tokens[useOpen].Kind == parser.TokenOpenParen {
				useClose := parser.MatchingClose(result.Tokens, useOpen)
				if useClose < 0 {
					continue
				}
				body = parser.NextSignificant(result.Tokens, useClose+1)
			}
		}
		for body >= 0 && body < len(result.Tokens) &&
			result.Tokens[body].Kind != parser.TokenOpenBrace &&
			result.Tokens[body].Kind != parser.TokenDoubleArrow &&
			result.Tokens[body].Kind != parser.TokenSemicolon {
			body = parser.NextSignificant(result.Tokens, body+1)
		}
		if body >= 0 && result.Tokens[body].Kind == parser.TokenDoubleArrow {
			bodyStart := parser.NextSignificant(result.Tokens, body+1)
			bodyEnd := parser.ArrowExpressionEnd(result.Tokens, bodyStart)
			if bodyStart < 0 || bodyEnd < 0 {
				continue
			}

			start := result.Tokens[i]
			end := result.Tokens[bodyEnd]
			if !posInTokenRange(pos, start, end) {
				continue
			}

			candidate := &Scope{
				Kind:      ScopeClosure,
				ClassFQN:  classFQN,
				StartLine: start.Line,
				EndLine:   end.Line,
			}
			if best == nil || candidate.StartLine >= best.StartLine {
				best = candidate
			}
			continue
		}

		if body < 0 || result.Tokens[body].Kind != parser.TokenOpenBrace {
			continue
		}

		closeBrace := parser.MatchingClose(result.Tokens, body)
		if closeBrace < 0 {
			continue
		}

		startLine := result.Tokens[i].Line
		endLine := result.Tokens[closeBrace].Line
		if !lineInRange(pos.Line, startLine, endLine) {
			continue
		}

		candidate := &Scope{
			Kind:      ScopeClosure,
			ClassFQN:  classFQN,
			StartLine: startLine,
			EndLine:   endLine,
		}
		if best == nil || candidate.StartLine >= best.StartLine {
			best = candidate
		}
	}
	return best
}

func namespaceUsesAt(result *parser.ParseResult, pos protocol.Position) (string, []parser.UseNode) {
	if result == nil {
		return "", nil
	}

	namespace := ""
	var uses []parser.UseNode
	braceDepth := 0
	namespaceDepth := 0
	namespaceBraced := false
	pendingNamespaceBrace := false

	for i := 0; i < len(result.Tokens); i++ {
		tok := result.Tokens[i]
		if tokenStartsAfterPos(tok, pos) {
			break
		}

		switch tok.Kind {
		case parser.TokenNamespace:
			if braceDepth == 0 {
				namespace, namespaceBraced, pendingNamespaceBrace = parseNamespaceState(result.Tokens, i)
				namespaceDepth = 0
				uses = nil
			}
		case parser.TokenUse:
			if isTopLevelImportUse(result.Tokens, i, braceDepth, namespaceDepth) {
				if useNode, ok := parseUseNode(result.Tokens, i); ok {
					uses = append(uses, useNode)
				}
			}
		case parser.TokenOpenBrace:
			braceDepth++
			if pendingNamespaceBrace {
				namespaceDepth = braceDepth
				pendingNamespaceBrace = false
			}
		case parser.TokenCloseBrace:
			if namespaceBraced && braceDepth == namespaceDepth {
				namespace = ""
				uses = nil
				namespaceBraced = false
				namespaceDepth = 0
			}
			if braceDepth > 0 {
				braceDepth--
			}
		}
	}

	return namespace, uses
}

func enclosingClassAt(result *parser.ParseResult, file *parser.FileNode, pos protocol.Position) string {
	if anon := file.AnonymousClassAt(pos.Line, pos.Character); anon != nil {
		return anon.FullName
	}
	if result == nil {
		return ""
	}

	for _, cls := range result.Classes {
		if lineInRange(pos.Line, cls.Line, cls.EndLine) {
			return cls.FullName
		}
	}
	for _, trait := range result.Traits {
		if lineInRange(pos.Line, trait.Line, trait.EndLine) {
			return trait.FullName
		}
	}
	for _, enum := range result.Enums {
		if lineInRange(pos.Line, enum.Line, enum.EndLine) {
			return enum.FullName
		}
	}
	for _, iface := range result.Interfaces {
		if lineInRange(pos.Line, iface.Line, iface.EndLine) {
			return iface.FullName
		}
	}

	return ""
}

func parseNamespaceState(tokens []parser.Token, start int) (string, bool, bool) {
	var parts []string
	for i := start + 1; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenIdentifier:
			parts = append(parts, tokens[i].Value)
		case parser.TokenBackslash:
			parts = append(parts, "\\")
		case parser.TokenComment, parser.TokenDocComment:
			continue
		case parser.TokenOpenBrace:
			return strings.Join(parts, ""), true, true
		case parser.TokenSemicolon:
			return strings.Join(parts, ""), false, false
		default:
			return strings.Join(parts, ""), false, false
		}
	}
	return strings.Join(parts, ""), false, false
}

func parseUseNode(tokens []parser.Token, start int) (parser.UseNode, bool) {
	kind := "class"
	i := parser.NextSignificant(tokens, start+1)
	if i < 0 {
		return parser.UseNode{}, false
	}
	if tokens[i].Kind == parser.TokenFunction {
		kind = "function"
		i = parser.NextSignificant(tokens, i+1)
	} else if tokens[i].Kind == parser.TokenConst {
		kind = "const"
		i = parser.NextSignificant(tokens, i+1)
	}

	if i < 0 {
		return parser.UseNode{}, false
	}

	var parts []string
	line := tokens[start].Line
	alias := ""
	for ; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenIdentifier:
			if tokens[i].Value == "as" {
				aliasIdx := parser.NextSignificant(tokens, i+1)
				if aliasIdx >= 0 && tokens[aliasIdx].Kind == parser.TokenIdentifier {
					alias = tokens[aliasIdx].Value
				}
				i = aliasIdx
				continue
			}
			parts = append(parts, tokens[i].Value)
		case parser.TokenBackslash:
			parts = append(parts, "\\")
		case parser.TokenComment, parser.TokenDocComment:
			continue
		case parser.TokenSemicolon:
			fullName := strings.Join(parts, "")
			if fullName == "" {
				return parser.UseNode{}, false
			}
			if alias == "" {
				segments := strings.Split(fullName, "\\")
				alias = segments[len(segments)-1]
			}
			return parser.UseNode{FullName: fullName, Alias: alias, Kind: kind, StartLine: line}, true
		default:
			return parser.UseNode{}, false
		}
	}

	return parser.UseNode{}, false
}

func isTopLevelImportUse(tokens []parser.Token, idx, braceDepth, namespaceDepth int) bool {
	if braceDepth != namespaceDepth {
		return false
	}
	prev := parser.PrevSignificant(tokens, idx-1)
	if prev < 0 {
		return true
	}
	switch tokens[prev].Kind {
	case parser.TokenPHP, parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace:
		return true
	default:
		return false
	}
}

func posInTokenRange(pos protocol.Position, start, end parser.Token) bool {
	if pos.Line < start.Line || pos.Line > end.Line {
		return false
	}
	if pos.Line == start.Line && pos.Character < start.Column {
		return false
	}
	if pos.Line == end.Line && pos.Character > end.Column+len(end.Value) {
		return false
	}
	return true
}

func tokenStartsAfterPos(tok parser.Token, pos protocol.Position) bool {
	if tok.Line > pos.Line {
		return true
	}
	if tok.Line < pos.Line {
		return false
	}
	return tok.Column > pos.Character
}

func lineInRange(line, start, end int) bool {
	return line >= start && line <= end
}
