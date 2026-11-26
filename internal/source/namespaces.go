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
	src     string
	lines   ast.Lines
	version token.Version
	blocks  []namespaceBlock
}

type namespaceBlock struct {
	start     uint32
	name      string
	braced    *ast.Namespace
	uses      []parser.UseNode
	useStarts []uint32
	header    uint32
	lastUse   uint32
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
	n := &Namespaces{src: src, lines: lines, version: tree.Version}
	cur := namespaceBlock{header: openTagOf(tree)}
	leading := true
	for _, stmt := range tree.Stmts {
		switch s := stmt.(type) {
		case *ast.Namespace:
			n.blocks = append(n.blocks, cur)
			leading = false
			if !s.Braced {
				cur = namespaceBlock{start: s.Start, name: nameText(src, s.Name), header: s.End - 1}
				continue
			}
			block := namespaceBlock{start: s.Start, name: nameText(src, s.Name), braced: s}
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
		if html, ok := tree.Stmts[0].(*ast.InlineHTML); ok {
			return html.End
		}
	}
	return tree.Shebang.End
}

func nameText(src string, name *ast.Name) string {
	if name == nil {
		return ""
	}
	return name.Text(src)
}

func (b *namespaceBlock) addUse(src string, lines ast.Lines, use *ast.Use) {
	for _, node := range parser.UseNodes(src, lines, use) {
		b.uses = append(b.uses, node)
		b.useStarts = append(b.useStarts, use.Start)
	}
	b.lastUse = use.End
}

func (n *Namespaces) block(offset uint32) *namespaceBlock {
	i := sort.Search(len(n.blocks), func(i int) bool { return n.blocks[i].start > offset })
	return &n.blocks[max(i-1, 0)]
}

func (n *Namespaces) At(offset uint32) Namespace {
	b := n.block(offset)
	k := sort.Search(len(b.useStarts), func(i int) bool { return b.useStarts[i] > offset })
	return Namespace{Name: b.name, Uses: b.uses[:k:k]}
}

func (n *Namespaces) UseSiteAt(offset uint32) UseSite {
	b := n.block(offset)
	if b.lastUse > 0 {
		return UseSite{Line: n.line(b.lastUse-1) + 1}
	}
	anchor := b.header
	if b.braced != nil {
		anchor = n.openBrace(b.braced)
	}
	line := n.line(anchor) + 1
	return UseSite{Line: line, separate: line < len(n.lines) && !n.blank(line)}
}

func (n *Namespaces) openBrace(ns *ast.Namespace) uint32 {
	end := ns.End
	if len(ns.Stmts) > 0 {
		end = ns.Stmts[0].Bounds().Start
	}
	tokens := lexer.LexCode(n.src[ns.Start:end], n.version)
	brace := slices.IndexFunc(tokens, func(tok lexer.Token) bool { return tok.Kind == token.LeftBrace })
	return ns.Start + tokens[brace].Start
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
