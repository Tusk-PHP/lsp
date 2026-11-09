package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupTemplateProviderCoverage(t *testing.T) (*Provider, *symbols.Index, string, *parser.FileNode) {
	t.Helper()

	source := `<?php
namespace App;

class ShapeCarrier {}

class Arr {
    /**
     * @template TKey
     * @template TValue
     * @param array<TKey, TValue> $array
     * @return TValue|null
     */
    public static function first($array) {}
}

$rows = [
    ['id' => 1],
    ['id' => 2],
];

$selected = Arr::first($rows);
`

	idx := symbols.NewIndex()
	idx.IndexFile("file:///provider.php", source)
	file := parser.ParseFile(source)
	if file == nil {
		t.Fatal("expected parsed file")
	}
	return NewProvider(idx, nil, "none"), idx, source, file
}

func providerLineContaining(t *testing.T, source, needle string) int {
	t.Helper()
	for i, line := range strings.Split(source, "\n") {
		if strings.Contains(line, needle) {
			return i
		}
	}
	t.Fatalf("line containing %q not found", needle)
	return -1
}

func TestProviderGenericCoverageHelpers(t *testing.T) {
	p, idx, source, file := setupTemplateProviderCoverage(t)

	if p.resolver.TypedChainResolver == nil {
		t.Fatal("expected NewProvider to wire TypedChainResolver")
	}

	if got := p.ResolveExpressionTypeTyped("   ", source, protocol.Position{}, file); !got.IsEmpty() {
		t.Fatalf("expected empty expression type, got %#v", got)
	}

	if got := p.ResolveChainTypeTyped(source, "$rows", "->", protocol.Position{}, file); !got.IsEmpty() {
		t.Fatalf("expected empty chain type, got %#v", got)
	}

	member := idx.Lookup("App\\Arr::first")
	if member == nil {
		t.Fatal("expected App\\Arr::first symbol")
	}

	pos := protocol.Position{Line: providerLineContaining(t, source, "$selected =")}

	ctx := resolve.ExprContext{Lines: resolve.SplitLines(source), Pos: pos, File: file}
	call := func(args string) resolve.ResolvedType {
		return p.resolver.MemberTypeForCall(member, file, "", nil, p.resolver.CallSiteFor(args, ctx))
	}

	if rt := call("[1, 2, 3]"); rt.String() != "?int" {
		t.Fatalf("literal array inference got %q", rt.String())
	}

	if rt := call("$rows"); rt.String() != "?array{id: int}" {
		t.Fatalf("variable array inference got %q", rt.String())
	}

	// Nothing passed: the template has nothing to bind it.
	if rt := call(""); rt.BaseFQN() != "mixed" && !rt.IsEmpty() {
		t.Fatalf("expected an unbound template for an empty argument list, got %#v", rt)
	}
}

func TestCompletionDetailFormatsConditionalReturnType(t *testing.T) {
	source := `<?php
class Container {
    /**
     * @template T
     * @param string|class-string<T> $id
     * @return ($id is class-string<T> ? T : object)
     */
    public function get(string $id) {}
}

$container = new Container();
$container->
`

	idx := symbols.NewIndex()
	idx.IndexFile("file:///completion-conditional.php", source)
	p := NewProvider(idx, nil, "none")
	items := p.GetCompletions("file:///completion-conditional.php", source, protocol.Position{
		Line:      providerLineContaining(t, source, "$container->"),
		Character: len("$container->"),
	})

	for _, item := range items {
		if item.Label != "get" {
			continue
		}
		if !strings.Contains(item.Detail, "($id is class-string<T> ? T : object)") {
			t.Fatalf("expected conditional return in detail, got %q", item.Detail)
		}
		return
	}

	t.Fatal("expected get completion item")
}
