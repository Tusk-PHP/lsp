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

type CursorContext struct {
	URI             string
	Position        protocol.Position
	Namespace       string
	Uses            []parser.UseNode
	SymbolText      string
	SymbolKind      ContextSymbolKind
	AccessKind      AccessKind
	SubjectExpr     string
	EnclosingFQN    string
	Scope           *Scope
	WordRange       protocol.Range
	Line            string
	JoinedLine      string
	JoinedWordStart int
	File            *parser.FileNode
}

type MemberContext struct {
	Prefix     string
	Filter     string
	AccessKind AccessKind
}

func Analyze(uri, source string, pos protocol.Position) *CursorContext {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return nil
	}

	result := parser.New().Parse(source)
	file := parser.ParseFile(source)
	word, wordRange := WordAt(source, pos)
	line := lines[pos.Line]
	joinedLine, joinedWordStart := resolve.JoinChainLines(lines, pos.Line, wordRange.Start.Character)
	accessKind, subjectExpr := detectAccess(joinedLine, joinedWordStart)
	namespace, uses := namespaceUsesAt(result, pos)
	enclosingFQN := enclosingClassAt(result, pos)

	ctx := &CursorContext{
		URI:             uri,
		Position:        pos,
		Namespace:       namespace,
		Uses:            uses,
		SymbolText:      word,
		SymbolKind:      classifySymbolKind(word),
		AccessKind:      accessKind,
		SubjectExpr:     subjectExpr,
		EnclosingFQN:    enclosingFQN,
		WordRange:       wordRange,
		Line:            line,
		JoinedLine:      joinedLine,
		JoinedWordStart: joinedWordStart,
		File:            file,
	}
	ctx.Scope = scopeAt(result, pos, ctx.EnclosingFQN)
	return ctx
}

func Namespace(source string) string {
	return namespaceFromFile(parser.ParseFile(source))
}

func WordAt(source string, pos protocol.Position) (string, protocol.Range) {
	lines := strings.Split(source, "\n")
	if pos.Line < 0 || pos.Line >= len(lines) {
		return "", protocol.Range{}
	}
	line := lines[pos.Line]
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

func namespaceFromFile(file *parser.FileNode) string {
	if file == nil {
		return ""
	}
	return file.Namespace
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
	for _, cls := range result.Classes {
		if !lineInRange(pos.Line, cls.Line, cls.EndLine) {
			continue
		}
		for _, method := range cls.Methods {
			if !lineInRange(pos.Line, method.Line, method.EndLine) {
				continue
			}
			return &Scope{
				Kind:       ScopeMethod,
				Name:       method.Name,
				ClassFQN:   classFQN,
				MethodName: method.Name,
				StartLine:  method.Line,
				EndLine:    method.EndLine,
			}
		}
	}

	for _, trait := range result.Traits {
		if !lineInRange(pos.Line, trait.Line, trait.EndLine) {
			continue
		}
		for _, method := range trait.Methods {
			if !lineInRange(pos.Line, method.Line, method.EndLine) {
				continue
			}
			return &Scope{
				Kind:       ScopeMethod,
				Name:       method.Name,
				ClassFQN:   classFQN,
				MethodName: method.Name,
				StartLine:  method.Line,
				EndLine:    method.EndLine,
			}
		}
	}

	for _, enum := range result.Enums {
		if !lineInRange(pos.Line, enum.Line, enum.EndLine) {
			continue
		}
		for _, method := range enum.Methods {
			if !lineInRange(pos.Line, method.Line, method.EndLine) {
				continue
			}
			return &Scope{
				Kind:       ScopeMethod,
				Name:       method.Name,
				ClassFQN:   classFQN,
				MethodName: method.Name,
				StartLine:  method.Line,
				EndLine:    method.EndLine,
			}
		}
	}

	return nil
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
		next := nextSignificantToken(result.Tokens, i+1)
		if next < 0 || result.Tokens[next].Kind == parser.TokenIdentifier {
			continue
		}
		if result.Tokens[next].Kind != parser.TokenOpenParen {
			continue
		}

		closeParen := findMatchingToken(result.Tokens, next, parser.TokenOpenParen, parser.TokenCloseParen)
		if closeParen < 0 {
			continue
		}

		body := nextSignificantToken(result.Tokens, closeParen+1)
		if body >= 0 && result.Tokens[body].Kind == parser.TokenUse {
			useOpen := nextSignificantToken(result.Tokens, body+1)
			if useOpen >= 0 && result.Tokens[useOpen].Kind == parser.TokenOpenParen {
				useClose := findMatchingToken(result.Tokens, useOpen, parser.TokenOpenParen, parser.TokenCloseParen)
				if useClose < 0 {
					continue
				}
				body = nextSignificantToken(result.Tokens, useClose+1)
			}
		}
		for body >= 0 && body < len(result.Tokens) &&
			result.Tokens[body].Kind != parser.TokenOpenBrace &&
			result.Tokens[body].Kind != parser.TokenDoubleArrow &&
			result.Tokens[body].Kind != parser.TokenSemicolon {
			body = nextSignificantToken(result.Tokens, body+1)
		}
		if body >= 0 && result.Tokens[body].Kind == parser.TokenDoubleArrow {
			bodyStart := nextSignificantToken(result.Tokens, body+1)
			bodyEnd := arrowExpressionEnd(result.Tokens, bodyStart)
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

		closeBrace := findMatchingToken(result.Tokens, body, parser.TokenOpenBrace, parser.TokenCloseBrace)
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

func enclosingClassAt(result *parser.ParseResult, pos protocol.Position) string {
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
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
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
	i := nextSignificantToken(tokens, start+1)
	if i < 0 {
		return parser.UseNode{}, false
	}
	if tokens[i].Kind == parser.TokenFunction {
		kind = "function"
		i = nextSignificantToken(tokens, i+1)
	} else if tokens[i].Kind == parser.TokenConst {
		kind = "const"
		i = nextSignificantToken(tokens, i+1)
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
				aliasIdx := nextSignificantToken(tokens, i+1)
				if aliasIdx >= 0 && tokens[aliasIdx].Kind == parser.TokenIdentifier {
					alias = tokens[aliasIdx].Value
				}
				i = aliasIdx
				continue
			}
			parts = append(parts, tokens[i].Value)
		case parser.TokenBackslash:
			parts = append(parts, "\\")
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
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
	prev := prevSignificantToken(tokens, idx-1)
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

func nextSignificantToken(tokens []parser.Token, i int) int {
	for i < len(tokens) {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i++
		default:
			return i
		}
	}
	return -1
}

func prevSignificantToken(tokens []parser.Token, i int) int {
	for i >= 0 {
		switch tokens[i].Kind {
		case parser.TokenWhitespace, parser.TokenComment, parser.TokenDocComment:
			i--
		default:
			return i
		}
	}
	return -1
}

func findMatchingToken(tokens []parser.Token, start int, openKind, closeKind parser.TokenKind) int {
	depth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case openKind:
			depth++
		case closeKind:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

func arrowExpressionEnd(tokens []parser.Token, start int) int {
	if start < 0 {
		return -1
	}
	parenDepth := 0
	bracketDepth := 0
	braceDepth := 0
	for i := start; i < len(tokens); i++ {
		switch tokens[i].Kind {
		case parser.TokenOpenParen:
			parenDepth++
		case parser.TokenCloseParen:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return prevSignificantToken(tokens, i-1)
			}
			if parenDepth > 0 {
				parenDepth--
			}
		case parser.TokenOpenBracket:
			bracketDepth++
		case parser.TokenCloseBracket:
			if bracketDepth == 0 && parenDepth == 0 && braceDepth == 0 {
				return prevSignificantToken(tokens, i-1)
			}
			if bracketDepth > 0 {
				bracketDepth--
			}
		case parser.TokenOpenBrace:
			braceDepth++
		case parser.TokenCloseBrace:
			if braceDepth == 0 && parenDepth == 0 && bracketDepth == 0 {
				return prevSignificantToken(tokens, i-1)
			}
			if braceDepth > 0 {
				braceDepth--
			}
		case parser.TokenComma, parser.TokenSemicolon:
			if parenDepth == 0 && bracketDepth == 0 && braceDepth == 0 {
				return prevSignificantToken(tokens, i-1)
			}
		}
	}
	return prevSignificantToken(tokens, len(tokens)-1)
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
