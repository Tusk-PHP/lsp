package laravel

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

var joinMethods = map[string]bool{
	"join": true, "joinwhere": true, "joinsub": true, "joinlateral": true,
	"leftjoin": true, "leftjoinwhere": true, "leftjoinsub": true, "leftjoinlateral": true,
	"rightjoin": true, "rightjoinwhere": true, "rightjoinsub": true,
	"crossjoin": true, "crossjoinsub": true,
	"straightjoin": true, "straightjoinwhere": true, "straightjoinsub": true,
}

func BuilderMayBeJoined(tokens []parser.Token, doc *scope.Document, line, col int) bool {
	op := tokenAt(tokens, protocol.Position{Line: line, Character: col})
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
	at := protocol.Position{Line: tokens[head].Line, Character: tokens[head].Column}
	binding := doc.BindingAt(at)
	if binding == nil {
		return false
	}
	since := lastValueWritten(binding, at)
	for _, occurrence := range doc.Occurrences(binding) {
		start := occurrence.Start
		if before(start, since) || !before(start, at) {
			continue
		}
		if i := tokenAt(tokens, start); i >= 0 && (chainJoins(tokens, i) || assignedJoined(tokens, i)) {
			return true
		}
	}
	return false
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

func lastValueWritten(binding *scope.Binding, at protocol.Position) protocol.Position {
	for i := len(binding.Rebinds) - 1; i >= 0; i-- {
		if binding.Rebinds[i].HoldsAt(at) && !binding.ReadsItself(i) {
			return binding.Rebinds[i].At
		}
	}
	return protocol.Position{}
}

func assignedJoined(tokens []parser.Token, variable int) bool {
	eq := parser.NextSignificant(tokens, variable+1)
	return eq >= 0 && tokens[eq].Kind == parser.TokenEquals && chainJoins(tokens, parser.NextSignificant(tokens, eq+1))
}

func tokenAt(tokens []parser.Token, pos protocol.Position) int {
	i := sort.Search(len(tokens), func(i int) bool {
		t := tokens[i]
		return t.Line > pos.Line || t.Line == pos.Line && t.Column >= pos.Character
	})
	if i == len(tokens) || tokens[i].Line != pos.Line || tokens[i].Column != pos.Character {
		return -1
	}
	return i
}

func before(a, b protocol.Position) bool {
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
