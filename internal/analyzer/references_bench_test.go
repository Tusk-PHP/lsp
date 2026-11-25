package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

func BenchmarkFindReferencesLaravel(b *testing.B) {
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		b.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "vendor", "laravel", "framework")); err != nil {
		b.Skip("testdata/laravel/vendor is missing: run `composer install --working-dir testdata/laravel`")
	}
	ws := workspace.New(workspace.Options{RootPath: root, Framework: "laravel"})
	ws.IndexWorkspace()
	ws.IndexComposerDependencies()
	a := NewAnalyzer(ws.Index, nil)
	for _, c := range []struct{ name, file, needle string }{
		{"class", "app/Models/User.php", "class User"},
		{"method", "app/Models/User.php", "function initials"},
		{"common static method", "vendor/laravel/framework/src/Illuminate/Support/Str.php", "function of"},
		{"common method", "vendor/laravel/framework/src/Illuminate/Collections/Collection.php", "public function map"},
	} {
		path := filepath.Join(root, filepath.FromSlash(c.file))
		content, err := os.ReadFile(path)
		if err != nil {
			b.Fatal(err)
		}
		source := string(content)
		uri := "file://" + path
		i := strings.Index(source, c.needle) + len(c.needle) - 1
		pos := protocol.Position{Line: strings.Count(source[:i], "\n"), Character: i - strings.LastIndex(source[:i], "\n") - 1}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if len(a.FindAllReferences(uri, source, pos, nil)) == 0 {
					b.Fatal("the declaration is a reference")
				}
			}
		})
	}
}
