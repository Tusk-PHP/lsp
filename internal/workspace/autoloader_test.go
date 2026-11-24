package workspace

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		full := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestClassLoaderLoadsFromComposerAndAnalysesProjectModels(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"composer.json": `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"vendor/composer/installed.json": `{"packages": [{"name": "laravel/framework", "install-path": "../laravel/framework",
			"autoload": {"psr-4": {"Illuminate\\": "src/Illuminate/"}}}]}`,
		"vendor/laravel/framework/src/Illuminate/Database/Eloquent/Model.php": "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model { public function hasMany($related) {} }\n",
		"app/Models/Comment.php": "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Comment extends Model {}\n",
		"app/Models/Post.php":    "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Post extends Model {\n    public function comments() { return $this->hasMany(Comment::class); }\n}\n",
	})
	ws := New(Options{RootPath: root, Framework: "laravel"})
	ws.Index.SetClassLoader(ws.ClassLoader())

	post := ws.Index.Lookup(`App\Models\Post`)
	if post == nil || post.Source != symbols.SourceProject {
		t.Fatalf("the project model loads on demand: %+v", post)
	}
	if model := ws.Index.Lookup(`Illuminate\Database\Eloquent\Model`); model == nil || model.Source != symbols.SourceVendor {
		t.Errorf("the vendor parent loads as vendor code: %+v", model)
	}
	if ws.Index.Lookup(`App\Models\Post::comments`) == nil {
		t.Error("a model loaded on demand gets its relations straight away")
	}
	if ws.Index.Lookup(`App\Models\Missing`) != nil {
		t.Error("a class with no file stays unknown")
	}
	var none *Bootstrapped
	if none.ClassLoader() != nil {
		t.Error("no workspace, no loader")
	}
}
