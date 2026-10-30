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
