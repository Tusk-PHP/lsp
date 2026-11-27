package sourcectx

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type Call struct {
	Name      string
	Offset    uint32
	Position  protocol.Position
	Argument  int
	Function  bool
	Namespace Namespace
}

type callSearch int

const (
	callUnknown callSearch = iota
	callFound
	callNone
)

func CallAt(idx *symbols.Index, uri, src string, pos protocol.Position) (Call, bool) {
	from, at := idx.DeclarationBefore(uri, src, pos)
	end, ok := offsetAt(src, from-uint32(at.Character), at.Line, pos)
	if !ok {
		return Call{}, false
	}
	call, r, ok := callBefore(src[:end], from)
	if !ok {
		return Call{}, false
	}
	call.Position = protocol.Position{
		Line:      pos.Line - strings.Count(src[call.Offset:end], "\n"),
		Character: int(call.Offset) - strings.LastIndexByte(src[:call.Offset], '\n') - 1,
	}
	if call.Function {
		call.Namespace = r.namespaces(idx, uri, src, call.Offset).At(call.Offset)
	}
	return call, true
}

func offsetAt(src string, lineStart uint32, line int, pos protocol.Position) (uint32, bool) {
	if pos.Line < line {
		return 0, false
	}
	start := int(lineStart)
	for range pos.Line - line {
		nl := strings.IndexByte(src[start:], '\n')
		if nl < 0 {
			return 0, false
		}
		start += nl + 1
	}
	lineEnd := len(src)
	if nl := strings.IndexByte(src[start:], '\n'); nl >= 0 {
		lineEnd = start + nl
	}
	return uint32(min(start+max(pos.Character, 0), lineEnd)), true
}

func callBefore(src string, from uint32) (Call, *callReader, bool) {
	r := &callReader{src: src, from: from}
	call, search := r.call()
	if search == callUnknown {
		r = &callReader{src: src}
		call, search = r.call()
	}
	return call, r, search == callFound
}

type callReader struct {
	src    string
	from   uint32
	tokens []lexer.Token
}

func (r *callReader) call() (Call, callSearch) {
	r.lex()
	depth, argument := 0, 0
	for k := len(r.tokens) - 1; k >= 0; k-- {
		switch r.tokens[k].Kind {
		case token.RightParen, token.RightBracket, token.RightBrace:
			depth++
		case token.LeftBracket, token.Attribute:
			if depth > 0 {
				depth--
				continue
			}
			argument = 0
		case token.LeftBrace, token.CurlyOpen, token.DollarOpenCurlyBraces:
			if depth == 0 {
				return Call{}, callNone
			}
			depth--
		case token.LeftParen:
			if depth > 0 {
				depth--
				continue
			}
			return r.callee(r.prev(k), argument)
		case token.Comma:
			if depth == 0 {
				argument++
			}
		case token.Semicolon, token.OpenTag, token.OpenTagWithEcho, token.CloseTag:
			if depth == 0 {
				return Call{}, callNone
			}
		}
	}
	return r.exhausted()
}

func (r *callReader) lex() {
	if r.from == 0 {
		r.tokens = lex(r.src)
		return
	}
	r.tokens = lexer.LexCode(r.src[r.from:], token.Latest)
	for i := range r.tokens {
		r.tokens[i].Start += r.from
		r.tokens[i].End += r.from
	}
}

func (r *callReader) prev(k int) int {
	for k--; k >= 0 && r.tokens[k].Kind.IsTrivia(); k-- {
	}
	return k
}

func (r *callReader) exhausted() (Call, callSearch) {
	if r.from > 0 {
		return Call{}, callUnknown
	}
	return Call{}, callNone
}

func (r *callReader) callee(k, argument int) (Call, callSearch) {
	if k < 0 {
		return r.exhausted()
	}
	name := r.tokens[k]
	switch name.Kind {
	case token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative:
	default:
		return Call{}, callNone
	}
	return Call{Name: name.Text(r.src), Offset: name.Start, Argument: argument, Function: r.calledAsFunction(k)}, callFound
}

func (r *callReader) calledAsFunction(k int) bool {
	before := r.prev(k)
	if before < 0 {
		return r.from == 0
	}
	switch r.tokens[before].Kind {
	case token.ObjectOperator, token.NullsafeObjectOperator, token.DoubleColon, token.New, token.Function, token.Fn:
		return false
	case token.Ampersand, token.AmpersandFollowedByVarOrVararg, token.AmpersandNotFollowedByVarOrVararg:
		if keyword := r.prev(before); keyword >= 0 && r.tokens[keyword].Kind == token.Function {
			return false
		}
	}
	return !r.inAttribute(k)
}

func (r *callReader) inAttribute(k int) bool {
	depth := 0
	for i := k - 1; i >= 0; i-- {
		switch r.tokens[i].Kind {
		case token.RightBracket, token.RightParen:
			depth++
		case token.LeftParen, token.LeftBracket:
			if depth > 0 {
				depth--
			}
		case token.Attribute:
			if depth == 0 {
				return true
			}
			depth--
		case token.Semicolon, token.LeftBrace, token.RightBrace:
			return false
		}
	}
	return r.from > 0
}

func (r *callReader) namespaces(idx *symbols.Index, uri, src string, upTo uint32) *Namespaces {
	if r.from == 0 {
		return namespacesOfTokens(r.src, r.tokens)
	}
	if end, ok := idx.ImportsEnd(uri, src, upTo); ok {
		return LexNamespaces(src[:end])
	}
	return LexNamespaces(src[:upTo])
}
