package ast

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type ArgValue uint8

const (
	ArgEmpty ArgValue = iota
	ArgString
	ArgName
)

type CallArg struct {
	Call     Expr
	Function string
	Method   string
	Receiver Expr
	Static   bool
	Index    int
	InArray  bool
	Value    ArgValue
	Partial  string
	Quote    string
}

type StringDim struct {
	Fetch   *ArrayDimFetch
	Value   ArgValue
	Partial string
	Quote   string
}

type cursor struct {
	src     string
	off     uint32
	grammar token.Version
}

func CallArgAt(tree *File, src string, off uint32) (CallArg, bool) {
	return cursor{src: src, off: off, grammar: tree.Version}.callArg(tree)
}

func StringDimAt(tree *File, src string, off uint32) (StringDim, bool) {
	cur := cursor{src: src, off: off, grammar: tree.Version}
	var fetch *ArrayDimFetch
	cur.visit(tree, func(n Node) {
		if f, ok := n.(*ArrayDimFetch); ok && cur.inBrackets(f.Array.Bounds().End, f.End, f.Unclosed) {
			fetch = f
		}
	})
	if fetch == nil {
		return StringDim{}, false
	}
	value, partial, quote, ok := cur.classify(fetch.Dim)
	if !ok || value == ArgName {
		return StringDim{}, false
	}
	return StringDim{Fetch: fetch, Value: value, Partial: partial, Quote: quote}, true
}

func CallAt(tree *File, off uint32) (Expr, *ArgList) {
	return cursor{off: off}.call(tree)
}

func FunctionName(src string, call *Call) string {
	name, ok := call.Callee.(*ConstFetch)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(name.Name.Bounds().Text(src), `\`)
}

func StringValue(src string, e Expr) (string, bool) {
	lit, ok := e.(*Literal)
	if !ok || lit.Kind != token.ConstantString {
		return "", false
	}
	return src[lit.Start+1 : lit.End-1], true
}

func (cur cursor) visit(n Node, f func(Node)) {
	span := n.Bounds()
	if cur.off < span.Start || cur.off > span.End {
		return
	}
	f(n)
	Children(n, func(c Node) { cur.visit(c, f) })
}

func (cur cursor) inBrackets(open, end uint32, unclosed bool) bool {
	return open < cur.off && (cur.off < end || unclosed && cur.off == end)
}

func argList(n Node) *ArgList {
	switch n := n.(type) {
	case *Call:
		return n.Args
	case *MethodCall:
		return n.Args
	case *StaticCall:
		return n.Args
	}
	return nil
}

func (cur cursor) call(tree *File) (Expr, *ArgList) {
	var call Expr
	var args *ArgList
	cur.visit(tree, func(n Node) {
		if list := argList(n); list != nil && cur.inBrackets(list.Start, list.End, list.Unclosed) {
			call, args = n.(Expr), list
		}
	})
	return call, args
}

func (cur cursor) callArg(tree *File) (CallArg, bool) {
	call, args := cur.call(tree)
	if call == nil {
		return CallArg{}, false
	}
	slots := make([]Node, len(args.Args))
	for i, a := range args.Args {
		slots[i] = a
	}
	index, slot, ok := cur.slot(slots)
	if !ok {
		return CallArg{}, false
	}
	out := CallArg{Call: call, Index: index}
	cur.callee(call, &out)
	var value Expr
	if slot != nil {
		value = slot.(*Arg).Value
	}
	if array, ok := value.(*Array); ok && cur.inBrackets(array.Start, array.End, array.Unclosed) {
		out.InArray = true
		if value, ok = cur.arrayElement(array); !ok {
			return CallArg{}, false
		}
	}
	out.Value, out.Partial, out.Quote, ok = cur.classify(value)
	return out, ok
}

func (cur cursor) callee(call Expr, out *CallArg) {
	switch call := call.(type) {
	case *Call:
		out.Function = FunctionName(cur.src, call)
	case *MethodCall:
		out.Receiver, out.Method = call.Object, MethodName(cur.src, call)
	case *StaticCall:
		out.Receiver, out.Static, out.Method = call.Class, true, MethodName(cur.src, call)
	}
}

func MethodName(src string, call Expr) string {
	_, name, _ := MemberAccess(call)
	if id, ok := name.(*IdentifierExpr); ok {
		return id.Bounds().Text(src)
	}
	return ""
}

func (cur cursor) arrayElement(array *Array) (Expr, bool) {
	slots := make([]Node, 0, len(array.Items))
	for _, item := range array.Items {
		if item == nil {
			continue
		}
		if item.Key != nil {
			return nil, false
		}
		slots = append(slots, item)
	}
	_, slot, ok := cur.slot(slots)
	if !ok || slot == nil {
		return nil, ok
	}
	return slot.(*ArrayItem).Value, true
}

func (cur cursor) slot(slots []Node) (int, Node, bool) {
	prevEnd := uint32(0)
	for i, s := range slots {
		span := s.Bounds()
		if cur.off < span.Start {
			if i == 0 || cur.commaIn(prevEnd, cur.off) {
				return i, nil, true
			}
			return 0, nil, false
		}
		if cur.off <= span.End {
			return i, s, true
		}
		prevEnd = span.End
	}
	if len(slots) == 0 || cur.commaIn(prevEnd, cur.off) {
		return len(slots), nil, true
	}
	return 0, nil, false
}

func (cur cursor) commaIn(from, to uint32) bool {
	for _, tok := range lexer.LexCode(cur.src[from:to], cur.grammar) {
		if tok.Kind == token.Comma {
			return true
		}
	}
	return false
}

func (cur cursor) classify(e Expr) (ArgValue, string, string, bool) {
	if e == nil || IsMissing(e) || cur.off <= e.Bounds().Start {
		return ArgEmpty, "", "", true
	}
	switch e := e.(type) {
	case *Literal:
		closed := e.Kind == token.ConstantString
		if !closed && e.Kind != token.EncapsedAndWhitespace || cur.off == e.End && closed {
			return 0, "", "", false
		}
		return ArgString, cur.src[e.Start+1 : cur.off], cur.src[e.Start : e.Start+1], true
	case *InterpolatedString:
		if e.Kind != token.DoubleQuote {
			return 0, "", "", false
		}
		for _, part := range e.Parts {
			if _, plain := part.(*StringPart); !plain && part.Bounds().Start < cur.off {
				return 0, "", "", false
			}
		}
		return ArgString, cur.src[e.Start+1 : cur.off], `"`, true
	case *ConstFetch:
		return ArgName, cur.src[e.Start:cur.off], "", true
	}
	return 0, "", "", false
}
