package sourcectx

import (
	"cmp"
	"slices"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
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
	Scope  *Scope
	NewUse UseSite
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
	uri         string
	source      string
	lines       []string
	offsets     ast.Lines
	tree        *ast.File
	significant []lexer.Token
	file        *parser.FileNode
	namespaces  *Namespaces
}

func NewDocument(uri, source string, grammars phpparser.GrammarRange) *Document {
	parsed := parser.ParseIn(source, grammars)
	return documentOf(uri, source, parsed, parser.FileNodeOf(source, parsed.Tree))
}

// IndexedDocument is the document for source, built from the parse the index
// keeps of it when source is the text idx last indexed for uri, and parsed as
// NewDocument does otherwise. Every request on an open file reads it here.
func IndexedDocument(idx *symbols.Index, uri, source string) *Document {
	parsed, file := indexedParse(idx, uri, source)
	return documentOf(uri, source, parsed, file)
}

// IndexedFile is the declarations of IndexedDocument(idx, uri, source).
func IndexedFile(idx *symbols.Index, uri, source string) *parser.FileNode {
	_, file := indexedParse(idx, uri, source)
	return file
}

// IndexedTree is the syntax tree and declarations of IndexedDocument(idx,
// uri, source).
func IndexedTree(idx *symbols.Index, uri, source string) (*ast.File, *parser.FileNode) {
	parsed, file := indexedParse(idx, uri, source)
	return parsed.Tree, file
}

func indexedParse(idx *symbols.Index, uri, source string) (phpparser.Parsed, *parser.FileNode) {
	if parsed, file, ok := idx.IndexedParse(uri, source); ok {
		return parsed, file
	}
	parsed := parser.ParseIn(source, idx.GrammarOf(uri))
	file := parser.FileNodeOf(source, parsed.Tree)
	idx.KeepParse(uri, source, parsed, file)
	return parsed, file
}

func documentOf(uri, source string, parsed phpparser.Parsed, file *parser.FileNode) *Document {
	offsets := ast.NewLines(source)
	return &Document{
		uri:         uri,
		source:      source,
		lines:       strings.Split(source, "\n"),
		offsets:     offsets,
		tree:        parsed.Tree,
		significant: parsed.Significant,
		file:        file,
		namespaces:  NamespacesOf(source, offsets, parsed.Tree),
	}
}

func (d *Document) Significant() []lexer.Token {
	return d.significant
}

func (d *Document) Range(start, end uint32) protocol.Range {
	startLine, startCol := d.offsets.Position(start)
	endLine, endCol := d.offsets.Position(end)
	return protocol.Range{Start: protocol.Position{Line: startLine, Character: startCol}, End: protocol.Position{Line: endLine, Character: endCol}}
}

func (d *Document) File() *parser.FileNode {
	return d.file
}

func Analyze(idx *symbols.Index, uri, source string, pos protocol.Position) *CursorContext {
	return IndexedDocument(idx, uri, source).At(pos)
}

func (d *Document) At(pos protocol.Position) *CursorContext {
	names, ats := d.namesAt([]protocol.Position{pos})
	if names[0] == nil {
		return nil
	}
	return &CursorContext{NameContext: *names[0], Scope: ats[0].scope, NewUse: ats[0].newUse}
}

func (d *Document) NameAt(pos protocol.Position) *NameContext {
	return d.NamesAt([]protocol.Position{pos})[0]
}

// NamesAt is NameAt for each position, reading the syntax tree once for all
// of them; a position outside the document gives nil.
func (d *Document) NamesAt(positions []protocol.Position) []*NameContext {
	names, _ := d.namesAt(positions)
	return names
}

func (d *Document) namesAt(positions []protocol.Position) ([]*NameContext, []cursorDeclarations) {
	names := make([]*NameContext, len(positions))
	ats := make([]cursorDeclarations, len(positions))
	inside := make([]int, 0, len(positions))
	for i, pos := range positions {
		if pos.Line < 0 || pos.Line >= len(d.lines) {
			continue
		}
		line := d.lines[pos.Line]
		word, wordRange := wordAtLine(line, pos)
		joinedLine, joinedWordStart := resolve.JoinChainLines(d.lines, pos.Line, wordRange.Start.Character)
		accessKind, subjectExpr := detectAccess(joinedLine, joinedWordStart)
		ats[i] = d.declarationsBefore(pos, wordRange)
		names[i] = &NameContext{
			URI:             d.uri,
			Position:        pos,
			SymbolText:      word,
			SymbolKind:      classifySymbolKind(word),
			AccessKind:      accessKind,
			SubjectExpr:     subjectExpr,
			WordRange:       wordRange,
			Line:            line,
			JoinedLine:      joinedLine,
			JoinedWordStart: joinedWordStart,
		}
		inside = append(inside, i)
	}
	slices.SortStableFunc(inside, func(a, b int) int { return cmp.Compare(ats[a].offset, ats[b].offset) })
	for _, stmt := range d.tree.Stmts {
		encloseAll(ats, inside, stmt)
	}
	for _, i := range inside {
		at, name := &ats[i], names[i]
		name.EnclosingFQN = at.classFQN
		name.File = at.namespace.View(d.file)
		name.Called = at.called
		name.FunctionCall = at.called && name.AccessKind == AccessNone
	}
	return names, ats
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
	wordEnd   uint32
	namespace Namespace
	newUse    UseSite
	classFQN  string
	scope     *Scope
	called    bool
}

func (d *Document) declarationsBefore(pos protocol.Position, word protocol.Range) cursorDeclarations {
	offset := offsetOf(d.source, d.offsets, pos)
	return cursorDeclarations{
		src:       d.source,
		file:      d.file,
		pos:       pos,
		lines:     d.offsets,
		offset:    offset,
		wordEnd:   offsetOf(d.source, d.offsets, word.End),
		namespace: d.namespaces.At(offset),
		newUse:    d.namespaces.UseSiteAt(offset),
	}
}

func encloseAll(ats []cursorDeclarations, byOffset []int, n ast.Node) {
	span := n.Bounds()
	lo := sort.Search(len(byOffset), func(k int) bool { return ats[byOffset[k]].offset >= span.Start })
	hi := sort.Search(len(byOffset), func(k int) bool { return ats[byOffset[k]].offset > span.End })
	if lo >= hi {
		return
	}
	inside := byOffset[lo:hi]
	for _, i := range inside {
		ats[i].enter(n, span)
	}
	ast.Children(n, func(c ast.Node) { encloseAll(ats, inside, c) })
}

func (at *cursorDeclarations) enter(n ast.Node, span ast.Span) {
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
	case *ast.Call:
		at.calls(n.Callee)
	case *ast.MethodCall:
		at.calls(n.Name)
	case *ast.StaticCall:
		at.calls(n.Name)
	}
}

func (at *cursorDeclarations) calls(callee ast.Expr) {
	switch callee := callee.(type) {
	case *ast.ConstFetch:
		at.called = at.called || callee.Name.End == at.wordEnd
	case *ast.IdentifierExpr:
		at.called = at.called || callee.End == at.wordEnd
	}
}

func (at *cursorDeclarations) enterClass(name *ast.Identifier) {
	if name == nil {
		return
	}
	at.classFQN = name.Text(at.src)
	if at.namespace.Name != "" {
		at.classFQN = at.namespace.Name + "\\" + at.classFQN
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
