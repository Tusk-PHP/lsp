package parser

import "testing"

func TestStatementStart(t *testing.T) {
	for _, tc := range []struct {
		name, source, from, want string
	}{
		{"after the open tag on its line", "<?php $a = 1;", "1", "$a"},
		{"after the open tag on its own line", "<?php\n/** doc */\n$a = 1;", "1", "$a"},
		{"after a semicolon", "<?php $a = 1; $b = 2;", "2", "$b"},
		{"after a close tag and inline HTML", "<?php $a = 1; ?><p>x</p><?php $b = 2;", "2", "$b"},
		{"after an opening brace", "<?php function f() { return g(1); }", "1", "return"},
		{"after a closing brace", "<?php if ($x) {} $b = 2;", "2", "$b"},
		{"skipping a closure passed earlier in the chain", "<?php $r = f(function () { return 1; })->g(2);", "2", "$r"},
		{"inside unclosed parentheses", "<?php $r = f(g(1), 2", "2", "$r"},
		{"a snippet without an open tag", "$a = [1, (2)];", "2", "$a"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tokens := New().Parse(tc.source).Tokens
			from := -1
			for i, tok := range tokens {
				if tok.Value == tc.from {
					from = i
				}
			}
			if from < 0 {
				t.Fatalf("no token %q", tc.from)
			}
			if got := StatementStart(tokens, from); got < 0 || tokens[got].Value != tc.want {
				t.Errorf("StatementStart = %d, want the index of %q", got, tc.want)
			}
		})
	}
}
