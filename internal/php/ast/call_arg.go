package ast

import (
	"slices"
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
	Class    string
	Receiver Expr
	Static   bool
	New      bool
	Index    int
	InArray  bool
	Value    ArgValue
	Partial  string
	Text     string
	Quote    string
	src      string
	args     *ArgList
}

type StringDim struct {
	Fetch   *ArrayDimFetch
	Base    Expr
	Keys    []string
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
	value, partial, _, quote, ok := cur.classify(fetch.Dim)
	if !ok || value == ArgName {
		return StringDim{}, false
	}
	base, keys, ok := dimKeys(cur.src, fetch.Array)
	if !ok {
		return StringDim{}, false
	}
	return StringDim{Fetch: fetch, Base: base, Keys: keys, Value: value, Partial: partial, Quote: quote}, true
}

func dimKeys(src string, e Expr) (Expr, []string, bool) {
	var keys []string
	for fetch, ok := e.(*ArrayDimFetch); ok; fetch, ok = e.(*ArrayDimFetch) {
		key, isString := StringValue(src, fetch.Dim)
		if !isString {
			return nil, nil, false
		}
		keys = append(keys, key)
		e = fetch.Array
	}
	slices.Reverse(keys)
	return e, keys, true
}

func (a CallArg) StringArg(i int) (string, bool) {
	if i >= len(a.args.Args) {
		return "", false
	}
	return StringValue(a.src, a.args.Args[i].Value)
}

func LastSegment(name string) string {
	return name[strings.LastIndexByte(name, '\\')+1:]
}

func CallAt(tree *File, off uint32) (Expr, *ArgList) {
	return cursor{off: off}.call(tree)
}

func FunctionName(src string, call *Call) string {
	return className(src, call.Callee)
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
	case *New:
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
	out := CallArg{Call: call, Index: index, src: cur.src, args: args}
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
	out.Value, out.Partial, out.Text, out.Quote, ok = cur.classify(value)
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
		out.Class = className(cur.src, call.Class)
	case *New:
		out.New, out.Class = true, className(cur.src, call.Class)
	}
}

func className(src string, e Expr) string {
	name, ok := e.(*ConstFetch)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(name.Name.Bounds().Text(src), `\`)
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
			if i == 0 || cur.tokenIn(prevEnd, cur.off, token.Comma) {
				return i, nil, true
			}
			return 0, nil, false
		}
		if cur.off <= span.End {
			return i, s, true
		}
		prevEnd = span.End
	}
	if len(slots) == 0 || cur.tokenIn(prevEnd, cur.off, token.Comma) {
		return len(slots), nil, true
	}
	return 0, nil, false
}

func (cur cursor) tokenIn(from, to uint32, kind token.Kind) bool {
	for _, tok := range lexer.LexCode(cur.src[from:to], cur.grammar) {
		if tok.Kind == kind {
			return true
		}
	}
	return false
}

func (cur cursor) classify(e Expr) (value ArgValue, partial, text, quote string, ok bool) {
	if e == nil || IsMissing(e) || cur.off <= e.Bounds().Start {
		return ArgEmpty, "", "", "", true
	}
	switch e := e.(type) {
	case *Literal:
		closed := e.Kind == token.ConstantString
		if !closed && e.Kind != token.EncapsedAndWhitespace || cur.off == e.End && closed {
			return 0, "", "", "", false
		}
		return ArgString, cur.src[e.Start+1 : cur.off], cur.stringText(e.Span, closed), cur.src[e.Start : e.Start+1], true
	case *InterpolatedString:
		if e.Kind != token.DoubleQuote {
			return 0, "", "", "", false
		}
		for _, part := range e.Parts {
			if _, plain := part.(*StringPart); !plain && part.Bounds().Start < cur.off {
				return 0, "", "", "", false
			}
		}
		closed := e.End-e.Start > 1 && cur.src[e.End-1] == '"'
		return ArgString, cur.src[e.Start+1 : cur.off], cur.stringText(e.Span, closed), `"`, true
	case *ConstFetch:
		return ArgName, cur.src[e.Start:cur.off], e.Bounds().Text(cur.src), "", true
	}
	return 0, "", "", "", false
}

func (cur cursor) stringText(span Span, closed bool) string {
	if closed {
		return cur.src[span.Start+1 : span.End-1]
	}
	return cur.src[span.Start+1 : span.End]
}
