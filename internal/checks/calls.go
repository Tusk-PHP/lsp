package checks

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
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

type resolvedCall struct {
	kind   callKind
	sym    *symbols.Symbol
	owner  string
	site   parser.Token
	name   string
	at     int
	open   int
	close  int
	tokens []parser.Token
}

func eachResolvedCall(file *parser.FileNode, source string, index *symbols.Index, shared *Shared, typeResolver func(string, string, int, int, *parser.FileNode) string, wants func(tokens []parser.Token, open, close int) bool, visit func(resolvedCall)) {
	tokens := shared.parse(source).Tokens
	var lines []string
	var scopes *scope.Document
	arguments := func(open int) (int, bool) {
		closeIdx := findMatchingTokenIdx(tokens, open, parser.TokenOpenParen, parser.TokenCloseParen)
		return closeIdx, closeIdx >= 0 && (wants == nil || wants(tokens, open, closeIdx))
	}
	emit := func(kind callKind, sym *symbols.Symbol, owner string, site parser.Token, name string, at, open, closeIdx int) {
		visit(resolvedCall{kind: kind, sym: sym, owner: owner, site: site, name: name, at: at, open: open, close: closeIdx, tokens: tokens})
	}
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if tok.Kind != parser.TokenIdentifier || isSkippedCallKeyword(tok.Value) {
			continue
		}
		next := nextSignificantTokenIdx(tokens, i+1)
		if tokens[next].Kind == parser.TokenDoubleColon {
			methodIdx := nextSignificantTokenIdx(tokens, next+1)
			if tokens[methodIdx].Kind != parser.TokenIdentifier {
				continue
			}
			openIdx := nextSignificantTokenIdx(tokens, methodIdx+1)
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
			if sym := findProjectMethod(classFQN, tokens[methodIdx].Value, index); sym != nil {
				emit(callStatic, sym, classFQN, tok, tokens[methodIdx].Value, i, openIdx, closeIdx)
			}
			continue
		}
		if tokens[next].Kind != parser.TokenOpenParen {
			continue
		}
		closeIdx, wanted := arguments(next)
		if !wanted {
			continue
		}
		prev := prevSignificantTokenIdx(tokens, i-1)
		if prev >= 0 && tokens[prev].Kind == parser.TokenArrow {
			if typeResolver == nil {
				continue
			}
			if lines == nil {
				lines = strings.Split(source, "\n")
			}
			sym, owner, receiver := calledMethod(tokens, prev, tok, lines, source, file, index, typeResolver)
			if sym == nil {
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
			emit(callMethod, sym, owner, tok, tok.Value, i, next, closeIdx)
			continue
		}
		if prev >= 0 && tokens[prev].Kind == parser.TokenNew {
			classFQN, ok := resolveClassLikeName(tok.Value, tok.Line, file)
			if !ok || classFQN == "" {
				continue
			}
			if sym := findProjectMethod(classFQN, "__construct", index); sym != nil {
				emit(callConstructor, sym, classFQN, tok, tok.Value, i, next, closeIdx)
			}
			continue
		}
		if prev >= 0 {
			switch tokens[prev].Kind {
			case parser.TokenArrow, parser.TokenDoubleColon, parser.TokenFunction:
				continue
			case parser.TokenAmpersand:
				if pp := prevSignificantTokenIdx(tokens, prev-1); pp >= 0 && tokens[pp].Kind == parser.TokenFunction {
					continue
				}
			}
		}
		fqns, _ := resolveFunctionNames(tok.Value, file)
		if sym := findProjectFunction(fqns, index); sym != nil {
			emit(callFunction, sym, "", tok, tok.Value, i, next, closeIdx)
		}
	}
}
