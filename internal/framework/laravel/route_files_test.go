package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

const routeFilesRoot = "/tmp/project"

func routeFileURI(name string) string {
	return "file://" + routeFilesRoot + "/routes/" + name
}

func indexRouteFiles(t *testing.T, files map[string]string) *RouteIndex {
	t.Helper()
	idx := NewRouteIndex(routeFilesRoot)
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	slices.Sort(names)
	for _, name := range names {
		idx.IndexFile(routeFileURI(name), files[name])
	}
	return idx
}

func assertIndexedRoutes(t *testing.T, idx *RouteIndex, want map[string]string) {
	t.Helper()
	got := map[string]string{}
	for _, r := range idx.All() {
		if _, dup := got[r.Name]; dup {
			t.Fatalf("route %q indexed twice: %v", r.Name, idx.All())
		}
		got[r.Name] = r.Path + " " + filepath.Base(r.URI)
	}
	if len(got) != len(want) {
		t.Fatalf("routes = %v, want %v", got, want)
	}
	for name, path := range want {
		if got[name] != path {
			t.Fatalf("route %q = %q, want %q (all: %v)", name, got[name], path, got)
		}
	}
}

const adminRoutes = routeTestHeader + `Route::get('/users', 'index')->name('users');
`

func TestIncludedRouteFilesTakeTheGroupsPrefixes(t *testing.T) {
	tests := []struct {
		name string
		web  string
	}{
		{"group with base_path", `Route::prefix('admin')->name('admin.')->controller(UserController::class)->group(base_path('routes/admin.php'));`},
		{"group array with __DIR__", `Route::group(['prefix' => 'admin', 'as' => 'admin.', 'controller' => UserController::class], __DIR__.'/admin.php');`},
		{"require in a closure", `Route::prefix('admin')->name('admin.')->controller(UserController::class)->group(function () {
    require __DIR__.'/admin.php';
});`},
		{"require_once of a concatenation", `Route::prefix('admin')->name('admin.')->controller(UserController::class)->group(function () {
    require_once __DIR__ . '/' . 'admin.php';
});`},
		{"include of base_path concatenation", `Route::prefix('admin')->name('admin.')->controller(UserController::class)->group(function () {
    include base_path('routes') . '/admin.php';
});`},
		{"include_once through a parent directory", `Route::prefix('admin')->name('admin.')->controller(UserController::class)->group(function () {
    include_once __DIR__.'/../routes/admin.php';
});`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			idx := indexRouteFiles(t, map[string]string{"web.php": routeTestHeader + tt.web, "admin.php": adminRoutes})
			assertIndexedRoutes(t, idx, map[string]string{"admin.users": "/admin/users admin.php"})
			if r := idx.Find("admin.users"); r.HandlerFQN != `App\Http\Controllers\UserController` || r.HandlerMethod != "index" {
				t.Fatalf("admin.users handler = %s@%s", r.HandlerFQN, r.HandlerMethod)
			}
		})
	}
}

func TestIncludedRouteFileIsNotAlsoIndexedOnItsOwn(t *testing.T) {
	web := routeTestHeader + `Route::name('admin.')->group(base_path('routes/admin.php'));`
	want := map[string]string{"admin.users": "/users admin.php"}

	idx := NewRouteIndex(routeFilesRoot)
	idx.IndexFile(routeFileURI("admin.php"), adminRoutes)
	assertIndexedRoutes(t, idx, map[string]string{"users": "/users admin.php"})
	idx.IndexFile(routeFileURI("web.php"), web)
	assertIndexedRoutes(t, idx, want)

	idx = NewRouteIndex(routeFilesRoot)
	idx.IndexFile(routeFileURI("web.php"), web)
	idx.IndexFile(routeFileURI("admin.php"), adminRoutes)
	assertIndexedRoutes(t, idx, want)
}

func TestIncludedRouteFilesFollowEditsOnEitherSide(t *testing.T) {
	idx := indexRouteFiles(t, map[string]string{
		"web.php":   routeTestHeader + `Route::name('admin.')->group(base_path('routes/admin.php'));`,
		"admin.php": adminRoutes,
	})
	assertIndexedRoutes(t, idx, map[string]string{"admin.users": "/users admin.php"})

	idx.IndexFile(routeFileURI("admin.php"), routeTestHeader+`Route::get('/people', 'index')->name('people');`)
	assertIndexedRoutes(t, idx, map[string]string{"admin.people": "/people admin.php"})

	idx.IndexFile(routeFileURI("web.php"), routeTestHeader+`Route::prefix('staff')->name('staff.')->group(base_path('routes/admin.php'));`)
	assertIndexedRoutes(t, idx, map[string]string{"staff.people": "/staff/people admin.php"})

	idx.IndexFile(routeFileURI("web.php"), routeTestHeader+`Route::get('/', 'home')->name('home');`)
	assertIndexedRoutes(t, idx, map[string]string{"home": "/ web.php", "people": "/people admin.php"})

	idx.IndexFile(routeFileURI("web.php"), routeTestHeader+`Route::name('admin.')->group(base_path('routes/admin.php'));`)
	idx.RemoveFile(routeFileURI("admin.php"))
	assertIndexedRoutes(t, idx, map[string]string{})
}

func TestIncludedRouteFileUnderTwoGroups(t *testing.T) {
	idx := indexRouteFiles(t, map[string]string{
		"web.php": routeTestHeader + `Route::name('admin.')->group(base_path('routes/admin.php'));
Route::name('staff.')->prefix('staff')->group(base_path('routes/admin.php'));
require __DIR__.'/settings.php';`,
		"admin.php":    adminRoutes,
		"settings.php": routeTestHeader + `Route::get('/settings', 'edit')->name('settings');`,
	})
	assertIndexedRoutes(t, idx, map[string]string{
		"admin.users": "/users admin.php",
		"staff.users": "/staff/users admin.php",
		"settings":    "/settings settings.php",
	})
}

func TestIncludedRouteFileCycles(t *testing.T) {
	idx := indexRouteFiles(t, map[string]string{
		"a.php": routeTestHeader + `Route::get('/a', 'a')->name('a');
Route::name('b.')->group(__DIR__.'/b.php');`,
		"b.php": routeTestHeader + `Route::get('/b', 'b')->name('b');
Route::name('a.')->group(function () {
    require __DIR__.'/a.php';
});`,
		"self.php": routeTestHeader + `Route::get('/s', 's')->name('s');
Route::name('again.')->group(__DIR__.'/self.php');`,
	})
	assertIndexedRoutes(t, idx, map[string]string{
		"a":   "/a a.php",
		"b.b": "/b b.php",
		"s":   "/s self.php",
	})
}

func TestRouteIndexScanWorkspaceFollowsIncludes(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "routes", "admin"), 0o755); err != nil {
		t.Fatal(err)
	}
	for name, src := range map[string]string{
		"web.php":         routeTestHeader + `Route::prefix('admin')->name('admin.')->group(base_path('routes/admin/users.php'));`,
		"admin/users.php": adminRoutes,
	} {
		if err := os.WriteFile(filepath.Join(root, "routes", name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := NewRouteIndex(root)
	if err := idx.ScanWorkspace(); err != nil {
		t.Fatal(err)
	}
	if got := idx.Names(); !slices.Equal(got, []string{"admin.users"}) {
		t.Fatalf("names = %v", got)
	}
	if r := idx.Find("admin.users"); !strings.HasSuffix(r.URI, "/routes/admin/users.php") || r.Path != "/admin/users" {
		t.Fatalf("admin.users = %+v", r)
	}
}

func TestIncludedRouteFileKeepsOnlyTheReadsInUse(t *testing.T) {
	idx := indexRouteFiles(t, map[string]string{"admin.php": adminRoutes})
	for _, prefix := range []string{"a", "ad", "adm", "admin."} {
		idx.IndexFile(routeFileURI("web.php"), routeTestHeader+`Route::name('`+prefix+`')->group(base_path('routes/admin.php'));`)
	}
	assertIndexedRoutes(t, idx, map[string]string{"admin.users": "/users admin.php"})
	if reads := len(idx.files[routeFilesRoot+"/routes/admin.php"].reads); reads != 1 {
		t.Fatalf("admin.php keeps %d reads, want 1", reads)
	}
}

func TestRouteFilesIncludedByAPathThatCantBeReadStayOnTheirOwn(t *testing.T) {
	for _, path := range []string{
		"5",
		"__FILE__",
		"__DIR__ + '/admin.php'",
		"$dir . '/admin.php'",
		"__DIR__ . $file",
		"app_path('routes/admin.php')",
		"base_path()",
		"base_path(path: 'routes/admin.php')",
		"base_path(...$parts)",
		"base_path('routes', 'admin.php')",
	} {
		t.Run(path, func(t *testing.T) {
			web := routeTestHeader + "Route::prefix('admin')->name('admin.')->group(function () {\n    require " + path + ";\n});\nRoute::name('staff.')->group(" + path + ");\n"
			idx := indexRouteFiles(t, map[string]string{"web.php": web, "admin.php": adminRoutes})
			assertIndexedRoutes(t, idx, map[string]string{"users": "/users admin.php"})
		})
	}
}

func TestRoutesOfOneNameAreOrderedByFileThenPlace(t *testing.T) {
	idx := indexRouteFiles(t, map[string]string{
		"b.php": routeTestHeader + "Route::get('/b', 'b')->name('home');\n",
		"a.php": routeTestHeader + "Route::get('/a2', 'a')->name('home');\nRoute::get('/a1', 'a')->name('home'); Route::get('/a3', 'a')->name('home');\n",
	})
	var got []string
	for _, r := range idx.All() {
		got = append(got, r.Path)
	}
	if want := []string{"/a2", "/a1", "/a3", "/b"}; !slices.Equal(got, want) {
		t.Errorf("routes named home = %v, want %v: by file, then line, then column", got, want)
	}
	if r := idx.Find("home"); r == nil || r.Path != "/a2" {
		t.Errorf("Find(home) = %+v, want the first in that order", r)
	}
	idx.RemoveFile("file:///tmp/project/app/Http/Kernel.php")
	idx.RemoveFile(routeFileURI("a.php"))
	if r := idx.Find("home"); r == nil || r.Path != "/b" {
		t.Errorf("after a.php is removed, and a file outside routes/ is ignored, Find(home) = %+v", r)
	}
}
