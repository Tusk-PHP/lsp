package sourcectx

import (
	"slices"
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

type Namespaces struct {
	src    string
	lines  ast.Lines
	blocks []namespaceBlock
}

type namespaceBlock struct {
	start   uint32
	name    string
	nameAt  ast.Span
	braced  bool
	brace   uint32
	uses    []parser.UseNode
	imports []importSite
	header  uint32
	lastUse uint32
}

type importSite struct {
	stmt    ast.Span
	nameEnd uint32
}

type Namespace struct {
	Name string
	Uses []parser.UseNode
}

type UseSite struct {
	Line     int
	separate bool
}

func NamespacesOf(src string, lines ast.Lines, tree *ast.File) *Namespaces {
	n := &Namespaces{src: src, lines: lines}
	cur := namespaceBlock{header: openTagOf(tree)}
	leading := true
	for _, stmt := range tree.Stmts {
		switch s := stmt.(type) {
		case *ast.Namespace:
			n.blocks = append(n.blocks, cur)
			leading = false
			block := namespaceBlock{start: s.Start}
			if s.Name != nil {
				block.name, block.nameAt = s.Name.Text(src), s.Name.Span
			}
			if !s.Braced {
				block.header = s.End - 1
				cur = block
				continue
			}
			block.braced, block.brace = true, openBrace(src, tree.Version, s)
			for _, inner := range s.Stmts {
				if use, ok := inner.(*ast.Use); ok {
					block.addUse(src, lines, use)
				}
			}
			n.blocks = append(n.blocks, block)
			cur = namespaceBlock{start: s.End + 1, header: s.End - 1}
		case *ast.Use:
			cur.addUse(src, lines, s)
			leading = false
		case *ast.Declare:
			if leading && s.Body == nil {
				cur.header = s.End - 1
			}
		case *ast.InlineHTML:
		default:
			leading = false
		}
	}
	n.blocks = append(n.blocks, cur)
	return n
}

func openTagOf(tree *ast.File) uint32 {
	if len(tree.Stmts) > 0 {
		if html, ok := tree.Stmts[0].(*ast.InlineHTML); ok && html.Start <= tree.Shebang.End {
			return html.End
		}
	}
	return tree.Shebang.End
}

func openBrace(src string, version token.Version, ns *ast.Namespace) uint32 {
	end := ns.End
	if len(ns.Stmts) > 0 {
		end = ns.Stmts[0].Bounds().Start
	}
	tokens := lexer.LexCode(src[ns.Start:end], version)
	brace := slices.IndexFunc(tokens, func(tok lexer.Token) bool { return tok.Kind == token.LeftBrace })
	return ns.Start + tokens[brace].Start
}

func (b *namespaceBlock) addUse(src string, lines ast.Lines, use *ast.Use) {
	b.uses = append(b.uses, parser.UseNodes(src, lines, use)...)
	for _, item := range use.Items {
		if item.Name != nil {
			b.imports = append(b.imports, importSite{stmt: use.Span, nameEnd: item.Name.End})
		}
	}
	b.lastUse = use.End
}

func (n *Namespaces) Offset(pos protocol.Position) uint32 {
	return offsetOf(n.src, n.lines, pos)
}

func (n *Namespaces) block(offset uint32) *namespaceBlock {
	i := sort.Search(len(n.blocks), func(i int) bool { return n.blocks[i].start > offset })
	return &n.blocks[max(i-1, 0)]
}

func (n *Namespaces) At(offset uint32) Namespace {
	return n.block(offset).at(offset)
}

func (b *namespaceBlock) at(offset uint32) Namespace {
	k := b.importsBefore(offset)
	return Namespace{Name: b.name, Uses: b.uses[:k:k]}
}

func (b *namespaceBlock) importsBefore(offset uint32) int {
	return sort.Search(len(b.imports), func(i int) bool { return b.imports[i].stmt.Start > offset })
}

func (n *Namespaces) UseSiteAt(offset uint32) UseSite {
	b := n.block(offset)
	if b.lastUse > 0 {
		return UseSite{Line: n.line(b.lastUse-1) + 1}
	}
	anchor := b.header
	if b.braced {
		anchor = b.brace
	}
	line := n.line(anchor) + 1
	return UseSite{Line: line, separate: line < len(n.lines) && !n.blank(line)}
}

func (n *Namespaces) line(offset uint32) int {
	line, _ := n.lines.Position(offset)
	return line
}

func (n *Namespaces) blank(line int) bool {
	end := uint32(len(n.src))
	if line+1 < len(n.lines) {
		end = n.lines[line+1]
	}
	return strings.TrimSpace(n.src[n.lines[line]:end]) == ""
}

func (u UseSite) Insert(line string) protocol.TextEdit {
	text := line
	if u.separate {
		text += "\n"
	}
	at := protocol.Position{Line: u.Line}
	return protocol.TextEdit{Range: protocol.Range{Start: at, End: at}, NewText: text}
}

func (ns Namespace) View(file *parser.FileNode) *parser.FileNode {
	view := *file
	view.Namespace, view.Uses = ns.Name, ns.Uses
	return &view
}

func (ns Namespace) resolveClass(written string, kind token.Kind) string {
	switch kind {
	case token.NameFullyQualified:
		return written[1:]
	case token.NameRelative:
		_, rest, _ := strings.Cut(written, "\\")
		return parser.ResolveImportedName(rest, ns.Name, nil, "class")
	}
	return parser.ResolveImportedName(written, ns.Name, ns.Uses, "class")
}
