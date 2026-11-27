package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"testing"
)

// PHP 8.6 attaches a doc comment to a parameter, written before it or after
// it before the comma.
func TestParameterDocComments(t *testing.T) {
	file := ParseFile(`<?php
function f(
    /** Leading. */
    string $a,
    string $b = 'x' /** Trailing with a default. */,
    int $c /** Trailing. */,
    $d,
    /** Last. */ $e
) {}
`, token.Latest)
	if len(file.Functions) != 1 {
		t.Fatalf("functions: %d", len(file.Functions))
	}
	want := map[string]string{"$a": "/** Leading. */", "$b": "/** Trailing with a default. */", "$c": "/** Trailing. */", "$d": "", "$e": "/** Last. */"}
	params := file.Functions[0].Params
	if len(params) != len(want) {
		t.Fatalf("params: %+v", params)
	}
	for _, p := range params {
		if p.DocComment != want[p.Name] {
			t.Errorf("%s: doc %q, want %q", p.Name, p.DocComment, want[p.Name])
		}
		if p.Name == "$b" && p.DefaultValue != "'x'" {
			t.Errorf("$b default %q", p.DefaultValue)
		}
	}
}
