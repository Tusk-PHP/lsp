package completion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// benchLaravelProvider indexes the Laravel fixture's real Eloquent and
// collection sources, whose PHPDoc generics completion has to follow.
func benchLaravelProvider(b *testing.B) *Provider {
	b.Helper()
	root := filepath.Join("..", "..", "testdata", "laravel")
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for _, dir := range []string{
		"vendor/laravel/framework/src/Illuminate/Database/Eloquent",
		"vendor/laravel/framework/src/Illuminate/Collections",
	} {
		_ = filepath.Walk(filepath.Join(root, dir), func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || filepath.Ext(path) != ".php" {
				return nil
			}
			data, err := os.ReadFile(path)
			if err == nil {
				rel, _ := filepath.Rel(root, path)
				idx.IndexFileWithSource("file:///"+rel, string(data), symbols.SourceVendor)
			}
			return nil
		})
	}
	for _, rel := range []string{"app/Models/Category.php", "app/Models/Product.php", "app/Models/User.php"} {
		if data, err := os.ReadFile(filepath.Join(root, rel)); err == nil {
			idx.IndexFileWithSource("file:///"+rel, string(data), symbols.SourceProject)
		}
	}
	models.AnalyzeEloquentModels(idx, root)
	return NewProvider(idx, container.NewContainerAnalyzer(idx, root, "laravel"), "laravel")
}

// BenchmarkCompleteGenericChain completes after a chain whose every step is
// typed by PHPDoc generics: Builder<Category>, Collection<int, Category>, a
// closure typed by the collection, and the value it maps to.
func BenchmarkCompleteGenericChain(b *testing.B) {
	p := benchLaravelProvider(b)
	source := "<?php\nuse App\\Models\\Category;\n\nfunction run(): void {\n    Category::query()->where('a', 1)->get()->map(fn ($c) => $c)->first()->\n}\n"
	line := 4
	pos := protocol.Position{Line: line, Character: len(strings.Split(source, "\n")[line])}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.GetCompletions("file:///s.php", source, pos)
	}
}

// BenchmarkResolveGenericVariable types a variable assigned from a generic
// chain and read further down, as hover and completion do.
func BenchmarkResolveGenericVariable(b *testing.B) {
	p := benchLaravelProvider(b)
	source := "<?php\nuse App\\Models\\Category;\n\nfunction run(): void {\n    $cats = Category::query()->get();\n    foreach ($cats as $cat) {\n        $cat->\n    }\n}\n"
	pos := protocol.Position{Line: 6, Character: 14}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.GetCompletions("file:///s.php", source, pos)
	}
}

// BenchmarkTypeGenericChain types a generic chain without building
// completion items, so it measures resolution alone.
func BenchmarkTypeGenericChain(b *testing.B) {
	p := benchLaravelProvider(b)
	source := "<?php\nuse App\\Models\\Category;\n\nfunction run(): void {\n    $cats = Category::query()->get();\n    foreach ($cats as $cat) {\n        $x = 1;\n    }\n}\n"
	file := parser.ParseFile(source)
	pos := protocol.Position{Line: 6, Character: 8}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.ResolveExpressionTypeTyped("$cat", source, pos, file)
		p.ResolveExpressionTypeTyped("Category::query()->where('a', 1)->get()->first()", source, pos, file)
	}
}

func TestBenchGenericChainsResolve(t *testing.T) {
	if testing.Short() {
		t.Skip()
	}
	p := setupLaravelE2E(t)
	source := "<?php\nuse App\\Models\\Category;\n\nfunction run(): void {\n    $cats = Category::query()->get();\n    foreach ($cats as $cat) {\n        $cat->\n    }\n}\n"
	t.Logf("items: %d", len(p.GetCompletions("file:///s.php", source, protocol.Position{Line: 6, Character: 14})))
	file := parser.ParseFile(source)
	t.Logf("$cat: %s", p.ResolveExpressionTypeTyped("$cat", source, protocol.Position{Line: 6, Character: 8}, file).String())
}
