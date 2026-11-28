package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverDocTypesWrittenOverSeveralLines(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
namespace App;

/**
 * @property array{
 *     theme: string,
 * } $settings
 */
class User {}

/**
 * @return array{
 *     id: int,
 *     name: string,
 * }
 */
function row(): array {}

function run(User $u): void {
    $r = row();
    $u->settings;
}
`
	idx.IndexFile("file:///app/User.php", src)
	p := NewProvider(idx, nil, "none")
	for _, tt := range []struct{ line, word, want string }{
		{"$r = row()", "row", "array{id: int, name: string}"},
		{"$r = row()", "$r", "array{id: int, name: string}"},
		{"$u->settings", "settings", "array{theme: string} $settings"},
	} {
		if got := hoverAt(t, p, src, tt.line, tt.word); !strings.Contains(got, tt.want) {
			t.Errorf("hover on %s = %q, want it to contain %q", tt.word, got, tt.want)
		}
	}
}
