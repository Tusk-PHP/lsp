package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// benchmarkSearchAfterEdit indexes the Laravel fixture, then times what a
// keystroke costs in an editor: the edited file is re-indexed and the next
// completion or workspace-symbol request searches names by prefix.
func benchmarkSearchAfterEdit(b *testing.B, edit func(i int) string) {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "laravel")
	if _, err := os.Stat(filepath.Join(root, "vendor", "autoload.php")); err != nil {
		b.Skip("testdata/laravel/vendor is missing: run composer install there")
	}
	cfg := config.DefaultConfig()
	cfg.DatabaseEnabled = boolPtr(false)
	ws, err := Build(context.Background(), Options{RootPath: root, Framework: "laravel", Config: cfg, BuiltinProfile: symbols.DefaultBuiltinProfile()})
	if err != nil {
		b.Fatalf("Build() error = %v", err)
	}
	uri := "file://" + filepath.Join(root, "app", "Http", "Controllers", "CategoryController.php")
	i := 0
	for b.Loop() {
		ws.Index.IndexFile(uri, edit(i))
		if len(ws.Index.SearchByPrefix("Coll")) == 0 {
			b.Fatal("expected names starting with Coll")
		}
		if syms, _ := ws.Index.SearchByFQNPrefix(`App\Http\Controllers\`); len(syms) == 0 {
			b.Fatal(`expected classes under App\Http\Controllers`)
		}
		i++
	}
}

// BenchmarkSearchAfterBodyEdit edits a method body: no name changes.
func BenchmarkSearchAfterBodyEdit(b *testing.B) {
	benchmarkSearchAfterEdit(b, func(i int) string {
		return fmt.Sprintf("<?php\nnamespace App\\Http\\Controllers;\n\nclass CategoryController\n{\n    public function index()\n    {\n        return %d;\n    }\n}\n", i)
	})
}

// BenchmarkSearchAfterNewName types a new method name on every edit.
func BenchmarkSearchAfterNewName(b *testing.B) {
	benchmarkSearchAfterEdit(b, func(i int) string {
		return fmt.Sprintf("<?php\nnamespace App\\Http\\Controllers;\n\nclass CategoryController\n{\n    public function index%d()\n    {\n        return 1;\n    }\n}\n", i)
	})
}
