package parser

import (
	"fmt"
	"math/bits"
	"strings"
)

type tokenSpan struct {
	start, end int
}

type importKinds uint8

const (
	classImport importKinds = 1 << iota
	functionImport
	constImport

	foldedImports = classImport | functionImport
)

func importKindOf(kind string) importKinds {
	switch kind {
	case "class":
		return classImport
	case "function":
		return functionImport
	case "const":
		return constImport
	}
	panic(fmt.Sprintf("parser: unknown import kind %q", kind))
}

type ImportReferences struct {
	used map[string]importKinds
}

type importCollector struct {
	imported map[string]importKinds
	spelled  map[string]importKinds
	used     map[string]importKinds
	pending  int
}

func (r *ParseResult) ImportReferences() ImportReferences {
	c := importCollector{
		imported: make(map[string]importKinds, len(r.Uses)),
		spelled:  make(map[string]importKinds, len(r.Uses)),
		used:     make(map[string]importKinds, len(r.Uses)),
	}
	var buf [64]byte
	for _, u := range r.Uses {
		kind := importKindOf(u.Kind)
		c.spelled[u.Alias] |= kind
		key := appendImportKey(buf[:0], u.Alias, kind)
		if c.imported[string(key)]&kind == 0 {
			c.imported[string(key)] |= kind
			c.pending++
		}
	}
	c.collect(r.Tokens, r.importSpans)
	return ImportReferences{used: c.used}
}

func (refs ImportReferences) Used(u UseNode) bool {
	kind := importKindOf(u.Kind)
	var buf [64]byte
	return refs.used[string(appendImportKey(buf[:0], u.Alias, kind))]&kind != 0
}

func appendImportKey(dst []byte, name string, kinds importKinds) []byte {
	if kinds&foldedImports == 0 {
		return append(dst, name...)
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		dst = append(dst, c)
	}
	return dst
}

func (c *importCollector) collect(tokens []Token, skip []tokenSpan) {
	for i := 0; i < len(tokens) && c.pending > 0; i++ {
		if len(skip) > 0 && i == skip[0].start {
			i = skip[0].end - 1
			skip = skip[1:]
			continue
		}
		switch tok := tokens[i]; tok.Kind {
		case TokenIdentifier:
			c.mark(tok.Value, nameImportKinds(tokens, i))
		case TokenDocComment:
			c.markWords(tok.Value)
		case TokenComment:
			if isAnnotationComment(tok.Value) {
				c.markWords(tok.Value)
			}
		case TokenStringLiteral:
			c.collect(InterpolatedTokens(tok), nil)
		}
	}
}

func nameImportKinds(tokens []Token, i int) importKinds {
	switch significantKind(tokens, PrevSignificant(tokens, i-1)) {
	case TokenBackslash, TokenArrow, TokenDoubleColon,
		TokenNamespace, TokenFunction, TokenClass, TokenInterface, TokenTrait, TokenEnum:
		return 0
	case TokenNew:
		return classImport
	}
	switch significantKind(tokens, NextSignificant(tokens, i+1)) {
	case TokenBackslash, TokenDoubleColon:
		return classImport
	case TokenOpenParen:
		return classImport | functionImport
	}
	return classImport | constImport
}

func significantKind(tokens []Token, i int) TokenKind {
	if i < 0 {
		return TokenEOF
	}
	return tokens[i].Kind
}

func isAnnotationComment(comment string) bool {
	body, ok := strings.CutPrefix(comment, "/*")
	return ok && strings.HasPrefix(strings.TrimLeft(body, " \t\r\n"), "@")
}

func (c *importCollector) markWords(text string) {
	for i := 0; i < len(text); {
		if !IsIdentByte(text[i]) {
			i++
			continue
		}
		start := i
		for i < len(text) && IsIdentByte(text[i]) {
			i++
		}
		word := text[start:i]
		if kinds := c.spelled[word]; kinds != 0 {
			c.mark(word, kinds)
		}
	}
}

func (c *importCollector) mark(name string, kinds importKinds) {
	var buf [64]byte
	if exact := kinds &^ foldedImports; exact != 0 {
		c.markKey(appendImportKey(buf[:0], name, exact), exact)
	}
	if folded := kinds & foldedImports; folded != 0 {
		c.markKey(appendImportKey(buf[:0], name, folded), folded)
	}
}

func (c *importCollector) markKey(key []byte, kinds importKinds) {
	hit := c.imported[string(key)] & kinds &^ c.used[string(key)]
	if hit == 0 {
		return
	}
	c.used[string(key)] |= hit
	c.pending -= bits.OnesCount8(uint8(hit))
}
