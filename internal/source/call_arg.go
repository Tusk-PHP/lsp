package sourcectx

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
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
	Call     ast.Expr
	Function string
	Method   string
	Receiver ast.Expr
	Static   bool
	Index    int
	InArray  bool
	Value    ArgValue
	Partial  string
	Quote    string
}

type StringDim struct {
	Fetch   *ast.ArrayDimFetch
	Value   ArgValue
	Partial string
	Quote   string
}

type cursor struct {
	src     string
	off     uint32
	grammar token.Version
}

func (c *CursorContext) cursor() (cursor, *ast.File) {
	return cursor{src: c.File.Source, off: c.Offset, grammar: c.File.Grammar}, c.File.Tree
}

func (c *CursorContext) CallArg() (CallArg, bool) {
	cur, tree := c.cursor()
	return cur.callArg(tree)
}

func (c *CursorContext) StringDim() (StringDim, bool) {
	cur, tree := c.cursor()
	var fetch *ast.ArrayDimFetch
	cur.visit(tree, func(n ast.Node) {
		if f, ok := n.(*ast.ArrayDimFetch); ok && cur.inBrackets(f.Array.Bounds().End, f.End, f.Unclosed) {
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

func CallNodeAt(tree *ast.File, src string, off uint32) (ast.Expr, *ast.ArgList) {
	cur := cursor{src: src, off: off}
	return cur.call(tree)
}

func FunctionName(src string, call *ast.Call) string {
	name, ok := call.Callee.(*ast.ConstFetch)
	if !ok {
		return ""
	}
	return strings.TrimPrefix(name.Name.Bounds().Text(src), `\`)
}

func StringValue(src string, e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != token.ConstantString {
		return "", false
	}
	return src[lit.Start+1 : lit.End-1], true
}

func (cur cursor) visit(n ast.Node, f func(ast.Node)) {
	span := n.Bounds()
	if cur.off < span.Start || cur.off > span.End {
		return
	}
	f(n)
	ast.Children(n, func(c ast.Node) { cur.visit(c, f) })
}

func (cur cursor) inBrackets(open, end uint32, unclosed bool) bool {
	return open < cur.off && (cur.off < end || unclosed && cur.off == end)
}

func argList(n ast.Node) *ast.ArgList {
	switch n := n.(type) {
	case *ast.Call:
		return n.Args
	case *ast.MethodCall:
		return n.Args
	case *ast.StaticCall:
		return n.Args
	}
	return nil
}

func (cur cursor) call(tree *ast.File) (ast.Expr, *ast.ArgList) {
	var call ast.Expr
	var args *ast.ArgList
	cur.visit(tree, func(n ast.Node) {
		if list := argList(n); list != nil && cur.inBrackets(list.Start, list.End, list.Unclosed) {
			call, args = n.(ast.Expr), list
		}
	})
	return call, args
}

func (cur cursor) callArg(tree *ast.File) (CallArg, bool) {
	call, args := cur.call(tree)
	if call == nil {
		return CallArg{}, false
	}
	slots := make([]ast.Node, len(args.Args))
	for i, a := range args.Args {
		slots[i] = a
	}
	index, slot, ok := cur.slot(slots)
	if !ok {
		return CallArg{}, false
	}
	out := CallArg{Call: call, Index: index}
	cur.callee(call, &out)
	var value ast.Expr
	if slot != nil {
		value = slot.(*ast.Arg).Value
	}
	if array, ok := value.(*ast.Array); ok && cur.inBrackets(array.Start, array.End, array.Unclosed) {
		out.InArray = true
		if value, ok = cur.arrayElement(array); !ok {
			return CallArg{}, false
		}
	}
	out.Value, out.Partial, out.Quote, ok = cur.classify(value)
	return out, ok
}

func (cur cursor) callee(call ast.Expr, out *CallArg) {
	switch call := call.(type) {
	case *ast.Call:
		out.Function = FunctionName(cur.src, call)
	case *ast.MethodCall:
		out.Receiver, out.Method = call.Object, MethodName(cur.src, call)
	case *ast.StaticCall:
		out.Receiver, out.Static, out.Method = call.Class, true, MethodName(cur.src, call)
	}
}

func MethodName(src string, call ast.Expr) string {
	_, name, _ := ast.MemberAccess(call)
	if id, ok := name.(*ast.IdentifierExpr); ok {
		return id.Bounds().Text(src)
	}
	return ""
}

func (cur cursor) arrayElement(array *ast.Array) (ast.Expr, bool) {
	slots := make([]ast.Node, 0, len(array.Items))
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
	return slot.(*ast.ArrayItem).Value, true
}

func (cur cursor) slot(slots []ast.Node) (int, ast.Node, bool) {
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

func (cur cursor) classify(e ast.Expr) (ArgValue, string, string, bool) {
	if e == nil || ast.IsMissing(e) || cur.off <= e.Bounds().Start {
		return ArgEmpty, "", "", true
	}
	switch e := e.(type) {
	case *ast.Literal:
		closed := e.Kind == token.ConstantString
		if !closed && e.Kind != token.EncapsedAndWhitespace || cur.off == e.End && closed {
			return 0, "", "", false
		}
		return ArgString, cur.src[e.Start+1 : cur.off], cur.src[e.Start : e.Start+1], true
	case *ast.InterpolatedString:
		if e.Kind != token.DoubleQuote {
			return 0, "", "", false
		}
		for _, part := range e.Parts {
			if _, plain := part.(*ast.StringPart); !plain && part.Bounds().Start < cur.off {
				return 0, "", "", false
			}
		}
		return ArgString, cur.src[e.Start+1 : cur.off], `"`, true
	case *ast.ConstFetch:
		return ArgName, cur.src[e.Start:cur.off], "", true
	}
	return 0, "", "", false
}
