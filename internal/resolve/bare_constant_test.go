package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestABareNameInValuePositionIsAConstantNotAClass(t *testing.T) {
	src := `<?php
namespace App;
const LIMIT = 3;
class User {
    public static function make(): static { return new static(); }
}
function run(): void {
    echo 1;
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///constants.php", src)
	r := NewResolver(idx)
	lines := SplitLines(src)
	ctx := ExprContext{Lines: lines, Pos: textpos.Position{Line: 7, Character: 4}, File: parser.ParseFile(src)}
	for _, c := range []struct{ expr, want string }{
		{"PHP_EOL", ""},
		{`\E_ALL`, ""},
		{"LIMIT", ""},
		{"User", ""},
		{"PHP_EOL[0]", ""},
		{"[PHP_EOL, PHP_EOL]", "array<int, mixed>"},
		{"fn() => PHP_EOL", "Closure()"},
		{"User::make()", `App\User`},
		{"User :: make()", `App\User`},
		{"User::class", `class-string<App\User>`},
	} {
		if got := r.ExpressionType(c.expr, ctx).FullString(); got != c.want {
			t.Errorf("ExpressionType(%q) = %q, want %q", c.expr, got, c.want)
		}
	}
}
