package workspace

import (
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BenchmarkReindexModelFile measures what a didChange of a model file costs:
// re-indexing the file alone, and with UpdateFile, which re-runs model
// analysis for it. The non-model case is what every other file pays.
func BenchmarkReindexModelFile(b *testing.B) {
	root := b.TempDir()
	migration := `<?php
Schema::create('posts', function (Blueprint $table) {
    $table->id();
    $table->string('title');
    $table->text('body');
    $table->foreignId('user_id');
    $table->timestamps();
});
`
	if err := os.MkdirAll(filepath.Join(root, "database", "migrations"), 0o755); err != nil {
		b.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "database", "migrations", "2024_01_01_000000_create_posts_table.php"), []byte(migration), 0o644); err != nil {
		b.Fatal(err)
	}
	model := `<?php
namespace App\Models;

use Illuminate\Database\Eloquent\Model;
use Illuminate\Database\Eloquent\Relations\BelongsTo;
use Illuminate\Database\Eloquent\Relations\HasMany;

class Post extends Model
{
    protected $fillable = ['title', 'body'];

    public function user(): BelongsTo { return $this->belongsTo(User::class); }
    public function comments(): HasMany { return $this->hasMany(Comment::class); }
    public function getExcerptAttribute(): string { return substr($this->body, 0, 100); }
}
`
	plain := `<?php
namespace App\Support;

class Helper
{
    public function run(array $items): array { return array_map(fn ($i) => $i * 2, $items); }
}
`
	cfg := config.DefaultConfig()
	off := false
	cfg.DatabaseEnabled = &off
	ws := New(Options{RootPath: root, Framework: "laravel", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.Index.IndexFile("file:///vendor/Model.php", "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model {}\n")
	const modelURI, plainURI = "file:///app/Models/Post.php", "file:///app/Support/Helper.php"
	ws.Index.IndexFile(modelURI, model)
	ws.Index.IndexFile(plainURI, plain)
	ws.AnalyzeModels()

	for _, c := range []struct {
		name, uri, src string
		update         bool
	}{
		{"model/reindex", modelURI, model, false},
		{"model/reindex+UpdateFile", modelURI, model, true},
		{"plain/reindex", plainURI, plain, false},
		{"plain/reindex+UpdateFile", plainURI, plain, true},
	} {
		b.Run(c.name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				ws.Index.IndexFile(c.uri, c.src)
				if c.update {
					ws.UpdateFile(c.uri, c.src)
				}
			}
		})
	}
}
