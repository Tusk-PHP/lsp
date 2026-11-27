package container

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func BenchmarkSave(b *testing.B) {
	for _, project := range []struct{ name, framework, file string }{
		{"laravel/unrelated", "laravel", "app/Models/User.php"},
		{"laravel/provider", "laravel", "app/Providers/AppServiceProvider.php"},
		{"symfony/unrelated", "symfony", "translations/security.en.php"},
		{"symfony/service", "symfony", "src/Service/NotificationService.php"},
	} {
		b.Run(project.name, func(b *testing.B) {
			root, err := filepath.Abs(filepath.Join("..", "..", "testdata", project.framework))
			if err != nil {
				b.Fatal(err)
			}
			ca := NewContainerAnalyzer(symbols.NewIndex(), root, project.framework)
			ca.Analyze()
			b.ReportAllocs()
			path := filepath.Join(root, filepath.FromSlash(project.file))
			for b.Loop() {
				if err := ca.Update(path); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func BenchmarkAnalyze(b *testing.B) {
	for _, framework := range []string{"laravel", "symfony"} {
		b.Run(framework, func(b *testing.B) {
			root, err := filepath.Abs(filepath.Join("..", "..", "testdata", framework))
			if err != nil {
				b.Fatal(err)
			}
			ca := NewContainerAnalyzer(symbols.NewIndex(), root, framework)
			b.ReportAllocs()
			for b.Loop() {
				ca.Analyze()
			}
		})
	}
}
