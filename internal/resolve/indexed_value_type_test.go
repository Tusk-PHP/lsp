package resolve

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const indexedValueSource = `<?php
namespace App;
class User {}
class Repo {}
class Box {
    const TYPES = array('a' => 1, 'b' => 2);
    const LIST = [1, 2];
    /** @return key-of<self::TYPES> */
    public function key() {}
    /** @return value-of<self::LIST> */
    public function item() {}
    /** @return ($v is string ? User : Repo) */
    public function pick($v = <<<EOT
        x
        EOT) {}
    /** @return ($v is true ? User : Repo) */
    public function flag($v = TRUE) {}
}
/**
 * @template T
 * @param T $value
 * @return T
 */
function same($value = 0x10) { return $value; }
/**
 * @template T
 * @param T $value
 * @return T
 */
function nothing($value = null) { return $value; }
function run(Box $b) {
    echo $b; // here
}
`

// Default values and constant values are typed from their nodes when the
// file is indexed, so a long array(), a heredoc or a hexadecimal number is
// typed as PHP reads it.
func TestDefaultsAndConstantsAreTypedFromTheirNodes(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///values.php", indexedValueSource)
	r := NewResolver(idx)
	lines := SplitLines(indexedValueSource)
	file := parser.ParseFile(indexedValueSource, token.Latest)
	ctx := ExprContext{Lines: lines, Pos: textpos.Position{Line: markedLine(t, lines, "here"), Character: 8}, File: file}
	for expr, want := range map[string]string{
		"$b->key()":  "string",
		"$b->item()": "int",
		"$b->pick()": `App\User`,
		"$b->flag()": `App\User`,
		"same()":     "int",
		"nothing()":  "null",
		"(int) $b":   "int",
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%q) = %q, want %q", expr, got, want)
		}
	}
}
