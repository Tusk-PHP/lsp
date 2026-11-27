package inlayhint

import (
	"os"
	"path/filepath"
	"testing"
)

func BenchmarkInlayHintsEloquentBuilder(b *testing.B) {
	path := filepath.Join(projectRoot(), "..", "laravel", "vendor", "laravel", "framework", "src", "Illuminate", "Database", "Eloquent", "Builder.php")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Skipf("testdata/laravel/vendor is missing (run `composer install --working-dir testdata/laravel`): %v", err)
	}
	benchmarkInlayHints(b, "file:///vendor/Builder.php", string(data))
}

func BenchmarkInlayHintsHintsFixture(b *testing.B) {
	path := filepath.Join(localTestdata(), "hints.php")
	data, err := os.ReadFile(path)
	if err != nil {
		b.Fatalf("read %s: %v", path, err)
	}
	benchmarkInlayHints(b, "file:///app/hints.php", string(data))
}

func benchmarkInlayHints(b *testing.B, uri, source string) {
	p := setupProvider(b, uri, source)
	cfg := allCfg()
	cfg.SuppressSingleParam = false
	cfg.SuppressNameMatch = false
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		p.GetInlayHints(uri, source, cfg)
	}
}
