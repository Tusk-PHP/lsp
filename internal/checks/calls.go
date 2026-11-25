package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type callKind int

const (
	callFunction callKind = iota
	callStatic
	callMethod
	callConstructor
)

type callee struct {
	sym      *symbols.Symbol
	forwards bool
}

func (c callee) checkable() bool {
	return c.sym.Source != symbols.SourceBuiltin && !hasVariadicParam(c.sym.Params) && !c.forwards
}

func allCheckable(callees []callee) bool {
	for _, c := range callees {
		if !c.checkable() {
			return false
		}
	}
	return true
}

type resolvedCall struct {
	kind    callKind
	callees []callee
	site    parser.Token
	name    string
	at      int
	open    int
	close   int
	tokens  []parser.Token
}

func eachResolvedCall(file *parser.FileNode, source string, index *symbols.Index, shared *Shared, typeResolver func(string, string, int, int, *parser.FileNode) string, wants func(tokens []parser.Token, open, close int) bool, visit func(resolvedCall)) {
	tokens := shared.parse(source).Tokens
	var lines []string
	var scopes *scope.Document
	arguments := func(open int) (int, bool) {
		closeIdx := parser.MatchingClose(tokens, open)
		return closeIdx, closeIdx >= 0 && (wants == nil || wants(tokens, open, closeIdx))
	}
	emit := func(kind callKind, callees []callee, site parser.Token, name string, at, open, closeIdx int) {
		if len(callees) > 0 {
			visit(resolvedCall{kind: kind, callees: callees, site: site, name: name, at: at, open: open, close: closeIdx, tokens: tokens})
		}
	}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind != parser.TokenIdentifier || isSkippedCallKeyword(tok.Value) {
			continue
		}
		next := parser.NextSignificant(tokens, i+1)
		if tokens[next].Kind == parser.TokenDoubleColon {
			methodIdx := parser.NextSignificant(tokens, next+1)
			if tokens[methodIdx].Kind != parser.TokenIdentifier {
				continue
			}
			openIdx := parser.NextSignificant(tokens, methodIdx+1)
			if tokens[openIdx].Kind != parser.TokenOpenParen {
				continue
			}
			closeIdx, wanted := arguments(openIdx)
			if !wanted {
				continue
			}
			classFQN, ok := resolveClassLikeName(tok.Value, tok.Line, file)
			if !ok || classFQN == "" {
				continue
			}
			emit(callStatic, methodCallees(classFQN, tokens[methodIdx].Value, index), tok, tokens[methodIdx].Value, i, openIdx, closeIdx)
			continue
		}
		if tokens[next].Kind != parser.TokenOpenParen {
			continue
		}
		closeIdx, wanted := arguments(next)
		if !wanted {
			continue
		}
		prev := parser.PrevSignificant(tokens, i-1)
		if prev >= 0 && tokens[prev].Kind == parser.TokenArrow {
			if typeResolver == nil {
				continue
			}
			if lines == nil {
				lines = strings.Split(source, "\n")
			}
			callees, receiver := calledMethod(tokens, prev, tok, lines, source, file, index, typeResolver)
			if len(callees) == 0 {
				continue
			}
			if receiver == "$this" || strings.HasPrefix(receiver, "$this->") || strings.HasPrefix(receiver, "$this?->") {
				if scopes == nil {
					scopes = scope.Collect(source)
				}
				if inClosure(scopes, tok) {
					continue
				}
			}
			emit(callMethod, callees, tok, tok.Value, i, next, closeIdx)
			continue
		}
		if prev >= 0 && tokens[prev].Kind == parser.TokenNew {
			classFQN, ok := resolveClassLikeName(tok.Value, tok.Line, file)
			if !ok || classFQN == "" {
				continue
			}
			emit(callConstructor, methodCallees(classFQN, "__construct", index), tok, tok.Value, i, next, closeIdx)
			continue
		}
		if prev >= 0 {
			switch tokens[prev].Kind {
			case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenFunction:
				continue
			case parser.TokenAmpersand:
				if pp := parser.PrevSignificant(tokens, prev-1); pp >= 0 && tokens[pp].Kind == parser.TokenFunction {
					continue
				}
			}
		}
		fqns, _ := resolve.FunctionNames(tok.Value, file)
		emit(callFunction, functionCallees(fqns, index), tok, tok.Value, i, next, closeIdx)
	}
}

func functionCallees(fqns []string, index *symbols.Index) []callee {
	for _, fqn := range fqns {
		var callees []callee
		for _, def := range index.Definitions(fqn) {
			if def.Kind == symbols.KindFunction {
				callees = append(callees, callee{sym: def})
			}
		}
		if len(callees) > 0 {
			return callees
		}
	}
	return nil
}

func methodCallees(classFQN, method string, index *symbols.Index) []callee {
	var callees []callee
	for _, def := range index.Definitions(classFQN) {
		if !def.IsClassLike() {
			continue
		}
		c, ok := definitionMethod(index.DefinitionMembers(def), method)
		if !ok {
			return nil
		}
		callees = append(callees, c)
	}
	return callees
}

func definitionMethod(members []*symbols.Symbol, name string) (callee, bool) {
	var c callee
	for _, m := range members {
		if m.Kind != symbols.KindMethod {
			continue
		}
		if c.sym == nil && m.Name == name {
			c.sym = m
		}
		if m.Name == "__call" || m.Name == "__callStatic" {
			c.forwards = true
		}
	}
	return c, c.sym != nil
}
