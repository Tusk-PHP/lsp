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

// TestAnalyzeModelFileAfterAnalyzeModels checks that re-indexing a model file
// and re-analysing it gives the model back its migration columns, and that
// nothing happens before the workspace's own model analysis has run.
func TestAnalyzeModelFileAfterAnalyzeModels(t *testing.T) {
	root := t.TempDir()
	write := func(rel, content string) string {
		path := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	write("database/migrations/2024_01_01_000000_create_posts_table.php", `<?php
Schema::create('posts', function (Blueprint $table) {
    $table->id();
    $table->string('title');
});
`)
	postSrc := `<?php
namespace App\Models;
use Illuminate\Database\Eloquent\Model;
class Post extends Model {}
`
	postURI := symbols.PathToURI(write("app/Models/Post.php", postSrc))

	cfg := config.DefaultConfig()
	off := false
	cfg.DatabaseEnabled = &off
	ws := New(Options{RootPath: root, Framework: "laravel", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.Index.IndexFile("file:///vendor/Model.php", `<?php
namespace Illuminate\Database\Eloquent;
abstract class Model {}
`)
	ws.Index.IndexFile(postURI, postSrc)
	title := func() bool { return ws.Index.Lookup(`App\Models\Post::$title`) != nil }

	ws.AnalyzeModelFile(postURI, postSrc)
	if title() {
		t.Fatal("AnalyzeModelFile ran before AnalyzeModels")
	}
	ws.AnalyzeModels()
	if !title() {
		t.Fatal("AnalyzeModels gave Post no $title")
	}
	if ws.SchemaIsLive() {
		t.Error("a schema from migrations is not live")
	}
	edited := postSrc + "\n"
	ws.Index.IndexFile(postURI, edited)
	if title() {
		t.Fatal("re-indexing an edit kept $title; this test no longer covers the bug")
	}
	ws.AnalyzeModelFile(postURI, edited)
	if !title() {
		t.Error("AnalyzeModelFile didn't give Post back its $title")
	}

	var nilWS *Bootstrapped
	nilWS.AnalyzeModelFile(postURI, postSrc)
	if nilWS.SchemaIsLive() {
		t.Error("a nil workspace has no live schema")
	}
}
