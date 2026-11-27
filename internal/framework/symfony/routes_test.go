package symfony

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestDiscoverRoutesFromAttributes(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///src/Controller/ProductController.php", `<?php
namespace App\Controller;

use Symfony\Component\Routing\Attribute\Route;

#[Route('/products', name: 'product_')]
class ProductController
{
    #[Route('', name: 'index')]
    public function index(): void {}

    #[Route('/{id}', name: 'show')]
    public function show(): void {}
}
`, symbols.SourceProject)

	routes := DiscoverRoutes(idx)
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}
	if routes[0].Name != "product_index" {
		t.Fatalf("first route = %q, want %q", routes[0].Name, "product_index")
	}
	if routes[0].Path != "/products" {
		t.Fatalf("first route path = %q, want %q", routes[0].Path, "/products")
	}
	if routes[1].Name != "product_show" {
		t.Fatalf("second route = %q, want %q", routes[1].Name, "product_show")
	}
	if routes[1].Path != "/products/{id}" {
		t.Fatalf("second route path = %q, want %q", routes[1].Path, "/products/{id}")
	}
}

func TestExtractRouteNameContext(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		wantOk    bool
		wantPart  string
		wantQuote string
	}{
		{name: "generateUrl single quote", input: "$this->generateUrl('product_", wantOk: true, wantPart: "product_", wantQuote: "'"},
		{name: "generate no quote yet", input: "$router->generate(prod", wantOk: true, wantPart: "prod", wantQuote: ""},
		{name: "second arg should not match", input: "$this->redirectToRoute('product_show', ", wantOk: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, quote, ok := ExtractRouteNameContext(tt.input)
			if ok != tt.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOk)
			}
			if part != tt.wantPart {
				t.Fatalf("partial = %q, want %q", part, tt.wantPart)
			}
			if quote != tt.wantQuote {
				t.Fatalf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestRouteNameAtPosition(t *testing.T) {
	source := `<?php
class ProductController
{
    public function show(): void
    {
        $url = $this->generateUrl('product_show');
    }
}
`

	lineNo := -1
	col := -1
	for i, line := range strings.Split(source, "\n") {
		col = strings.Index(line, "product_show")
		if col >= 0 {
			lineNo = i
			break
		}
	}
	if lineNo < 0 {
		t.Fatal("failed to find route usage")
	}

	name, rng, ok := RouteNameAtPosition(source, textpos.Position{Line: lineNo, Character: col + 3})
	if !ok {
		t.Fatal("expected route name context")
	}
	if name != "product_show" {
		t.Fatalf("name = %q, want %q", name, "product_show")
	}
	if rng.Start.Line != lineNo || rng.End.Line != lineNo {
		t.Fatalf("range = %#v", rng)
	}
}

func TestDiscoverRoutesFromYAMLConfigImports(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "composer.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "config", "routes.yaml"), `product_routes:
  resource: ../src/Controller/
  type: attribute
  prefix: /api

catalog:
  resource: routes/catalog.yaml
  prefix: /shop
  name_prefix: shop_
`)
	mustWriteFile(t, filepath.Join(root, "config", "routes", "catalog.yaml"), `home:
  path: /
`)

	controllerPath := filepath.Join(root, "src", "Controller", "ProductController.php")
	controllerSource := `<?php
namespace App\Controller;

use Symfony\Component\Routing\Attribute\Route;

#[Route('/products', name: 'product_')]
class ProductController
{
    #[Route('', name: 'index')]
    public function index(): void {}

    #[Route('/{id}', name: 'show')]
    public function show(): void {}
}
`
	mustWriteFile(t, controllerPath, controllerSource)

	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file://"+filepath.ToSlash(controllerPath), controllerSource, symbols.SourceProject)

	routes := DiscoverRoutes(idx)
	if len(routes) != 3 {
		t.Fatalf("expected 3 routes, got %d", len(routes))
	}

	got := map[string]string{}
	for _, route := range routes {
		got[route.Name] = route.Path
	}

	if got["product_index"] != "/api/products" {
		t.Fatalf("product_index path = %q, want %q", got["product_index"], "/api/products")
	}
	if got["product_show"] != "/api/products/{id}" {
		t.Fatalf("product_show path = %q, want %q", got["product_show"], "/api/products/{id}")
	}
	if got["shop_home"] != "/shop/" {
		t.Fatalf("shop_home path = %q, want %q", got["shop_home"], "/shop/")
	}
}

func TestDiscoverRoutesFromXMLConfigImports(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "composer.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "config", "routes.xml"), `<?xml version="1.0" encoding="UTF-8"?>
<routes>
    <import resource="../src/Controller/" type="attribute" prefix="/xml" />
    <route id="homepage" path="/" />
</routes>
`)

	controllerPath := filepath.Join(root, "src", "Controller", "ProductController.php")
	controllerSource := `<?php
namespace App\Controller;

use Symfony\Component\Routing\Attribute\Route;

#[Route('/products', name: 'product_')]
class ProductController
{
    #[Route('', name: 'index')]
    public function index(): void {}
}
`
	mustWriteFile(t, controllerPath, controllerSource)

	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file://"+filepath.ToSlash(controllerPath), controllerSource, symbols.SourceProject)

	routes := DiscoverRoutes(idx)
	if len(routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(routes))
	}

	got := map[string]string{}
	for _, route := range routes {
		got[route.Name] = route.Path
	}

	if got["homepage"] != "/" {
		t.Fatalf("homepage path = %q, want %q", got["homepage"], "/")
	}
	if got["product_index"] != "/xml/products" {
		t.Fatalf("product_index path = %q, want %q", got["product_index"], "/xml/products")
	}
}

func mustWriteFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestDiscoverRoutesFromAControllerFileImportedInYAML(t *testing.T) {
	root := t.TempDir()
	mustWriteFile(t, filepath.Join(root, "composer.json"), "{}\n")
	mustWriteFile(t, filepath.Join(root, "config", "routes.yaml"), `admin:
  resource: ../src/Admin/DashboardController.php
  type: attribute
  prefix: /admin
  name_prefix: admin_
`)
	mustWriteFile(t, filepath.Join(root, "src", "Admin", "DashboardController.php"), `<?php
namespace App\Admin;

use Symfony\Component\Routing\Attribute\Route;

class DashboardController
{
    #[Route('/dashboard', name: 'dashboard')]
    public function index(): void {}
}
`)
	kernel := filepath.Join(root, "src", "Kernel.php")
	mustWriteFile(t, kernel, "<?php\nnamespace App;\nclass Kernel {}\n")
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file://"+filepath.ToSlash(kernel), "<?php\nnamespace App;\nclass Kernel {}\n", symbols.SourceProject)

	got := map[string]string{}
	for _, route := range DiscoverRoutes(idx) {
		got[route.Name] = route.Path
	}
	if got["admin_dashboard"] != "/admin/dashboard" {
		t.Errorf("the imported controller file's route is found under the import's prefixes: %v", got)
	}
}

func TestRouteAttributesFromTheSyntaxTree(t *testing.T) {
	src := `<?php
namespace App\Controller;
use Symfony\Component\Routing\Annotation\Route as LegacyRoute;
use Symfony\Component\Routing\Attribute\Route;
use Other\Route as NotSymfony;

#[Route('/blog', name: 'blog_')]
class BlogController
{
    const NAME = 'x';
    #[Route('/{slug}', 'show')]
    public function show() {}
    #[LegacyRoute(
        path: '/list',
        name: 'list',
    )]
    public function list() {}
    #[NotSymfony('/other', name: 'other')]
    public function other() {}
    #[Route('/dynamic', name: self::NAME), Route]
    public function dynamic() {}
}
`
	var got []string
	for _, r := range discoverRoutesInFile("file:///c.php", src, phpparser.OnlyGrammar(token.Latest)) {
		got = append(got, fmt.Sprintf("%s %s %d:%d-%d", r.Name, r.Path, r.NameRange.Start.Line, r.NameRange.Start.Character, r.NameRange.End.Character))
	}
	want := []string{"blog_show /blog/{slug} 10:24-28", "blog_list /blog/list 14:15-19"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s", strings.Join(got, "\n"))
	}
}
