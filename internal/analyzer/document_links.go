package analyzer

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var commentURL = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `()\[\]{}]+`)

const urlTrailingPunctuation = ".,;:!?"

var includeKeywords = map[string]bool{"require": true, "require_once": true, "include": true, "include_once": true}

func (a *Analyzer) DocumentLinks(uri, source, root string, exists func(path string) bool) []protocol.DocumentLink {
	links := make([]protocol.DocumentLink, 0)
	tokens := parser.Tokenize(source)
	dir := filepath.Dir(symbols.URIToPath(uri))
	laravel := a.framework == "laravel" && root != ""
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		switch {
		case (tok.Kind == parser.TokenComment || tok.Kind == parser.TokenDocComment) && strings.Contains(tok.Value, "://"):
			links = append(links, commentURLLinks(tok)...)
		case tok.Kind == parser.TokenIdentifier && includeKeywords[strings.ToLower(tok.Value)]:
			if link, ok := includeLink(tokens[i+1:], dir, exists); ok {
				links = append(links, link)
			}
		case laravel && tok.Kind == parser.TokenIdentifier:
			if link, ok := pathHelperLink(tokens[i:], root, exists); ok {
				links = append(links, link)
			}
		}
	}
	return links
}

func commentURLLinks(tok parser.Token) []protocol.DocumentLink {
	var links []protocol.DocumentLink
	for _, m := range commentURL.FindAllStringIndex(tok.Value, -1) {
		url := strings.TrimRight(tok.Value[m[0]:m[1]], urlTrailingPunctuation)
		start := positionIn(tok, m[0])
		end := positionIn(tok, m[0]+len(url))
		links = append(links, protocol.DocumentLink{Range: protocol.Range{Start: start, End: end}, Target: url})
	}
	return links
}

func includeLink(tokens []parser.Token, dir string, exists func(string) bool) (protocol.DocumentLink, bool) {
	e := pathExpr{tokens: tokens, dir: dir}
	path, ok := e.concat()
	path = filepath.Clean(path)
	if !ok || e.i >= len(tokens) || tokens[e.i].Kind != parser.TokenSemicolon || !filepath.IsAbs(path) || !exists(path) {
		return protocol.DocumentLink{}, false
	}
	return fileLink(tokens[0], tokens[e.i-1], path), true
}

func pathHelperLink(tokens []parser.Token, root string, exists func(string) bool) (protocol.DocumentLink, bool) {
	helperDir, ok := frameworklaravel.PathHelperDir(tokens[0].Value)
	if !ok || len(tokens) < 4 || tokens[1].Kind != parser.TokenOpenParen || tokens[3].Kind != parser.TokenCloseParen {
		return protocol.DocumentLink{}, false
	}
	relative, ok := parser.ConstantString(tokens[2])
	if !ok {
		return protocol.DocumentLink{}, false
	}
	path := filepath.Join(root, helperDir, relative)
	if !exists(path) {
		return protocol.DocumentLink{}, false
	}
	return fileLink(tokens[2], tokens[2], path), true
}

func fileLink(first, last parser.Token, path string) protocol.DocumentLink {
	return protocol.DocumentLink{
		Range:  protocol.Range{Start: positionIn(first, 0), End: positionIn(last, len(last.Value))},
		Target: symbols.PathToURI(path),
	}
}

func positionIn(tok parser.Token, offset int) protocol.Position {
	before := tok.Value[:offset]
	newlines := strings.Count(before, "\n")
	if newlines == 0 {
		return protocol.Position{Line: tok.Line, Character: tok.Column + offset}
	}
	return protocol.Position{Line: tok.Line + newlines, Character: len(before) - strings.LastIndexByte(before, '\n') - 1}
}

type pathExpr struct {
	tokens []parser.Token
	i      int
	dir    string
}

func (e *pathExpr) concat() (string, bool) {
	value, ok := e.term()
	for ok && e.i < len(e.tokens) && e.tokens[e.i].Value == "." {
		e.i++
		var next string
		next, ok = e.term()
		value += next
	}
	return value, ok
}

func (e *pathExpr) term() (string, bool) {
	if e.i >= len(e.tokens) {
		return "", false
	}
	tok := e.tokens[e.i]
	e.i++
	switch {
	case tok.Kind == parser.TokenStringLiteral:
		return parser.ConstantString(tok)
	case tok.Kind == parser.TokenOpenParen:
		value, ok := e.concat()
		return value, ok && e.expect(parser.TokenCloseParen)
	case tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, "__DIR__"):
		return e.dir, true
	case tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, "dirname"):
		return e.dirname()
	}
	return "", false
}

func (e *pathExpr) dirname() (string, bool) {
	if !e.expect(parser.TokenOpenParen) {
		return "", false
	}
	path, ok := e.concat()
	levels := 1
	if ok && e.i < len(e.tokens) && e.tokens[e.i].Kind == parser.TokenComma {
		e.i++
		ok = e.i < len(e.tokens) && e.tokens[e.i].Kind == parser.TokenNumber
		if ok {
			levels, ok = parseLevels(e.tokens[e.i].Value)
			e.i++
		}
	}
	if !ok || !e.expect(parser.TokenCloseParen) {
		return "", false
	}
	for ; levels > 0; levels-- {
		path = filepath.Dir(path)
	}
	return path, true
}

func parseLevels(value string) (int, bool) {
	levels, err := strconv.Atoi(value)
	return levels, err == nil && levels >= 1
}

func (e *pathExpr) expect(kind parser.TokenKind) bool {
	if e.i >= len(e.tokens) || e.tokens[e.i].Kind != kind {
		return false
	}
	e.i++
	return true
}
