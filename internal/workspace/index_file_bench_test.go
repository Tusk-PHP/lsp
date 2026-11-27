package workspace

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BenchmarkWorkspaceIndexFile measures what an edit costs through
// Bootstrapped.IndexFile, alternating between two versions of a file so
// each one is really re-indexed: a file no model depends on, a model with
// casts, and a cast class those casts are typed from.
func BenchmarkWorkspaceIndexFile(b *testing.B) {
	root := b.TempDir()
	files := map[string]string{
		"app/Casts/MoneyCast.php": "<?php\nnamespace App\\Casts;\nuse Illuminate\\Contracts\\Database\\Eloquent\\CastsAttributes;\nclass MoneyCast implements CastsAttributes\n{\n    public function get($model, string $key, mixed $value, array $attributes): \\App\\Money {}\n}\n",
		"app/Models/Order.php":    "<?php\nnamespace App\\Models;\nuse App\\Casts\\MoneyCast;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Order extends Model\n{\n    protected $casts = ['total' => MoneyCast::class, 'paid_at' => 'datetime', 'options' => 'array'];\n}\n",
		"app/Support/Helper.php":  "<?php\nnamespace App\\Support;\nclass Helper\n{\n    public function run(array $items): array { return $items; }\n}\n",
	}
	for name, src := range files {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			b.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			b.Fatal(err)
		}
	}
	cfg := config.DefaultConfig()
	off := false
	cfg.DatabaseEnabled = &off
	ws := New(Options{RootPath: root, Framework: "laravel", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.Index.IndexFile("file:///vendor/Model.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model {}\n")
	ws.Index.IndexFile("file:///vendor/CastsAttributes.php", "<?php\nnamespace Illuminate\\Contracts\\Database\\Eloquent;\n/** @template TGet\n * @template TSet */\ninterface CastsAttributes {\n    /** @return TGet|null */\n    public function get($model, string $key, mixed $value, array $attributes);\n}\n")
	uri := func(name string) string { return symbols.PathToURI(filepath.Join(root, filepath.FromSlash(name))) }
	for name, src := range files {
		ws.Index.IndexFile(uri(name), src)
	}
	ws.AnalyzeModels()

	for _, c := range []struct{ name, file, from, to string }{
		{"plain", "app/Support/Helper.php", "$items; }", "$items ; }"},
		{"model", "app/Models/Order.php", "'array'", "'json'"},
		{"castclass", "app/Casts/MoneyCast.php", `\App\Money`, `\App\Price`},
	} {
		versions := [2]string{files[c.file], strings.Replace(files[c.file], c.from, c.to, 1)}
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ws.IndexFile(uri(c.file), versions[i%2])
			}
		})
	}
}
