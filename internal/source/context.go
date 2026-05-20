package sourcectx

import (
	"strings"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/resolve"
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

type Scope struct {
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

	file := parser.ParseFile(source)
	word, wordRange := WordAt(source, pos)
	line := lines[pos.Line]
	joinedLine, joinedWordStart := resolve.JoinChainLines(lines, pos.Line, wordRange.Start.Character)
	accessKind, subjectExpr := detectAccess(joinedLine, joinedWordStart)

	ctx := &CursorContext{
		URI:             uri,
		Position:        pos,
		Namespace:       namespaceFromFile(file),
		SymbolText:      word,
		SymbolKind:      classifySymbolKind(word),
		AccessKind:      accessKind,
		SubjectExpr:     subjectExpr,
		EnclosingFQN:    resolve.FindEnclosingClass(file, pos),
		WordRange:       wordRange,
		Line:            line,
		JoinedLine:      joinedLine,
		JoinedWordStart: joinedWordStart,
		File:            file,
	}
	if file != nil {
		ctx.Uses = file.Uses
	}
	ctx.Scope = scopeAt(file, pos, ctx.EnclosingFQN)
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

func scopeAt(file *parser.FileNode, pos protocol.Position, classFQN string) *Scope {
	if file == nil {
		return nil
	}
	scope := &Scope{ClassFQN: classFQN}
	if method := resolve.FindEnclosingMethod(file, pos); method != nil {
		scope.MethodName = method.Name
		scope.StartLine = method.StartLine
		scope.EndLine = method.EndLine
	}
	if scope.ClassFQN == "" && scope.MethodName == "" && scope.StartLine == 0 && scope.EndLine == 0 {
		return nil
	}
	return scope
}

func isMemberFilter(filter string) bool {
	return filter != "" && !strings.ContainsAny(filter, " \t(=;,")
}
