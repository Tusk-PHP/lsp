package lexer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestStatementStart(t *testing.T) {
	for _, tc := range []struct {
		name, source, from, want string
	}{
		{"after the open tag on its line", "<?php $a = 1;", "1", "$a"},
		{"after the open tag on its own line", "<?php\n/** doc */\n$a = 1;", "1", "$a"},
		{"after an echo tag", "<?= $a . 1 ?>", "1", "$a"},
		{"after a semicolon", "<?php $a = 1; $b = 2;", "2", "$b"},
		{"after a close tag and inline HTML", "<?php $a = 1 ?><p>x</p><?php $b = 2;", "2", "$b"},
		{"after an opening brace", "<?php function f() { return g(1); }", "1", "return"},
		{"after a closing brace", "<?php if ($x) {} $b = 2;", "2", "$b"},
		{"skipping a closure passed earlier in the chain", "<?php $r = f(function () { return 1; })->g(2);", "2", "$r"},
		{"skipping an attribute", "<?php $r = #[A(1)] fn() => 2;", "2", "$r"},
		{"inside unclosed parentheses", "<?php $r = f(g(1), 2", "2", "$r"},
		{"a snippet without an open tag", "$a = [1, (2)];", "2", "$a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := tc.source
			lexed := Lex(src, token.Latest)
			if src[0] != '<' {
				lexed = LexCode(src, token.Latest)
			}
			for _, filter := range []struct {
				name   string
				tokens []Token
			}{
				{"all tokens", lexed},
				{"significant tokens", Significant(lexed)},
			} {
				tokens := filter.tokens
				from := -1
				for i, tok := range tokens {
					if tok.Text(src) == tc.from {
						from = i
					}
				}
				if from < 0 {
					t.Fatalf("%s: no token %q", filter.name, tc.from)
				}
				if got := StatementStart(tokens, from); tokens[got].Text(src) != tc.want {
					t.Errorf("%s: StatementStart = %q, want %q", filter.name, tokens[got].Text(src), tc.want)
				}
			}
		})
	}
}
