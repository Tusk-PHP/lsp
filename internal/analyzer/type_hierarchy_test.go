package analyzer

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestTypeHierarchy(t *testing.T) {
	shapes := `<?php
namespace App;
interface Shape {}
/** @deprecated */
abstract class Base implements Shape, \Countable {}
class Square extends Base { public function count(): int { return 0; } }
class Circle extends Base {}
`
	use := `<?php
namespace App;
function f(Base $b) { return $b->count(); }
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///shapes.php": shapes, "file:///use.php": use})
	at := func(needle string) protocol.Position {
		i := strings.Index(use, needle)
		return protocol.Position{Line: strings.Count(use[:i], "\n"), Character: i - strings.LastIndex(use[:i], "\n") - 1}
	}
	names := func(items []protocol.TypeHierarchyItem) []string {
		var out []string
		for _, item := range items {
			out = append(out, item.Name)
		}
		slices.Sort(out)
		return out
	}

	prepared := a.PrepareTypeHierarchy("file:///use.php", use, at("Base"))
	if len(prepared) != 1 {
		t.Fatalf("prepare on a class name: got %+v", prepared)
	}
	base := prepared[0]
	if base.Name != "Base" || base.Detail != "App" || base.Kind != protocol.SymbolKindClass || base.URI != "file:///shapes.php" || base.Range.Start.Line != 4 {
		t.Errorf("item: %+v", base)
	}
	if !slices.Equal(base.Tags, []protocol.SymbolTag{protocol.SymbolTagDeprecated}) {
		t.Errorf("a deprecated class is tagged: %v", base.Tags)
	}
	if got := names(a.Supertypes(base)); !slices.Equal(got, []string{"Shape"}) {
		t.Errorf("supertypes: %v (builtin interfaces have no location to open)", got)
	}
	if got := names(a.Subtypes(base)); !slices.Equal(got, []string{"Circle", "Square"}) {
		t.Errorf("subtypes: %v", got)
	}
	if got := a.Subtypes(protocol.TypeHierarchyItem{}); got == nil || len(got) != 0 {
		t.Errorf("an item without data has no subtypes: %#v", got)
	}
	if got := a.PrepareTypeHierarchy("file:///use.php", use, at("count")); got != nil {
		t.Errorf("a method is not a type: %+v", got)
	}
}
