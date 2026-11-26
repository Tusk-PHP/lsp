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

var commentURL = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\[\]{}]+`)

const urlTrailingPunctuation = ".,;:!?"

var includeKeywords = map[string]bool{"require": true, "require_once": true, "include": true, "include_once": true}

var memberOrDeclarationPrefixes = map[parser.TokenKind]bool{parser.TokenArrow: true, parser.TokenDoubleColon: true, parser.TokenFunction: true}

func (a *Analyzer) DocumentLinks(uri, source, root string, exists func(path string) bool) []protocol.DocumentLink {
	links := make([]protocol.DocumentLink, 0)
	tokens := parser.Tokenize(source)
	file, _ := symbols.URIToPath(uri)
	laravel := a.framework == "laravel" && root != ""
	for i, tok := range tokens {
		switch {
		case (tok.Kind == parser.TokenComment || tok.Kind == parser.TokenDocComment) && strings.Contains(tok.Value, "://"):
			links = append(links, commentURLLinks(source, tok)...)
		case tok.Kind != parser.TokenIdentifier || namesMemberOrNamespacedFunction(tokens, i):
		case includeKeywords[strings.ToLower(tok.Value)]:
			if link, ok := includeLink(source, tokens[i+1:], file, exists); ok {
				links = append(links, link)
			}
		case laravel:
			if link, ok := pathHelperLink(source, tokens[i:], root, exists); ok {
				links = append(links, link)
			}
		}
	}
	return links
}

func namesMemberOrNamespacedFunction(tokens []parser.Token, i int) bool {
	if i == 0 {
		return false
	}
	prev := tokens[i-1].Kind
	return memberOrDeclarationPrefixes[prev] || (prev == parser.TokenBackslash && i > 1 && tokens[i-2].Kind == parser.TokenIdentifier)
}

func commentURLLinks(source string, tok parser.Token) []protocol.DocumentLink {
	text := tok.Value
	if strings.HasPrefix(text, "/*") {
		text = strings.TrimSuffix(text, "*/")
	}
	var links []protocol.DocumentLink
	for _, m := range commentURL.FindAllStringIndex(text, -1) {
		url := trimURL(text[m[0]:m[1]])
		start := position(source, tok, m[0])
		end := position(source, tok, m[0]+len(url))
		links = append(links, protocol.DocumentLink{Range: protocol.Range{Start: start, End: end}, Target: url})
	}
	return links
}

func trimURL(url string) string {
	for {
		trimmed := strings.TrimRight(url, urlTrailingPunctuation)
		if strings.HasSuffix(trimmed, ")") && strings.Count(trimmed, "(") < strings.Count(trimmed, ")") {
			trimmed = trimmed[:len(trimmed)-1]
		}
		if trimmed == url {
			return url
		}
		url = trimmed
	}
}

func includeLink(source string, tokens []parser.Token, file string, exists func(string) bool) (protocol.DocumentLink, bool) {
	e := pathExpr{tokens: tokens, file: file}
	path, ok := e.concat()
	path = filepath.Clean(path)
	if !ok || e.i >= len(tokens) || tokens[e.i].Kind != parser.TokenSemicolon || !filepath.IsAbs(path) || !exists(path) {
		return protocol.DocumentLink{}, false
	}
	return fileLink(source, tokens[0], tokens[e.i-1], path), true
}

func pathHelperLink(source string, tokens []parser.Token, root string, exists func(string) bool) (protocol.DocumentLink, bool) {
	helperDir, ok := frameworklaravel.PathHelperDir(tokens[0].Value)
	if !ok || len(tokens) < 3 || tokens[1].Kind != parser.TokenOpenParen {
		return protocol.DocumentLink{}, false
	}
	if tokens[2].Kind == parser.TokenCloseParen {
		return existingFileLink(source, tokens[0], tokens[2], filepath.Join(root, helperDir), exists)
	}
	if len(tokens) < 4 || tokens[3].Kind != parser.TokenCloseParen {
		return protocol.DocumentLink{}, false
	}
	relative, ok := parser.ConstantString(tokens[2])
	if !ok {
		return protocol.DocumentLink{}, false
	}
	return existingFileLink(source, tokens[2], tokens[2], filepath.Join(root, helperDir, relative), exists)
}

func existingFileLink(source string, first, last parser.Token, path string, exists func(string) bool) (protocol.DocumentLink, bool) {
	if !exists(path) {
		return protocol.DocumentLink{}, false
	}
	return fileLink(source, first, last, path), true
}

func fileLink(source string, first, last parser.Token, path string) protocol.DocumentLink {
	return protocol.DocumentLink{
		Range:  protocol.Range{Start: position(source, first, 0), End: position(source, last, len(last.Value))},
		Target: symbols.PathToURI(path),
	}
}

func position(source string, tok parser.Token, offset int) protocol.Position {
	at := tok.Offset + offset
	lineStart := strings.LastIndexByte(source[:at], '\n') + 1
	return protocol.Position{Line: tok.Line + strings.Count(tok.Value[:offset], "\n"), Character: at - lineStart}
}

type pathExpr struct {
	tokens []parser.Token
	i      int
	file   string
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
		return filepath.Dir(e.file), e.file != ""
	case tok.Kind == parser.TokenIdentifier && strings.EqualFold(tok.Value, "__FILE__"):
		return e.file, e.file != ""
	case tok.Kind == parser.TokenIdentifier:
		return e.globalName(tok.Value)
	case tok.Kind == parser.TokenBackslash && e.i < len(e.tokens) && e.tokens[e.i].Kind == parser.TokenIdentifier:
		e.i++
		return e.globalName(e.tokens[e.i-1].Value)
	}
	return "", false
}

func (e *pathExpr) globalName(name string) (string, bool) {
	switch {
	case strings.EqualFold(name, "dirname"):
		return e.dirname()
	case name == "DIRECTORY_SEPARATOR":
		return string(filepath.Separator), true
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
