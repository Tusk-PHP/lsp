package laravel

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

// tokenSpan is a range of tokens [start, end).
type tokenSpan struct{ start, end int }

// splitTokenArgs splits the arguments of the call whose `(` is toks[open],
// returning them and the index of its `)`, or -1 when it isn't closed.
func splitTokenArgs(toks []parser.Token, open int) ([]tokenSpan, int) {
	if open >= len(toks) || toks[open].Kind != parser.TokenOpenParen {
		return nil, -1
	}
	var args []tokenSpan
	depth, start := 0, open+1
	for k := open; k < len(toks); k++ {
		switch toks[k].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
			if depth == 0 {
				if k > start {
					args = append(args, tokenSpan{start, k})
				}
				return args, k
			}
		case parser.TokenComma:
			if depth == 1 {
				args = append(args, tokenSpan{start, k})
				start = k + 1
			}
		case parser.TokenSemicolon:
			// A statement ends inside a closure's body, never in the
			// arguments themselves.
			if depth <= 1 {
				return nil, -1
			}
		case parser.TokenEOF:
			return nil, -1
		}
	}
	return nil, -1
}

// literalString returns the value of a span that is one string literal with
// nothing interpolated.
func literalString(toks []parser.Token, s tokenSpan) (string, bool) {
	if s.end-s.start != 1 || toks[s.start].Kind != parser.TokenStringLiteral {
		return "", false
	}
	v := toks[s.start].Value
	if len(v) < 2 || (v[0] == '"' && strings.ContainsAny(v, "${")) {
		return "", false
	}
	return v[1 : len(v)-1], true
}

// readData reads a render's data argument: an array literal, compact(), or
// a variable assigned an array literal earlier in the same function.
func readData(uri, source string, toks []parser.Token, s tokenSpan, call int) ([]ViewVar, bool) {
	if s.end <= s.start {
		return nil, false
	}
	first := toks[s.start]
	switch {
	case first.Kind == parser.TokenOpenBracket && matchingClose(toks, s.start) == s.end-1:
		return readArray(source, toks, tokenSpan{s.start + 1, s.end - 1})
	case first.Kind == parser.TokenIdentifier && strings.EqualFold(first.Value, "compact") && s.end-s.start >= 3:
		return readCompact(toks, s)
	case first.Kind == parser.TokenVariable && s.end-s.start == 1:
		return readAssignedArray(source, toks, first.Value, call)
	}
	return nil, true
}

// readWith reads a `->with()` call: with('key', $value), with([...]) or
// withKey($value).
func readWith(uri, source string, toks []parser.Token, method string, args []tokenSpan, call int) ([]ViewVar, bool) {
	if method != "with" {
		if len(args) != 1 || len(method) <= 4 {
			return nil, true
		}
		key := toks[args[0].start-2] // the method name token
		name := strings.ToLower(method[4:5]) + method[5:]
		v := exprVar(source, toks, name, args[0])
		v.Line, v.Col, v.EndCol = key.Line, key.Column+4, key.Column+len(method)
		return []ViewVar{v}, false
	}
	switch len(args) {
	case 1:
		return readData(uri, source, toks, args[0], call)
	case 2:
		name, ok := literalString(toks, args[0])
		if !ok {
			return nil, true
		}
		v := exprVar(source, toks, name, args[1])
		key := toks[args[0].start]
		v.Line, v.Col, v.EndCol = key.Line, key.Column+1, key.Column+1+len(name)
		return []ViewVar{v}, false
	}
	return nil, true
}

// readArray reads `'key' => expr` items; any other item makes the data
// opaque.
func readArray(source string, toks []parser.Token, inner tokenSpan) ([]ViewVar, bool) {
	var vars []ViewVar
	opaque := false
	for _, item := range splitItems(toks, inner) {
		arrow := -1
		for k := item.start; k < item.end; k++ {
			if toks[k].Kind == parser.TokenDoubleArrow {
				arrow = k
				break
			}
		}
		if arrow < 0 {
			opaque = true
			continue
		}
		name, ok := literalString(toks, tokenSpan{item.start, arrow})
		if !ok || !isVarName(name) {
			opaque = true
			continue
		}
		v := exprVar(source, toks, name, tokenSpan{arrow + 1, item.end})
		key := toks[item.start]
		v.Line, v.Col, v.EndCol = key.Line, key.Column+1, key.Column+1+len(name)
		vars = append(vars, v)
	}
	return vars, opaque
}

func readCompact(toks []parser.Token, s tokenSpan) ([]ViewVar, bool) {
	args, close := splitTokenArgs(toks, s.start+1)
	if close != s.end-1 {
		return nil, true
	}
	var vars []ViewVar
	opaque := false
	for _, a := range args {
		name, ok := literalString(toks, a)
		if !ok || !isVarName(name) {
			opaque = true
			continue
		}
		t := toks[a.start]
		vars = append(vars, ViewVar{
			Name: name, Line: t.Line, Col: t.Column + 1, EndCol: t.Column + 1 + len(name),
			Expr: "$" + name, ExprLine: t.Line, ExprCol: t.Column,
		})
	}
	return vars, opaque
}

// readAssignedArray reads `$data = [...]` assigned before the call in the
// same function, and the `$data['key'] = expr;` that follow it.
func readAssignedArray(source string, toks []parser.Token, name string, call int) ([]ViewVar, bool) {
	start := -1
	for k := call - 1; k >= 0; k-- {
		if toks[k].Kind == parser.TokenFunction {
			break
		}
		if toks[k].Kind == parser.TokenVariable && toks[k].Value == name && k+2 < len(toks) &&
			toks[k+1].Kind == parser.TokenEquals && toks[k+2].Kind == parser.TokenOpenBracket {
			start = k
			break
		}
	}
	if start < 0 {
		return nil, true
	}
	close := matchingClose(toks, start+2)
	if close < 0 || close > call {
		return nil, true
	}
	vars, opaque := readArray(source, toks, tokenSpan{start + 3, close})
	// $data['key'] = expr; between the assignment and the call.
	for k := close; k < call; k++ {
		if toks[k].Kind != parser.TokenVariable || toks[k].Value != name {
			continue
		}
		if k+4 < call && toks[k+1].Kind == parser.TokenOpenBracket && toks[k+3].Kind == parser.TokenCloseBracket && toks[k+4].Kind == parser.TokenEquals {
			key, ok := literalString(toks, tokenSpan{k + 2, k + 3})
			if !ok || !isVarName(key) {
				opaque = true
				continue
			}
			end := k + 5
			for end < call && toks[end].Kind != parser.TokenSemicolon {
				end++
			}
			v := exprVar(source, toks, key, tokenSpan{k + 5, end})
			kt := toks[k+2]
			v.Line, v.Col, v.EndCol = kt.Line, kt.Column+1, kt.Column+1+len(key)
			vars = append(vars, v)
			continue
		}
		if k+1 < call && toks[k+1].Kind != parser.TokenOpenBracket && toks[k+1].Kind != parser.TokenComma && toks[k+1].Kind != parser.TokenCloseParen {
			// Something else is done with the array: it may change.
			opaque = true
		}
	}
	return vars, opaque
}

// splitItems splits array items at top-level commas.
func splitItems(toks []parser.Token, s tokenSpan) []tokenSpan {
	var items []tokenSpan
	depth, start := 0, s.start
	for k := s.start; k < s.end; k++ {
		switch toks[k].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				if k > start {
					items = append(items, tokenSpan{start, k})
				}
				start = k + 1
			}
		}
	}
	if s.end > start {
		items = append(items, tokenSpan{start, s.end})
	}
	return items
}

func matchingClose(toks []parser.Token, open int) int {
	depth := 0
	for k := open; k < len(toks); k++ {
		switch toks[k].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

// exprVar is a variable whose value is the expression in s.
func exprVar(source string, toks []parser.Token, name string, s tokenSpan) ViewVar {
	v := ViewVar{Name: name}
	if s.end <= s.start {
		return v
	}
	first, last := toks[s.start], toks[s.end-1]
	end := last.Offset + len(last.Value)
	if first.Offset < end && end <= len(source) {
		v.Expr = strings.TrimSpace(source[first.Offset:end])
		v.ExprLine, v.ExprCol = first.Line, first.Column
	}
	return v
}

func isVarName(s string) bool {
	if s == "" || ('0' <= s[0] && s[0] <= '9') {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80) {
			return false
		}
	}
	return true
}

// parseViewEdges reads the views a Blade view renders inside it, and the
// data it passes them.
func parseViewEdges(uri, from, source string) ([]viewEdge, int) {
	nodes := blade.Parse(source)
	lines := blade.NewLines(source)
	var edges []viewEdge
	for _, inc := range blade.Includes(source, nodes) {
		e := viewEdge{from: from, to: inc.View, isolated: inc.Isolated}
		for _, v := range inc.Vars {
			line, col := lines.Position(v.Key.Start)
			el, ec := lines.Position(v.Value.Start)
			vv := ViewVar{Name: v.Name, URI: uri, Line: line, Col: col, EndCol: col + len(v.Name), FromTemplate: true}
			if v.Value.End > v.Value.Start {
				vv.Expr = source[v.Value.Start:v.Value.End]
				vv.ExprLine, vv.ExprCol = el, ec
			}
			e.vars = append(e.vars, vv)
		}
		edges = append(edges, e)
	}
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].to < edges[j].to })
	return edges, blade.DynamicIncludes(source, nodes)
}
