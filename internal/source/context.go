package sourcectx

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
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

func (c *NameContext) MemberAccess() resolve.MemberAccess {
	return resolve.MemberAccessOf(c.Called, c.AccessKind == AccessStatic, c.SymbolText)
}

type MemberContext struct {
	Prefix     string
	Filter     string
	AccessKind AccessKind
}

type Document struct {
	uri    string
	source string
	lines  []string
	result *parser.ParseResult
	tree   *ast.File
	file   *parser.FileNode
}

func NewDocument(uri, source string, grammars phpparser.GrammarRange) *Document {
	result := parser.New().Parse(source)
	tree := parser.ParseTreeIn(source, grammars)
	return &Document{uri: uri, source: source, lines: result.Lines, result: result, tree: tree, file: parser.FileNodeOf(source, tree)}
}

func (d *Document) Tokens() []parser.Token {
	return d.result.Tokens
}

func (d *Document) File() *parser.FileNode {
	return d.file
}

func Analyze(uri, source string, pos protocol.Position, grammars phpparser.GrammarRange) *CursorContext {
	return NewDocument(uri, source, grammars).At(pos)
}

func (d *Document) At(pos protocol.Position) *CursorContext {
	name, at := d.nameAt(pos)
	if name == nil {
		return nil
	}
	return &CursorContext{NameContext: *name, Namespace: at.namespace, Uses: at.uses, Scope: at.scope}
}

func (d *Document) NameAt(pos protocol.Position) *NameContext {
	name, _ := d.nameAt(pos)
	return name
}

func (d *Document) nameAt(pos protocol.Position) (*NameContext, cursorDeclarations) {
	if pos.Line < 0 || pos.Line >= len(d.lines) {
		return nil, cursorDeclarations{}
	}
	line := d.lines[pos.Line]
	word, wordRange := wordAtLine(line, pos)
	joinedLine, joinedWordStart := resolve.JoinChainLines(d.lines, pos.Line, wordRange.Start.Character)
	accessKind, subjectExpr := detectAccess(joinedLine, joinedWordStart)
	called := isCalled(d.result.Tokens, wordRange)
	at := declarationsAt(d.source, d.tree, d.file, pos)
	return &NameContext{
		URI:             d.uri,
		Position:        pos,
		SymbolText:      word,
		SymbolKind:      classifySymbolKind(word),
		AccessKind:      accessKind,
		SubjectExpr:     subjectExpr,
		EnclosingFQN:    at.classFQN,
		WordRange:       wordRange,
		Line:            line,
		JoinedLine:      joinedLine,
		JoinedWordStart: joinedWordStart,
		File:            d.file,
		Called:          called,
		FunctionCall:    called && accessKind == AccessNone,
	}, at
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

type cursorDeclarations struct {
	src       string
	file      *parser.FileNode
	pos       protocol.Position
	lines     ast.Lines
	offset    uint32
	namespace string
	uses      []parser.UseNode
	classFQN  string
	scope     *Scope
}

func declarationsAt(src string, tree *ast.File, file *parser.FileNode, pos protocol.Position) cursorDeclarations {
	lines := ast.NewLines(src)
	at := cursorDeclarations{src: src, file: file, pos: pos, lines: lines, offset: offsetOf(src, lines, pos)}
	for _, stmt := range tree.Stmts {
		if stmt.Bounds().Start > at.offset {
			break
		}
		switch s := stmt.(type) {
		case *ast.Namespace:
			at.enterNamespace(s)
		case *ast.Use:
			at.uses = append(at.uses, parser.UseNodes(src, lines, s)...)
		}
		at.enclose(stmt)
	}
	return at
}

func (at *cursorDeclarations) enterNamespace(ns *ast.Namespace) {
	at.namespace, at.uses = "", nil
	if ns.Braced && at.offset > ns.End {
		return
	}
	if ns.Name != nil {
		at.namespace = ns.Name.Text(at.src)
	}
	for _, stmt := range ns.Stmts {
		if stmt.Bounds().Start > at.offset {
			return
		}
		if use, ok := stmt.(*ast.Use); ok {
			at.uses = append(at.uses, parser.UseNodes(at.src, at.lines, use)...)
		}
	}
}

func (at *cursorDeclarations) enclose(stmt ast.Stmt) {
	ast.Walk(stmt, func(n ast.Node) bool {
		span := n.Bounds()
		if at.offset < span.Start || at.offset > span.End {
			return false
		}
		switch n := n.(type) {
		case *ast.ClassDecl:
			if n.Name == nil {
				at.enterAnonymousClass()
			} else {
				at.enterClass(n.Name)
			}
		case *ast.InterfaceDecl:
			at.enterClass(n.Name)
		case *ast.TraitDecl:
			at.enterClass(n.Name)
		case *ast.EnumDecl:
			at.enterClass(n.Name)
		case *ast.Method:
			name := n.Name.Text(at.src)
			at.scope = &Scope{Kind: ScopeMethod, Name: name, ClassFQN: at.classFQN, MethodName: name, StartLine: at.line(n.Name.Start), EndLine: at.endLine(span)}
		case *ast.FunctionDecl:
			at.scope = &Scope{Kind: ScopeFunction, Name: n.Name.Text(at.src), StartLine: at.line(n.Name.Start), EndLine: at.endLine(span)}
		case *ast.Closure, *ast.ArrowFunction:
			at.scope = &Scope{Kind: ScopeClosure, ClassFQN: at.classFQN, StartLine: at.line(span.Start), EndLine: at.endLine(span)}
		}
		return true
	})
}

func (at *cursorDeclarations) enterClass(name *ast.Identifier) {
	if name == nil {
		return
	}
	at.classFQN = name.Text(at.src)
	if at.namespace != "" {
		at.classFQN = at.namespace + "\\" + at.classFQN
	}
	at.scope = nil
}

func (at *cursorDeclarations) enterAnonymousClass() {
	anon := at.file.AnonymousClassAt(at.pos.Line, at.pos.Character)
	if anon == nil || anon.FullName == at.classFQN {
		return
	}
	at.classFQN = anon.FullName
	at.scope = nil
}

func (at *cursorDeclarations) line(offset uint32) int {
	line, _ := at.lines.Position(offset)
	return line
}

func (at *cursorDeclarations) endLine(span ast.Span) int {
	return at.line(max(span.End, span.Start+1) - 1)
}

func offsetOf(src string, lines ast.Lines, pos protocol.Position) uint32 {
	start := lines[pos.Line]
	end := uint32(len(src))
	if pos.Line+1 < len(lines) {
		end = lines[pos.Line+1] - 1
	}
	return min(start+uint32(max(pos.Character, 0)), end)
}
