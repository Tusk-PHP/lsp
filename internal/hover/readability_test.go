package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const readabilityController = `<?php
namespace App\Http\Controllers;

class CategoryController {
    /**
     * Display a listing of the resource.
     */
    public function index(\App\Http\Requests\IndexCategoriesRequest $request, \OpenSoutheners\LaravelApiable\Http\JsonApiResponse $response): mixed {}

    public function show(\App\Models\Category|null $category): \Illuminate\Http\JsonResponse {}
}
`

func readabilityProvider() *Provider {
	idx := symbols.NewIndex()
	// Indexed first and last so neither lookup order hides a wrong match.
	idx.IndexFile("file:///app/A.php", `<?php
namespace App\Api;

class AController {
    /** Not the method being hovered. */
    public function index(): void {}
}
`)
	idx.IndexFile("file:///app/CategoryController.php", readabilityController)
	idx.IndexFile("file:///app/Z.php", `<?php
namespace App\Api;

class ZController {
    /** Not the method being hovered either. */
    public function index(): void {}
}
`)
	return NewProvider(idx, nil, "laravel")
}

func hoverAt(t *testing.T, p *Provider, source, line, word string) string {
	t.Helper()
	for i, l := range strings.Split(source, "\n") {
		if strings.Contains(l, line) {
			pos := protocol.Position{Line: i, Character: strings.Index(l, word) + 1}
			h := p.GetHover("file:///app/CategoryController.php", source, pos)
			if h == nil {
				t.Fatalf("expected hover on %q", word)
			}
			return h.Contents.Value
		}
	}
	t.Fatalf("line %q not found", line)
	return ""
}

func TestHoverMethodDeclarationShowsOwnClass(t *testing.T) {
	val := hoverAt(t, readabilityProvider(), readabilityController, "function index", "index")
	if !strings.Contains(val, "**App\\Http\\Controllers\\CategoryController::index**") {
		t.Fatalf("expected the hovered class's method, got:\n%s", val)
	}
	if !strings.Contains(val, "Display a listing of the resource.") {
		t.Fatalf("expected the method's own docblock, got:\n%s", val)
	}
}

func TestHoverBareWordIgnoresOtherClassMembers(t *testing.T) {
	source := "<?php\nnamespace App;\n\nindex;\n"
	h := readabilityProvider().GetHover("file:///app/other.php", source, protocol.Position{Line: 3, Character: 1})
	if h != nil {
		t.Fatalf("expected no hover for a bare word, got:\n%s", h.Contents.Value)
	}
}

func TestHoverCodeBlockCanBeHighlighted(t *testing.T) {
	val := hoverAt(t, readabilityProvider(), readabilityController, "function show", "show")
	if !strings.Contains(val, "```php\n<?php\npublic function show(") {
		t.Fatalf("expected a PHP opening tag so editors colour the block, got:\n%s", val)
	}
}

func TestHoverSignatureUsesShortTypes(t *testing.T) {
	val := hoverAt(t, readabilityProvider(), readabilityController, "function show", "show")
	if !strings.Contains(val, "public function show(Category | null $category): JsonResponse") {
		t.Fatalf("expected short type names, got:\n%s", val)
	}
}

func TestHoverLongSignatureWraps(t *testing.T) {
	val := hoverAt(t, readabilityProvider(), readabilityController, "function index", "index")
	want := "public function index(\n    IndexCategoriesRequest $request,\n    JsonApiResponse $response\n): mixed"
	if !strings.Contains(val, want) {
		t.Fatalf("expected one parameter per line, got:\n%s", val)
	}
}

func TestHoverSummaryFollowsSignature(t *testing.T) {
	val := hoverAt(t, readabilityProvider(), readabilityController, "function index", "index")
	if strings.Index(val, "```php") > strings.Index(val, "Display a listing") {
		t.Fatalf("expected the signature before the summary, got:\n%s", val)
	}
}

func TestHoverVariableTypes(t *testing.T) {
	source := `<?php
namespace App;

class Other {
    public function name(): string { return ''; }
}

trait T {
    /** @return string|null */
    protected function maybe() { return null; }
    public function inTrait($untyped) {
        $traitLocal = $this->maybe();
        return [$traitLocal, $this, $untyped];
    }
}

class K {
    protected string $shadowed;
    protected function other(): Other { return new Other; }
    public function previous() {
        $leaked = new Other;
        return $leaked;
    }
    public function next() {
        echo $leaked;
    }
    public function chain() {
        $chained = $this->other()
            ->name();
        $cast = (int) $chained;
        $shadowed = $cast + 1;
        return [$chained, $cast, $shadowed];
    }
    public function closures(Other $param) {
        /**
         * @param  array<string>  $documented
         */
        return function (Other $typed, $untypedInClosure, null|int|string $union = null, ?int $nullable = null, array $documented = []) use ($param) {
            $fromParam = $typed->name();
            $inner = fn (Other $arrow) => [$arrow, $param, $fromParam];
            return [$typed, $untypedInClosure, $union, $nullable, $documented, $inner];
        };
    }
    public function noCapture(Other $outer) {
        return function () {
            return $outer;
        };
    }
}
`
	index := symbols.NewIndex()
	index.IndexFile("file:///Foo.php", source)
	provider := NewProvider(index, nil, "laravel")

	lines := strings.Split(source, "\n")
	hoverAt := func(lineContains, variable string) string {
		t.Helper()
		for i, line := range lines {
			if !strings.Contains(line, lineContains) {
				continue
			}
			col := strings.Index(line, variable)
			if col < 0 {
				t.Fatalf("%s not on line %q", variable, line)
			}
			hover := provider.GetHover("file:///Foo.php", source, protocol.Position{Line: i, Character: col + 1})
			if hover == nil {
				return ""
			}
			return hover.Contents.Value
		}
		t.Fatalf("no line contains %q", lineContains)
		return ""
	}

	cases := []struct{ line, variable, want string }{
		{"return [$traitLocal", "$untyped", "mixed $untyped"},
		{"return [$traitLocal", "$this", "T $this"},
		{"$cast = (int)", "$cast", "int $cast"},
		{"return function (Other $typed", "$typed", "Other $typed"},
		{"return function (Other $typed", "$untypedInClosure", "mixed $untypedInClosure"},
		{"return function (Other $typed", "$union", "null|int|string $union"},
		{"return function (Other $typed", "$nullable", "?int $nullable"},
		{"return function (Other $typed", "$documented", "array<string> $documented"},
		{"$inner = fn", "$arrow", "Other $arrow"},
		{"$inner = fn", "$param", "Other $param"},
		{"$shadowed = $cast", "$shadowed", "mixed $shadowed"},
	}
	for _, c := range cases {
		if got := hoverAt(c.line, c.variable); !strings.Contains(got, c.want) {
			t.Errorf("%s on %q: want %q in %q", c.variable, c.line, c.want, got)
		}
	}

	for _, c := range []struct{ line, variable string }{
		{"echo $leaked", "$leaked"},
		{"return $outer", "$outer"},
	} {
		if got := hoverAt(c.line, c.variable); got != "" {
			t.Errorf("%s on %q: want no hover, got %q", c.variable, c.line, got)
		}
	}
}
