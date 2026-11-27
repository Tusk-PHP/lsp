package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
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

func laravelModelsProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"composer.json": `{"autoload": {"psr-4": {"App\\": "app/", "Cached\\": "storage/cached/", "Outside\\": "../outside/"}}}`,
		"vendor/composer/installed.json": `{"packages": [{"name": "laravel/framework", "install-path": "../laravel/framework",
			"autoload": {"psr-4": {"Illuminate\\": "src/Illuminate/"}}}]}`,
		"vendor/laravel/framework/src/Illuminate/Database/Eloquent/Model.php": "<?php\nnamespace Illuminate\\Database\\Eloquent;\nabstract class Model { public function hasMany($related) {} }\n",
		"app/Models/Comment.php":   "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Comment extends Model {}\n",
		"app/Models/Post.php":      "<?php\nnamespace App\\Models;\nuse Illuminate\\Database\\Eloquent\\Model;\nclass Post extends Model {\n    public function comments() { return $this->hasMany(Comment::class); }\n}\n",
		"storage/cached/Thing.php": "<?php\nnamespace Cached;\nclass Thing {}\n",
	})
	return root
}

func loadingWorkspace(root string, cfg *config.Config) *Bootstrapped {
	ws := New(Options{RootPath: root, Framework: "laravel", Config: cfg})
	ws.Index.SetClassLoader(ws.ClassLoader())
	return ws
}

func TestClassLoaderLoadsFromComposerAndAnalysesProjectModels(t *testing.T) {
	ws := loadingWorkspace(laravelModelsProject(t), nil)

	post := ws.Index.Lookup(`App\Models\Post`)
	if post == nil || post.Source != symbols.SourceProject {
		t.Fatalf("the project model loads on demand: %+v", post)
	}
	if model := ws.Index.Lookup(`Illuminate\Database\Eloquent\Model`); model == nil || model.Source != symbols.SourceVendor {
		t.Errorf("the vendor parent loads as vendor code, though vendor/ is excluded from the workspace walk: %+v", model)
	}
	if relation := ws.Index.Lookup(`App\Models\Post::$comments`); relation == nil || relation.Type != `Illuminate\Database\Eloquent\Collection<int, App\Models\Comment>` {
		t.Errorf("a model loaded on demand gets its relation property straight away: %+v", relation)
	}
	if ws.Index.Lookup(`App\Models\Missing`) != nil {
		t.Error("a class with no file stays unknown")
	}
	var none *Bootstrapped
	if none.ClassLoader() != nil {
		t.Error("no workspace, no loader")
	}
}

func TestRelationsOfALoadedModelSurviveTheWorkspaceWalk(t *testing.T) {
	ws := loadingWorkspace(laravelModelsProject(t), nil)
	if ws.Index.Lookup(`App\Models\Post::$comments`) == nil {
		t.Fatal("the relation property is there after loading the model")
	}
	ws.IndexWorkspace()
	if ws.Index.Lookup(`App\Models\Post::$comments`) == nil {
		t.Error("the walk reaching the loaded model, unchanged, keeps its relation property")
	}
}

func TestClassLoaderUsesTheOnDiskCaseOfTheFile(t *testing.T) {
	root := laravelModelsProject(t)
	ws := loadingWorkspace(root, nil)
	post := ws.Index.LookupFold(`App\models\post`, nil)
	want := "file://" + filepath.Join(root, "app", "Models", "Post.php")
	if post == nil || post.URI != want {
		t.Fatalf("a name in another case loads the file under the URI the workspace walk gives it, %s: %+v", want, post)
	}
	ws.IndexWorkspace()
	declaring := 0
	for _, uri := range ws.Index.GetAllFileURIs() {
		if strings.EqualFold(filepath.Base(uri), "post.php") {
			declaring++
		}
	}
	if declaring != 1 {
		t.Errorf("one file declares the model, not a second URI in another case: %d", declaring)
	}
}

func TestClassLoaderLoadsOnlyWhatTheWorkspaceWalkWouldIndex(t *testing.T) {
	root := laravelModelsProject(t)
	writeFiles(t, filepath.Dir(root), map[string]string{"outside/Far.php": "<?php\nnamespace Outside;\nclass Far {}\n"})
	ws := loadingWorkspace(root, nil)
	if ws.Index.Lookup(`Cached\Thing`) != nil {
		t.Error("a project class in an excluded directory isn't loaded")
	}
	if ws.Index.Lookup(`Outside\Far`) != nil {
		t.Error("a project class outside the workspace isn't loaded")
	}

	ws.IndexWorkspace()
	writeFiles(t, root, map[string]string{"app/Late.php": "<?php\nnamespace App;\nclass Late {}\n"})
	if ws.Index.Lookup(`App\Late`) != nil {
		t.Error("once the walk is done, a project file it didn't index isn't loaded")
	}
}

func TestClassLoaderCountsAgainstMaxIndexFiles(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.MaxIndexFiles = 1
	ws := loadingWorkspace(laravelModelsProject(t), cfg)
	if ws.Index.Lookup(`App\Models\Post`) == nil {
		t.Fatal("the first project file loads")
	}
	if ws.Index.Lookup(`App\Models\Comment`) != nil {
		t.Error("a second project file would go past maxIndexFiles")
	}
	if got := ws.IndexWorkspace().Count; got != 1 {
		t.Errorf("the walk shares the limit with the loader: %d workspace files", got)
	}
	project := 0
	for _, uri := range ws.Index.GetAllFileURIs() {
		if strings.HasPrefix(uri, "file://") && !strings.Contains(uri, "/vendor/") {
			project++
		}
	}
	if project != 1 {
		t.Errorf("the index holds maxIndexFiles workspace files: %d", project)
	}
}
