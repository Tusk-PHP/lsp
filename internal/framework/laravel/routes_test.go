package laravel

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteIndexScanWorkspaceFindsNamedRoutes(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "laravel")
	idx := NewRouteIndex(root)

	if err := idx.ScanWorkspace(); err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}

	want := []string{"appearance.edit", "dashboard", "home", "profile.edit", "security.edit"}
	names := idx.Names()
	for _, name := range want {
		found := false
		for _, got := range names {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected route %q in %v", name, names)
		}
	}

	security := idx.Find("security.edit")
	if security == nil {
		t.Fatal("expected definition for security.edit")
	}
	if !strings.Contains(security.URI, "/routes/settings.php") {
		t.Fatalf("expected settings route URI, got %s", security.URI)
	}
	if security.Range.Start.Line != 23 {
		t.Fatalf("expected security.edit on line 23, got %d", security.Range.Start.Line)
	}
}

func TestRouteIndexReplacesEntriesOnReindex(t *testing.T) {
	idx := NewRouteIndex("/tmp/project")
	uri := "file:///tmp/project/routes/web.php"

	idx.IndexFile(uri, "<?php\nRoute::get('/')->name('home');\n")
	if got := idx.Find("home"); got == nil {
		t.Fatal("expected home after initial index")
	}

	idx.IndexFile(uri, "<?php\nRoute::get('/')->name('dashboard');\n")
	if got := idx.Find("home"); got != nil {
		t.Fatal("expected home to be removed after reindex")
	}
	if got := idx.Find("dashboard"); got == nil {
		t.Fatal("expected dashboard after reindex")
	}
}

func TestExtractRouteNameArgContext(t *testing.T) {
	tests := []struct {
		name      string
		trimmed   string
		wantOK    bool
		wantPart  string
		wantQuote string
	}{
		{"route helper", "route('da", true, "da", "'"},
		{"blade binding", ":href=\"route('pro", true, "pro", "'"},
		{"routeIs", "request()->routeIs('sec", true, "sec", "'"},
		{"to_route helper", "to_route(\"ho", true, "ho", "\""},
		{"past closing paren", "route('home')", false, "", ""},
		{"not a route call", "config('app", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, quote, ok := ExtractRouteNameArgContext(tt.trimmed)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if part != tt.wantPart {
				t.Fatalf("part = %q, want %q", part, tt.wantPart)
			}
			if quote != tt.wantQuote {
				t.Fatalf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestFindRouteNameReference(t *testing.T) {
	line := `<a href="{{ route('dashboard') }}" :current="request()->routeIs('profile.edit')">`

	if name, ok := FindRouteNameReference(line, 20); !ok || name != "dashboard" {
		t.Fatalf("dashboard reference = (%q, %v)", name, ok)
	}
	if name, ok := FindRouteNameReference(line, 67); !ok || name != "profile.edit" {
		t.Fatalf("profile.edit reference = (%q, %v)", name, ok)
	}
}

func TestRouteIndexCapturesControllerMetadata(t *testing.T) {
	idx := NewRouteIndex("/tmp/project")
	uri := "file:///tmp/project/routes/web.php"
	source := `<?php

use App\Http\Controllers\HomeController;
use Illuminate\Support\Facades\Route;

Route::get('/', [HomeController::class, 'index'])->name('home');
Route::post('/invokable', HomeController::class)->name('home.invoke');
Route::view('/welcome', 'welcome')->name('welcome');
`

	idx.IndexFile(uri, source)

	home := idx.Find("home")
	if home == nil {
		t.Fatal("expected home route")
	}
	if home.Path != "/" {
		t.Fatalf("expected route path '/', got %q", home.Path)
	}
	if home.TargetKind != "controller" {
		t.Fatalf("expected controller target, got %q", home.TargetKind)
	}
	if home.HandlerFQN != `App\Http\Controllers\HomeController` {
		t.Fatalf("expected controller FQN, got %q", home.HandlerFQN)
	}
	if home.HandlerMethod != "index" {
		t.Fatalf("expected handler method index, got %q", home.HandlerMethod)
	}

	invoke := idx.Find("home.invoke")
	if invoke == nil {
		t.Fatal("expected home.invoke route")
	}
	if invoke.HandlerMethod != "__invoke" {
		t.Fatalf("expected __invoke handler, got %q", invoke.HandlerMethod)
	}

	welcome := idx.Find("welcome")
	if welcome == nil {
		t.Fatal("expected welcome route")
	}
	if welcome.TargetKind != "view" || welcome.Target != "welcome" {
		t.Fatalf("expected view target welcome, got kind=%q target=%q", welcome.TargetKind, welcome.Target)
	}
}

func TestParseRouteDefinitionsReadsTheLexer(t *testing.T) {
	source := `<?php
namespace App;

use App\Http\Controllers\HomeController;

\Route::get('/a', [HomeController::class, 'index',])->name('a');
Illuminate\Support\Facades\Route::get('/b', 'HomeController@show')->name("b");
Route::any('/c', fn () => 1)->middleware(['x'])->name('c') ?>
<?php Route::livewire('/d', 'counter')->name('d');
Route::redirect('/e', '/f')->name('e');
Route::get('/g', "Home{$x}@show")->name('g');
Route::get('/h', 'closure')->name("h.$x");
Route::get('/i')->name('i');
Route::get('/j', Controllers\Some::class)->name('j');
Route::get('/k', [HomeController::class])->name('k');
Route::get('/l', [HomeController::class, 'm', 'x'])->name('l');
Route::get('/m', [HomeController::class, ''])->name('m');
Route::get('/n', [HomeController::class, 'x'] + [])->name('n');
Route::get('/o', $action)->name('');
Route::nope('/p')->name('p');
Route::get;
Route::match(['get', 'post'], '/q')->name('q');
Route::view('/v')->name('v');
Route::get('/z'
`
	routes := map[string]RouteName{}
	for _, r := range parseRouteDefinitions("file:///tmp/project/routes/web.php", source) {
		routes[r.Name] = r
	}
	for name, want := range map[string][2]string{
		"a": {"controller", `App\Http\Controllers\HomeController@index`},
		"b": {"controller", `App\Http\Controllers\HomeController@show`},
		"c": {"route", "fn()=>1"},
		"d": {"livewire", "counter"},
		"e": {"redirect", "/f"},
		"v": {"view", ""},
		"g": {"route", `"Home{$x}@show"`},
		"i": {"route", ""},
		"j": {"controller", `Controllers\Some@__invoke`},
		"k": {"route", "[HomeController::class]"},
		"l": {"route", "[HomeController::class,'m','x']"},
		"m": {"route", "[HomeController::class,'']"},
		"n": {"route", "[HomeController::class,'x']+[]"},
	} {
		r, ok := routes[name]
		if !ok {
			t.Errorf("route %q missing from %v", name, routes)
			continue
		}
		if r.TargetKind != want[0] || r.Target != want[1] {
			t.Errorf("route %q: target %s %q, want %s %q", name, r.TargetKind, r.Target, want[0], want[1])
		}
	}
	if _, ok := routes["q"]; !ok {
		t.Errorf("Route::match is not indexed")
	}
	for _, name := range []string{"h.$x", "", "p"} {
		if _, ok := routes[name]; ok {
			t.Errorf("route %q should not be indexed", name)
		}
	}
	if b := routes["b"]; b.Range.Start.Line != 6 || b.Range.Start.Character != len(`Illuminate\Support\Facades\Route::get('/b', 'HomeController@show')->name("`) {
		t.Errorf("route b range = %v", b.Range)
	}
}

func TestParseRouteDefinitionsReadsEachVerbsPathAndAction(t *testing.T) {
	source := `<?php
use App\Http\Controllers\ContactController;

Route::match(['get', 'post'], '/contact', [ContactController::class, 'show'])->name('contact');
Route::match(['put'], '/contact/{id}', 'ContactController@update')->name('contact.update');
Route::match(['get'])->name('contact.none');
Route::any('/any', [ContactController::class, 'any'])->name('any');
Route::get('/get', [ContactController::class, 'index'])->name('get');
Route::post('/post', ContactController::class)->name('post');
Route::put('/put', [ContactController::class, 'put'])->name('put');
Route::patch('/patch', [ContactController::class, 'patch'])->name('patch');
Route::delete('/delete', [ContactController::class, 'delete'])->name('delete');
Route::options('/options', [ContactController::class, 'options'])->name('options');
`
	routes := map[string]RouteName{}
	for _, r := range parseRouteDefinitions("file:///tmp/project/routes/web.php", source) {
		routes[r.Name] = r
	}
	const controller = `App\Http\Controllers\ContactController`
	for name, want := range map[string][3]string{
		"contact":        {"/contact", "controller", controller + "@show"},
		"contact.update": {"/contact/{id}", "controller", controller + "@update"},
		"contact.none":   {"", "route", ""},
		"any":            {"/any", "controller", controller + "@any"},
		"get":            {"/get", "controller", controller + "@index"},
		"post":           {"/post", "controller", controller + "@__invoke"},
		"put":            {"/put", "controller", controller + "@put"},
		"patch":          {"/patch", "controller", controller + "@patch"},
		"delete":         {"/delete", "controller", controller + "@delete"},
		"options":        {"/options", "controller", controller + "@options"},
	} {
		r, ok := routes[name]
		if !ok {
			t.Errorf("route %q missing from %v", name, routes)
			continue
		}
		if r.Path != want[0] || r.TargetKind != want[1] || r.Target != want[2] {
			t.Errorf("route %q: path %q target %s %q, want path %q target %s %q", name, r.Path, r.TargetKind, r.Target, want[0], want[1], want[2])
		}
	}
}
