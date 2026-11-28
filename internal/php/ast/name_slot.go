package ast

import (
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type NameKind uint8

const (
	NameNew NameKind = iota + 1
	NameUse
	NameAttribute
	NamePipe
)

type NameSlot struct {
	Kind    NameKind
	Partial string
}

func NameSlotAt(tree *File, src string, off uint32) (NameSlot, bool) {
	cur := cursor{src: src, off: off, grammar: tree.Version}
	var slot NameSlot
	found := false
	cur.visit(tree, func(n Node) {
		if s, ok, decided := cur.nameSlot(n); decided {
			slot, found = s, ok
		}
	})
	return slot, found
}

func (cur cursor) nameSlot(n Node) (NameSlot, bool, bool) {
	switch n := n.(type) {
	case *New:
		return cur.newSlot(n)
	case *Use:
		return cur.useSlot(n)
	case *TraitUse:
		return cur.traitUseSlot(n)
	case *AttributeGroup:
		return cur.attributeSlot(n)
	case *Binary:
		if n.Op == token.Pipe && IsMissing(n.Right) && cur.tokenIn(n.Left.Bounds().End, cur.off, token.Pipe) {
			return NameSlot{Kind: NamePipe}, true, true
		}
	}
	return NameSlot{}, false, false
}

func (cur cursor) newSlot(n *New) (NameSlot, bool, bool) {
	if n.Anonymous != nil {
		return NameSlot{}, false, false
	}
	if IsMissing(n.Class) {
		if cur.off < n.Start+uint32(len("new")) {
			return NameSlot{}, false, false
		}
		return NameSlot{Kind: NameNew}, true, true
	}
	if _, named := n.Class.(*ConstFetch); !named {
		return NameSlot{}, false, false
	}
	if partial, ok := cur.nameBefore(n.Class); ok {
		return NameSlot{Kind: NameNew, Partial: partial}, true, true
	}
	return NameSlot{}, false, false
}

func (cur cursor) useSlot(n *Use) (NameSlot, bool, bool) {
	if !cur.inStatement(n.Span, len("use")) {
		return NameSlot{}, false, false
	}
	slot := NameSlot{Kind: NameUse}
	if n.Prefix != nil {
		if partial, ok := cur.nameBefore(n.Prefix); ok {
			slot.Partial = partial
			return slot, true, true
		}
		slot.Partial = n.Prefix.Bounds().Text(cur.src) + `\`
	}
	for _, item := range n.Items {
		if item.Name == nil {
			continue
		}
		if partial, ok := cur.nameBefore(item.Name); ok {
			slot.Partial += partial
			break
		}
	}
	return slot, true, true
}

func (cur cursor) traitUseSlot(n *TraitUse) (NameSlot, bool, bool) {
	if !cur.inStatement(n.Span, len("use")) {
		return NameSlot{}, false, false
	}
	for _, name := range n.Traits {
		if name == nil {
			continue
		}
		if partial, ok := cur.nameBefore(name); ok {
			return NameSlot{Kind: NameUse, Partial: partial}, true, true
		}
	}
	if len(n.Adaptations) > 0 {
		return NameSlot{}, false, true
	}
	return NameSlot{Kind: NameUse}, true, true
}

func (cur cursor) attributeSlot(g *AttributeGroup) (NameSlot, bool, bool) {
	if cur.off < g.Start+uint32(len("#[")) || !cur.beforeCloser(g.Span, ']') {
		return NameSlot{}, false, false
	}
	slot := NameSlot{Kind: NameAttribute}
	for _, attr := range g.Attrs {
		if attr.Args != nil && attr.Args.Start < cur.off && cur.off <= attr.Args.End {
			return NameSlot{}, false, true
		}
		if attr.Name == nil {
			continue
		}
		if partial, ok := cur.nameBefore(attr.Name); ok {
			slot.Partial = partial
			break
		}
	}
	return slot, true, true
}

func (cur cursor) nameBefore(n Node) (string, bool) {
	span := n.Bounds()
	if cur.off < span.Start || cur.off > span.End {
		return "", false
	}
	return cur.src[span.Start:cur.off], true
}

func (cur cursor) inStatement(span Span, keyword int) bool {
	return cur.off > span.Start+uint32(keyword) && cur.beforeCloser(span, ';')
}

func (cur cursor) beforeCloser(span Span, closer byte) bool {
	if span.End > span.Start && cur.src[span.End-1] == closer {
		return cur.off < span.End
	}
	return cur.off <= span.End
}
