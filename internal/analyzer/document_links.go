package analyzer

import (
	"cmp"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var commentURL = regexp.MustCompile(`https?://[^\s<>"'` + "`" + `\[\]{}]+`)

const urlTrailingPunctuation = ".,;:!?"

var includeKeywords = map[token.Kind]bool{token.Require: true, token.RequireOnce: true, token.Include: true, token.IncludeOnce: true}

type linkTokens struct {
	src   string
	lines ast.Lines
	all   []lexer.Token
}

func (t linkTokens) kind(i int) token.Kind {
	if i < 0 || i >= len(t.all) {
		return token.Invalid
	}
	return t.all[i].Kind
}

func (t linkTokens) text(i int) string {
	return t.all[i].Text(t.src)
}

func (t linkTokens) position(offset uint32) protocol.Position {
	line, col := t.lines.Position(offset)
	return protocol.Position{Line: line, Character: col}
}

func (t linkTokens) globalName(i int) (string, bool) {
	switch t.kind(i) {
	case token.Identifier:
		return t.text(i), true
	case token.NameFullyQualified:
		name := t.text(i)[1:]
		return name, !strings.Contains(name, `\`)
	}
	return "", false
}

func (a *Analyzer) DocumentLinks(uri, source, root string, exists func(path string) bool) []protocol.DocumentLink {
	links := make([]protocol.DocumentLink, 0)
	all := lexer.Lex(source, token.Latest)
	toks := linkTokens{src: source, lines: ast.NewLines(source), all: lexer.WholeLiterals(lexer.Significant(all))}
	for _, tok := range all {
		if (tok.Kind == token.Comment || tok.Kind == token.DocComment) && strings.Contains(tok.Text(source), "://") {
			links = append(links, commentURLLinks(toks, tok)...)
		}
	}
	file := symbols.URIToPath(uri)
	laravel := a.framework == "laravel" && root != ""
	for i := range toks.all {
		switch {
		case includeKeywords[toks.kind(i)]:
			if link, ok := includeLink(toks, i+1, file, exists); ok {
				links = append(links, link)
			}
		case laravel:
			if link, ok := pathHelperLink(toks, i, root, exists); ok {
				links = append(links, link)
			}
		}
	}
	slices.SortStableFunc(links, func(a, b protocol.DocumentLink) int {
		return cmp.Or(cmp.Compare(a.Range.Start.Line, b.Range.Start.Line), cmp.Compare(a.Range.Start.Character, b.Range.Start.Character))
	})
	return links
}

func commentURLLinks(toks linkTokens, tok lexer.Token) []protocol.DocumentLink {
	text := tok.Text(toks.src)
	if strings.HasPrefix(text, "/*") {
		text = strings.TrimSuffix(text, "*/")
	}
	var links []protocol.DocumentLink
	for _, m := range commentURL.FindAllStringIndex(text, -1) {
		url := trimURL(text[m[0]:m[1]])
		start := toks.position(tok.Start + uint32(m[0]))
		end := toks.position(tok.Start + uint32(m[0]+len(url)))
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

func includeLink(toks linkTokens, from int, file string, exists func(string) bool) (protocol.DocumentLink, bool) {
	e := pathExpr{toks: toks, i: from, file: file}
	path, ok := e.concat()
	path = filepath.Clean(path)
	if end := toks.kind(e.i); !ok || (end != token.Semicolon && end != token.CloseTag) || !filepath.IsAbs(path) || !exists(path) {
		return protocol.DocumentLink{}, false
	}
	return fileLink(toks, from, e.i-1, path), true
}

func pathHelperLink(toks linkTokens, at int, root string, exists func(string) bool) (protocol.DocumentLink, bool) {
	name, ok := toks.globalName(at)
	if !ok || isMemberOrDeclaration(toks.kind(at-1)) || toks.kind(at+1) != token.LeftParen {
		return protocol.DocumentLink{}, false
	}
	helperDir, ok := frameworklaravel.PathHelperDir(name)
	if !ok {
		return protocol.DocumentLink{}, false
	}
	if toks.kind(at+2) == token.RightParen {
		return existingFileLink(toks, at, at+2, filepath.Join(root, helperDir), exists)
	}
	if toks.kind(at+2) != token.ConstantString || toks.kind(at+3) != token.RightParen {
		return protocol.DocumentLink{}, false
	}
	relative, ok := lexer.Unquote(toks.text(at + 2))
	if !ok {
		return protocol.DocumentLink{}, false
	}
	return existingFileLink(toks, at+2, at+2, filepath.Join(root, helperDir, relative), exists)
}

func isMemberOrDeclaration(kind token.Kind) bool {
	return kind == token.ObjectOperator || kind == token.NullsafeObjectOperator || kind == token.DoubleColon || kind == token.Function
}

func existingFileLink(toks linkTokens, first, last int, path string, exists func(string) bool) (protocol.DocumentLink, bool) {
	if !exists(path) {
		return protocol.DocumentLink{}, false
	}
	return fileLink(toks, first, last, path), true
}

func fileLink(toks linkTokens, first, last int, path string) protocol.DocumentLink {
	return protocol.DocumentLink{
		Range:  protocol.Range{Start: toks.position(toks.all[first].Start), End: toks.position(toks.all[last].End)},
		Target: symbols.PathToURI(path),
	}
}

type pathExpr struct {
	toks linkTokens
	i    int
	file string
}

func (e *pathExpr) concat() (string, bool) {
	value, ok := e.term()
	for ok && e.toks.kind(e.i) == token.Dot {
		e.i++
		var next string
		next, ok = e.term()
		value += next
	}
	return value, ok
}

func (e *pathExpr) term() (string, bool) {
	at := e.i
	e.i++
	switch e.toks.kind(at) {
	case token.ConstantString:
		return lexer.Unquote(e.toks.text(at))
	case token.LeftParen:
		value, ok := e.concat()
		return value, ok && e.expect(token.RightParen)
	case token.DirConst:
		return filepath.Dir(e.file), true
	case token.FileConst:
		return e.file, true
	}
	if name, ok := e.toks.globalName(at); ok {
		return e.globalName(name)
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
	if !e.expect(token.LeftParen) {
		return "", false
	}
	path, ok := e.concat()
	levels := 1
	if ok && e.toks.kind(e.i) == token.Comma {
		e.i++
		ok = e.toks.kind(e.i) == token.IntegerLiteral
		if ok {
			levels, ok = parseLevels(e.toks.text(e.i))
			e.i++
		}
	}
	if !ok || !e.expect(token.RightParen) {
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

func (e *pathExpr) expect(kind token.Kind) bool {
	if e.toks.kind(e.i) != kind {
		return false
	}
	e.i++
	return true
}
