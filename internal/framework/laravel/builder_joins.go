package laravel

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

var joinMethods = map[string]bool{
	"join": true, "joinwhere": true, "joinsub": true, "joinlateral": true,
	"leftjoin": true, "leftjoinwhere": true, "leftjoinsub": true, "leftjoinlateral": true,
	"rightjoin": true, "rightjoinwhere": true, "rightjoinsub": true,
	"crossjoin": true, "crossjoinsub": true,
	"straightjoin": true, "straightjoinwhere": true, "straightjoinsub": true,
}

func BuilderMayBeJoined(tokens []parser.Token, doc *scope.Document, line, col int) bool {
	op := tokenAt(tokens, textpos.Position{Line: line, Character: col})
	if op < 0 || !isMemberOperator(tokens[op].Kind) {
		return false
	}
	receiverEnd := nullsafeStart(tokens, op)
	head := chainHead(tokens, receiverEnd)
	if chainJoins(tokens, head) || !callChainFrom(tokens, head, receiverEnd) {
		return true
	}
	if tokens[head].Kind != parser.TokenVariable || doc == nil {
		return false
	}
	at := tokenPosition(tokens[head])
	binding := doc.BindingAt(at)
	if binding == nil {
		return false
	}
	return valueJoined(tokens, doc, valueSpan{binding, lastValueWritten(binding, at), at})
}

type valueSpan struct {
	binding  *scope.Binding
	from, to textpos.Position
}

func valueJoined(tokens []parser.Token, doc *scope.Document, start valueSpan) bool {
	pending := []valueSpan{start}
	seen := map[valueSpan]bool{}
	for len(pending) > 0 {
		span := pending[len(pending)-1]
		pending = pending[:len(pending)-1]
		if seen[span] {
			continue
		}
		seen[span] = true
		for _, occurrence := range doc.Occurrences(span.binding) {
			pos := occurrence.Start
			if before(pos, span.from) || !before(pos, span.to) || span != start && overwritten(span.binding, span.from, pos) {
				continue
			}
			i := tokenAt(tokens, pos)
			if i >= 0 && (chainJoins(tokens, i) || assignedJoined(tokens, i)) {
				return true
			}
			if alias, ok := aliasSpan(tokens, doc, i, start.to); ok {
				pending = append(pending, alias)
			}
		}
	}
	return false
}

func aliasSpan(tokens []parser.Token, doc *scope.Document, variable int, until textpos.Position) (valueSpan, bool) {
	if lhs, rhs, ok := variableAssignment(tokens, parser.NextSignificant(tokens, variable+1)); ok && lhs == variable {
		read := tokenPosition(tokens[rhs])
		source := doc.BindingAt(read)
		return valueSpan{source, lastValueWritten(source, read), until}, source != nil
	}
	if lhs, rhs, ok := variableAssignment(tokens, parser.PrevSignificant(tokens, variable-1)); ok && rhs == variable {
		written := tokenPosition(tokens[lhs])
		target := doc.BindingAt(written)
		return valueSpan{target, written, until}, target != nil
	}
	return valueSpan{}, false
}

func variableAssignment(tokens []parser.Token, eq int) (lhs, rhs int, ok bool) {
	if eq < 0 || tokens[eq].Kind != parser.TokenEquals {
		return -1, -1, false
	}
	lhs, rhs = parser.PrevSignificant(tokens, eq-1), parser.NextSignificant(tokens, eq+1)
	if lhs < 0 || rhs < 0 || tokens[lhs].Kind != parser.TokenVariable || tokens[rhs].Kind != parser.TokenVariable {
		return -1, -1, false
	}
	end := parser.NextSignificant(tokens, rhs+1)
	return lhs, rhs, end >= 0 && tokens[end].Kind == parser.TokenSemicolon
}

func overwritten(binding *scope.Binding, from, pos textpos.Position) bool {
	for i := range binding.Rebinds {
		rebind := &binding.Rebinds[i]
		if before(from, rebind.At) && before(rebind.At, pos) && rebind.HoldsAt(pos) && !binding.ReadsItself(i) {
			return true
		}
	}
	return false
}

func tokenPosition(t parser.Token) textpos.Position {
	return textpos.Position{Line: t.Line, Character: t.Column}
}

func callChainFrom(tokens []parser.Token, head, end int) bool {
	i, ok := chainStart(tokens, head, end)
	for ok && i >= 0 && i < end {
		i, ok = callLink(tokens, i)
	}
	return ok && i == end
}

func chainStart(tokens []parser.Token, head, end int) (int, bool) {
	switch tokens[head].Kind {
	case parser.TokenVariable:
		return parser.NextSignificant(tokens, head+1), true
	case parser.TokenDoubleColon:
		prev := parser.PrevSignificant(tokens, head-1)
		return head, prev >= 0 && tokens[prev].Kind == parser.TokenStatic
	case parser.TokenIdentifier, parser.TokenBackslash:
		i := head
		for i < end && (tokens[i].Kind == parser.TokenIdentifier || tokens[i].Kind == parser.TokenBackslash) {
			i = parser.NextSignificant(tokens, i+1)
		}
		return i, i == end || tokens[i].Kind == parser.TokenDoubleColon
	}
	return head, false
}

func callLink(tokens []parser.Token, i int) (int, bool) {
	op := i
	if tokens[op].Kind == parser.TokenQuestion {
		op = parser.NextSignificant(tokens, op+1)
	}
	if op < 0 || !isMemberOperator(tokens[op].Kind) {
		return i, false
	}
	name := parser.NextSignificant(tokens, op+1)
	if name < 0 || tokens[name].Kind != parser.TokenIdentifier || !opensCall(tokens, name) {
		return i, false
	}
	closing := parser.MatchingClose(tokens, parser.NextSignificant(tokens, name+1))
	return parser.NextSignificant(tokens, closing+1), closing >= 0
}

func lastValueWritten(binding *scope.Binding, at textpos.Position) textpos.Position {
	for i := len(binding.Rebinds) - 1; i >= 0; i-- {
		if binding.Rebinds[i].HoldsAt(at) && !binding.ReadsItself(i) {
			return binding.Rebinds[i].At
		}
	}
	return textpos.Position{}
}

func assignedJoined(tokens []parser.Token, variable int) bool {
	eq := parser.NextSignificant(tokens, variable+1)
	return eq >= 0 && tokens[eq].Kind == parser.TokenEquals && chainJoins(tokens, parser.NextSignificant(tokens, eq+1))
}

func tokenAt(tokens []parser.Token, pos textpos.Position) int {
	i := sort.Search(len(tokens), func(i int) bool {
		t := tokens[i]
		return t.Line > pos.Line || t.Line == pos.Line && t.Column >= pos.Character
	})
	if i == len(tokens) || tokens[i].Line != pos.Line || tokens[i].Column != pos.Character {
		return -1
	}
	return i
}

func before(a, b textpos.Position) bool {
	return a.Line < b.Line || a.Line == b.Line && a.Character < b.Character
}

func isMemberOperator(kind parser.TokenKind) bool {
	return kind == parser.TokenArrow || kind == parser.TokenDoubleColon
}

func chainHead(tokens []parser.Token, op int) int {
	head := op
	for {
		start := operandStart(tokens, parser.PrevSignificant(tokens, head-1))
		if start < 0 {
			return head
		}
		prev := parser.PrevSignificant(tokens, start-1)
		switch {
		case prev >= 0 && isMemberOperator(tokens[prev].Kind):
			head = nullsafeStart(tokens, prev)
		default:
			return start
		}
	}
}

func operandStart(tokens []parser.Token, end int) int {
	if end < 0 {
		return -1
	}
	switch tokens[end].Kind {
	case parser.TokenCloseParen, parser.TokenCloseBracket:
		open := parser.MatchingOpen(tokens, end)
		if open < 0 {
			return -1
		}
		if callee := parser.PrevSignificant(tokens, open-1); callee >= 0 && isName(tokens[callee].Kind) {
			return qualifiedStart(tokens, callee)
		}
		return open
	case parser.TokenIdentifier, parser.TokenVariable:
		return qualifiedStart(tokens, end)
	}
	return -1
}

func qualifiedStart(tokens []parser.Token, i int) int {
	for {
		sep := parser.PrevSignificant(tokens, i-1)
		if sep < 0 || tokens[sep].Kind != parser.TokenBackslash {
			return i
		}
		name := parser.PrevSignificant(tokens, sep-1)
		if name < 0 || tokens[name].Kind != parser.TokenIdentifier {
			return sep
		}
		i = name
	}
}

func nullsafeStart(tokens []parser.Token, op int) int {
	if q := parser.PrevSignificant(tokens, op-1); q >= 0 && tokens[op].Kind == parser.TokenArrow && tokens[q].Kind == parser.TokenQuestion {
		return q
	}
	return op
}

func isName(kind parser.TokenKind) bool {
	return kind == parser.TokenIdentifier || kind == parser.TokenVariable
}

func chainJoins(tokens []parser.Token, head int) bool {
	wantOperand, afterOperator := true, false
	for i := head; i >= 0 && i < len(tokens); i = parser.NextSignificant(tokens, i+1) {
		t := tokens[i]
		switch t.Kind {
		case parser.TokenIdentifier, parser.TokenVariable:
			if !wantOperand {
				return false
			}
			if afterOperator && t.Kind == parser.TokenIdentifier && joinMethods[strings.ToLower(t.Value)] && opensCall(tokens, i) {
				return true
			}
			wantOperand, afterOperator = false, false
		case parser.TokenBackslash:
			wantOperand = true
		case parser.TokenOpenParen, parser.TokenOpenBracket:
			closing := parser.MatchingClose(tokens, i)
			if closing < 0 {
				return false
			}
			if groupJoins(tokens, i, closing) {
				return true
			}
			i, wantOperand, afterOperator = closing, false, false
		case parser.TokenArrow, parser.TokenDoubleColon:
			if wantOperand {
				return false
			}
			wantOperand, afterOperator = true, true
		case parser.TokenQuestion:
			if wantOperand || !opens(tokens, i, parser.TokenArrow) {
				return false
			}
		default:
			return false
		}
	}
	return false
}

func groupJoins(tokens []parser.Token, open, closing int) bool {
	for i := open + 1; i < closing; i++ {
		if tokens[i].Kind != parser.TokenIdentifier || !joinMethods[strings.ToLower(tokens[i].Value)] || !opensCall(tokens, i) {
			continue
		}
		if op := parser.PrevSignificant(tokens, i-1); isMemberOperator(tokens[op].Kind) {
			return true
		}
	}
	return false
}

func opensCall(tokens []parser.Token, name int) bool {
	return opens(tokens, name, parser.TokenOpenParen)
}

func opens(tokens []parser.Token, i int, kind parser.TokenKind) bool {
	next := parser.NextSignificant(tokens, i+1)
	return next >= 0 && tokens[next].Kind == kind
}
