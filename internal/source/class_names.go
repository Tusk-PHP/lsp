package sourcectx

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func (n *Namespaces) ClassNameAt(line string, lineStart uint32, col, length int) (fqn string, isName bool) {
	start := lineStart + uint32(col)
	end := start + uint32(length)
	b := n.block(start)
	if b.nameAt.Start <= start && end <= b.nameAt.End {
		return "", true
	}
	if fqn, inImport := b.importAt(start, end); inImport {
		return fqn, true
	}
	written, kind := line[col:col+length], token.Identifier
	if col > 0 && line[col-1] == '\\' || col+length < len(line) && line[col+length] == '\\' {
		tok, ok := nameTokenAt(line, col)
		if !ok {
			return "", false
		}
		if int(tok.End) != col+length {
			return "", true
		}
		written, kind = tok.Text(line), tok.Kind
	}
	return b.at(start).resolveClass(written, kind), true
}

func (b *namespaceBlock) importAt(start, end uint32) (string, bool) {
	k := b.importsBefore(start)
	inImport := false
	for i := k - 1; i >= 0 && b.imports[i].stmt.End > start; i-- {
		inImport = true
		if b.imports[i].nameEnd != end {
			continue
		}
		if b.uses[i].Kind != "class" {
			return "", true
		}
		return b.uses[i].FullName, true
	}
	return "", inImport
}

func nameTokenAt(line string, col int) (lexer.Token, bool) {
	tokens := lexer.LexCode(line, token.Latest)
	i := sort.Search(len(tokens), func(i int) bool { return int(tokens[i].End) > col })
	switch tok := tokens[i]; tok.Kind {
	case token.Identifier, token.NameQualified, token.NameFullyQualified, token.NameRelative:
		return tok, true
	}
	return lexer.Token{}, false
}
