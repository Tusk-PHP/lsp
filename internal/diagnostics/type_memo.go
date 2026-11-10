package diagnostics

import (
	"strconv"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// TypeResolverFunc types a PHP expression read at a line of a file. col is
// the byte column where the expression starts on that line, or 0 when only
// the line is known.
type TypeResolverFunc = func(expr string, source string, line, col int, file *parser.FileNode) string

// MemoizeTypeResolver returns a type resolver that remembers its answers for
// one run of the rules over one version of a file. Several rules ask about
// the same receivers (unknown-member, deprecated, unused-result all type
// every `->` and `::`), and the answer doesn't change within a run. The memo
// belongs to the run: make a new one for each analysis.
func MemoizeTypeResolver(resolve TypeResolverFunc) TypeResolverFunc {
	if resolve == nil {
		return nil
	}
	memo := make(map[string]string)
	var memoSource string
	return func(expr string, source string, line, col int, file *parser.FileNode) string {
		if source != memoSource {
			// Another file (or version): what was learnt doesn't apply.
			clear(memo)
			memoSource = source
		}
		key := strconv.Itoa(line) + ":" + strconv.Itoa(col) + "\x00" + expr
		if t, ok := memo[key]; ok {
			return t
		}
		t := resolve(expr, source, line, col, file)
		memo[key] = t
		return t
	}
}
