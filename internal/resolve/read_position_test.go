package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestAReadInTheIndentationOfAnAssignmentsLaterLineIsAfterIt(t *testing.T) {
	src := `<?php
namespace App;
class User {}
function run(): void {
    $user = new User(
        1
    );
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///indent.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	for _, c := range []struct {
		character int
		want      string
	}{
		{2, `App\User`},
		{9, ""},
	} {
		if got := r.ResolveVariableTypeTyped("$user", lines, textpos.Position{Line: 5, Character: c.character}, file).FullString(); got != c.want {
			t.Errorf("$user at character %d of the argument line = %q, want %q", c.character, got, c.want)
		}
	}
}
