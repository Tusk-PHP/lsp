package diagnostics

import (
	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

// MemoizeTypeResolver returns a type resolver that remembers its answers for
// one run of the rules over one version of a file. Several rules ask about
// the same receivers (unknown-member, deprecated, unused-result all type
// every `->` and `::`), and the answer doesn't change within a run. The memo
// belongs to the run: make a new one for each analysis.
func MemoizeTypeResolver(resolve checks.TypeResolver) checks.TypeResolver {
	if resolve == nil {
		return nil
	}
	memo := make(map[ast.Expr]string)
	var memoFile *parser.FileNode
	return func(file *parser.FileNode, e ast.Expr) string {
		if file != memoFile {
			clear(memo)
			memoFile = file
		}
		if t, ok := memo[e]; ok {
			return t
		}
		t := resolve(file, e)
		memo[e] = t
		return t
	}
}
