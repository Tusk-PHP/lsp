package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Go to definition on a local scope called on a query builder chain: the
// builder's receiver type carries the model, where the scope is declared.
func TestDefinitionOfLocalScopesOnABuilderChain(t *testing.T) {
	idx := symbols.NewIndex()
	idx.SetCallForwards(map[string][]string{
		"Illuminate\\Database\\Eloquent\\Model": {"\\Illuminate\\Database\\Eloquent\\Builder<static>"},
	})
	idx.SetScopeConventions(map[string]symbols.ScopeConvention{
		"Illuminate\\Database\\Eloquent\\Builder": {Prefix: "scope", Attribute: "Illuminate\\Database\\Eloquent\\Attributes\\Scope"},
	})
	idx.IndexFile("file:///vendor/Eloquent.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {
    /** @return \Illuminate\Database\Eloquent\Builder<static> */
    public static function query() {}
}
/** @template TModel of Model */
class Builder {
    /** @return TModel|null */
    public function first() {}
}
`)
	idx.IndexFile("file:///app/Category.php", `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Attributes\Scope;
use Illuminate\Database\Eloquent\Builder;
use Illuminate\Database\Eloquent\Model;
class Category extends Model {
    public function scopeActive(Builder $query): void {}
    #[Scope]
    protected function popular(Builder $query): void {}
}
`)
	a := NewAnalyzer(idx, nil)
	// Wired like the LSP and MCP servers do, to the shared typed resolver.
	a.SetTypedChainResolver(func(expr, _ string, _ protocol.Position, file *parser.FileNode) resolve.ResolvedType {
		return a.resolver.ExpressionType(expr, resolve.ExprContext{File: file})
	})
	src := "<?php\nuse App\\Models\\Category;\n$c = Category::query()->active()->popular()->first();\n"
	line := strings.Split(src, "\n")[2]
	for _, name := range []string{"active", "popular"} {
		pos := protocol.Position{Line: 2, Character: strings.Index(line, name) + 2}
		if loc := a.FindDefinition("file:///u.php", src, pos); loc == nil || loc.URI != "file:///app/Category.php" {
			t.Errorf("FindDefinition(%s) = %+v", name, loc)
		}
		if r := a.FindDefinitionWithManual("file:///u.php", src, pos, true, ""); r.Location == nil || r.Location.URI != "file:///app/Category.php" {
			t.Errorf("FindDefinitionWithManual(%s) = %+v", name, r)
		}
	}
	// A name that's neither a builder method nor a scope goes nowhere.
	nothing := "<?php\nuse App\\Models\\Category;\n$c = Category::query()->nothing();\n"
	pos := protocol.Position{Line: 2, Character: strings.Index(strings.Split(nothing, "\n")[2], "nothing") + 2}
	if loc := a.FindDefinition("file:///u.php", nothing, pos); loc != nil {
		t.Errorf("FindDefinition(nothing) = %+v", loc)
	}
	if r := a.FindDefinitionWithManual("file:///u.php", nothing, pos, true, ""); r.Location != nil {
		t.Errorf("FindDefinitionWithManual(nothing) = %+v", r)
	}
}
