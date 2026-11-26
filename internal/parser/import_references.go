package parser

import "strings"

type tokenSpan struct {
	start, end int
}

type ImportReferences struct {
	names map[string]struct{}
}

func (r *ParseResult) ImportReferences() ImportReferences {
	aliases := make(map[string]struct{}, len(r.Uses))
	for _, u := range r.Uses {
		aliases[u.Alias] = struct{}{}
	}
	refs := ImportReferences{names: make(map[string]struct{}, len(aliases))}
	refs.collect(r.Tokens, r.importSpans, aliases)
	return refs
}

func (refs ImportReferences) Used(u UseNode) bool {
	_, ok := refs.names[u.Alias]
	return ok
}

func (refs ImportReferences) collect(tokens []Token, skip []tokenSpan, aliases map[string]struct{}) {
	for i := 0; i < len(tokens) && len(refs.names) < len(aliases); i++ {
		if len(skip) > 0 && i == skip[0].start {
			i = skip[0].end - 1
			skip = skip[1:]
			continue
		}
		switch tok := tokens[i]; tok.Kind {
		case TokenIdentifier:
			refs.mark(tok.Value, aliases)
		case TokenDocComment:
			refs.markWords(tok.Value, aliases)
		case TokenComment:
			if isAnnotationComment(tok.Value) {
				refs.markWords(tok.Value, aliases)
			}
		case TokenStringLiteral:
			refs.collect(InterpolatedTokens(tok), nil, aliases)
		}
	}
}

func isAnnotationComment(comment string) bool {
	body, ok := strings.CutPrefix(comment, "/*")
	return ok && strings.HasPrefix(strings.TrimLeft(body, " \t\r\n"), "@")
}

func (refs ImportReferences) markWords(text string, aliases map[string]struct{}) {
	for i := 0; i < len(text); {
		if !IsIdentByte(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && IsIdentByte(text[i]) {
			i++
		}
		refs.mark(text[start:i], aliases)
	}
}

func (refs ImportReferences) mark(name string, aliases map[string]struct{}) {
	if _, ok := aliases[name]; ok {
		refs.names[name] = struct{}{}
	}
}
