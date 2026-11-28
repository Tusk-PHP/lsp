package laravel

import (
	"slices"
	"strings"
	"testing"
)

const routeTestHeader = `<?php

use App\Http\Controllers\PhotoController;
use App\Http\Controllers\UserController;
use Illuminate\Support\Facades\Route;

`

func readRoutes(t *testing.T, body string) map[string]RouteName {
	t.Helper()
	routes := map[string]RouteName{}
	for _, r := range parseRouteDefinitions("file:///tmp/project/routes/web.php", routeTestHeader+body) {
		if _, dup := routes[r.Name]; dup {
			t.Fatalf("route %q read twice", r.Name)
		}
		routes[r.Name] = r
	}
	return routes
}

func routeNames(routes map[string]RouteName) []string {
	names := make([]string, 0, len(routes))
	for name := range routes {
		names = append(names, name)
	}
	slices.Sort(names)
	return names
}

func assertRouteNames(t *testing.T, routes map[string]RouteName, want ...string) {
	t.Helper()
	slices.Sort(want)
	if got := routeNames(routes); !slices.Equal(got, want) {
		t.Fatalf("route names = %v, want %v", got, want)
	}
}

func assertRoute(t *testing.T, routes map[string]RouteName, name, path, handler string) {
	t.Helper()
	r, ok := routes[name]
	if !ok {
		t.Fatalf("route %q missing from %v", name, routeNames(routes))
	}
	if r.Path != path {
		t.Errorf("route %q path = %q, want %q", name, r.Path, path)
	}
	if got := r.HandlerFQN + "@" + r.HandlerMethod; handler != "" && got != handler {
		t.Errorf("route %q handler = %q, want %q", name, got, handler)
	}
}

func TestRouteGroupsPrefixNames(t *testing.T) {
	tests := []struct {
		name string
		body string
		want map[string]string
	}{
		{
			name: "name group",
			body: `Route::name('admin.')->group(function () {
    Route::get('/users', [UserController::class, 'index'])->name('users.index');
});`,
			want: map[string]string{"admin.users.index": "/users"},
		},
		{
			name: "as and prefix in the group array",
			body: `Route::group(['as' => 'admin.', 'prefix' => 'admin'], function () {
    Route::get('/users', [UserController::class, 'index'])->name('users.index');
});`,
			want: map[string]string{"admin.users.index": "/admin/users"},
		},
		{
			name: "prefix then name then middleware",
			body: `Route::prefix('/admin/')->name('admin.')->middleware(['auth'])->group(function () {
    Route::get('users', [UserController::class, 'index'])->name('users.index');
});`,
			want: map[string]string{"admin.users.index": "/admin/users"},
		},
		{
			name: "nested groups compose",
			body: `Route::name('admin.')->prefix('admin')->group(function () {
    Route::group(['as' => 'users.', 'prefix' => 'users'], function () {
        Route::get('/{user}', [UserController::class, 'show'])->name('show');
    });
    Route::get('/', [UserController::class, 'home'])->name('home');
});
Route::get('/', [UserController::class, 'home'])->name('home');`,
			want: map[string]string{"admin.users.show": "/admin/users/{user}", "admin.home": "/admin", "home": "/"},
		},
		{
			name: "arrow function group",
			body: `Route::name('api.')->group(fn () => Route::get('/ping', [UserController::class, 'ping'])->name('ping'));`,
			want: map[string]string{"api.ping": "/ping"},
		},
		{
			name: "verb on the registrar",
			body: `Route::name('admin.')->prefix('admin')->get('/dash', [UserController::class, 'dash'])->name('dash');`,
			want: map[string]string{"admin.dash": "/admin/dash"},
		},
		{
			name: "chained names append",
			body: `Route::get('/a', [UserController::class, 'a'])->name('a.')->name('b');`,
			want: map[string]string{"a.b": "/a"},
		},
		{
			name: "route inside a statement in a group",
			body: `Route::name('admin.')->group(function () {
    if (app()->isLocal()) {
        Route::get('/debug', [UserController::class, 'debug'])->name('debug');
    }
});`,
			want: map[string]string{"admin.debug": "/debug"},
		},
		{
			name: "action array names the route",
			body: `Route::name('admin.')->group(function () {
    Route::get('/old', ['as' => 'old', 'uses' => 'UserController@old']);
});`,
			want: map[string]string{"admin.old": "/old"},
		},
		{
			name: "unnamed routes in a named group stay out",
			body: `Route::name('admin.')->group(function () {
    Route::get('/x', [UserController::class, 'x']);
});`,
			want: map[string]string{},
		},
		{
			name: "a name prefix that is not a literal hides the names",
			body: `Route::name($area)->group(function () {
    Route::get('/x', [UserController::class, 'x'])->name('x');
    Route::resource('photos', PhotoController::class);
});
Route::name('a.')->name($b)->get('/y', [UserController::class, 'y'])->name('y');`,
			want: map[string]string{},
		},
		{
			name: "a path prefix that is not a literal leaves the path out",
			body: `Route::prefix($area)->group(function () {
    Route::get('/x', [UserController::class, 'x'])->name('x');
});`,
			want: map[string]string{"x": ""},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := readRoutes(t, tt.body)
			want := make([]string, 0, len(tt.want))
			for name := range tt.want {
				want = append(want, name)
			}
			assertRouteNames(t, routes, want...)
			for name, path := range tt.want {
				assertRoute(t, routes, name, path, "")
			}
		})
	}
}

func TestRouteGroupRangePointsAtTheRoutesOwnName(t *testing.T) {
	body := `Route::name('admin.')->group(function () {
    Route::get('/users', [UserController::class, 'index'])->name('users.index');
});`
	r := readRoutes(t, body)["admin.users.index"]
	line := strings.Split(routeTestHeader+body, "\n")[r.Range.Start.Line]
	if got := line[r.Range.Start.Character:r.Range.End.Character]; got != "users.index" {
		t.Fatalf("range covers %q, want users.index", got)
	}
}

func TestRouteGroupControllerAndNamespace(t *testing.T) {
	routes := readRoutes(t, `Route::controller(UserController::class)->prefix('users')->name('users.')->group(function () {
    Route::get('/', 'index')->name('index');
    Route::post('/', [PhotoController::class, 'store'])->name('store');
    Route::get('/legacy', 'Legacy\Thing@run')->name('legacy');
});
Route::group(['namespace' => 'Admin', 'as' => 'admin.'], function () {
    Route::get('/a', 'PanelController@show')->name('panel');
    Route::group(['namespace' => '\Other'], function () {
        Route::get('/b', 'ThingController@show')->name('thing');
    });
});
Route::namespace('App\Http\Controllers')->group(function () {
    Route::controller('UserController')->group(function () {
        Route::get('/c', 'show')->name('c');
    });
});`)
	assertRoute(t, routes, "users.index", "/users", `App\Http\Controllers\UserController@index`)
	assertRoute(t, routes, "users.store", "/users", `App\Http\Controllers\PhotoController@store`)
	assertRoute(t, routes, "users.legacy", "/users/legacy", `Legacy\Thing@run`)
	assertRoute(t, routes, "admin.panel", "/a", `Admin\PanelController@show`)
	assertRoute(t, routes, "admin.thing", "/b", `Other\ThingController@show`)
	assertRoute(t, routes, "c", "/c", `App\Http\Controllers\UserController@show`)
}

func TestResourceRoutes(t *testing.T) {
	const photos = `App\Http\Controllers\PhotoController`
	tests := []struct {
		name string
		body string
		want map[string]string
	}{
		{
			name: "resource",
			body: `Route::resource('photos', PhotoController::class);`,
			want: map[string]string{
				"photos.index": "/photos", "photos.create": "/photos/create", "photos.store": "/photos",
				"photos.show": "", "photos.edit": "", "photos.update": "", "photos.destroy": "",
			},
		},
		{
			name: "api resource",
			body: `Route::apiResource('photos', PhotoController::class);`,
			want: map[string]string{"photos.index": "/photos", "photos.store": "/photos", "photos.show": "", "photos.update": "", "photos.destroy": ""},
		},
		{
			name: "only and except",
			body: `Route::resource('photos', PhotoController::class)->only(['index', 'show']);
Route::resource('users', UserController::class)->except('create', 'edit', 'destroy');
Route::apiResource('tags', PhotoController::class)->only('index');
Route::resource('posts', PhotoController::class, ['except' => ['index', 'create', 'store', 'edit', 'update']]);`,
			want: map[string]string{
				"photos.index": "/photos", "photos.show": "",
				"users.index": "/users", "users.store": "/users", "users.show": "", "users.update": "",
				"tags.index": "/tags",
				"posts.show": "", "posts.destroy": "",
			},
		},
		{
			name: "names",
			body: `Route::resource('photos', PhotoController::class)->only('index', 'show')->names(['index' => 'gallery']);
Route::resource('users', UserController::class)->only('index')->names('people');
Route::resource('tags', PhotoController::class)->only('index', 'show')->name('show', 'tag');
Route::resource('posts', PhotoController::class, ['only' => ['index'], 'as' => 'blog']);`,
			want: map[string]string{
				"gallery": "/photos", "photos.show": "",
				"people.index": "/users",
				"tags.index":   "/tags", "tag": "",
				"blog.posts.index": "/posts",
			},
		},
		{
			name: "nested and shallow",
			body: `Route::resource('photos.comments', PhotoController::class)->only('index', 'show');
Route::resource('posts.notes', PhotoController::class)->shallow()->only('index', 'store', 'show', 'destroy');`,
			want: map[string]string{
				"photos.comments.index": "", "photos.comments.show": "",
				"posts.notes.index": "", "posts.notes.store": "", "notes.show": "", "notes.destroy": "",
			},
		},
		{
			name: "prefixed resource name",
			body: `Route::resource('admin/photos', PhotoController::class)->only('index');`,
			want: map[string]string{"photos.index": "/admin/photos"},
		},
		{
			name: "inside a group",
			body: `Route::prefix('admin')->name('admin.')->group(function () {
    Route::resource('photos', PhotoController::class)->only('index', 'create');
});
Route::middleware('auth')->apiResource('tags', PhotoController::class)->only('index');`,
			want: map[string]string{"admin.photos.index": "/admin/photos", "admin.photos.create": "/admin/photos/create", "tags.index": "/tags"},
		},
		{
			name: "resources and api resources",
			body: `Route::resources(['photos' => PhotoController::class, 'users' => UserController::class], ['only' => ['index']]);
Route::apiResources(['tags' => PhotoController::class]);`,
			want: map[string]string{
				"photos.index": "/photos", "users.index": "/users",
				"tags.index": "/tags", "tags.store": "/tags", "tags.show": "", "tags.update": "", "tags.destroy": "",
			},
		},
		{
			name: "singletons",
			body: `Route::singleton('profile', PhotoController::class);
Route::singleton('avatar', PhotoController::class)->creatable();
Route::singleton('banner', PhotoController::class)->destroyable();
Route::apiSingleton('settings', PhotoController::class);
Route::apiSingleton('token', PhotoController::class)->creatable();
Route::singleton('photos.thumbnail', PhotoController::class)->only('show');`,
			want: map[string]string{
				"profile.show": "/profile", "profile.edit": "/profile/edit", "profile.update": "/profile",
				"avatar.show": "/avatar", "avatar.edit": "/avatar/edit", "avatar.update": "/avatar",
				"avatar.create": "/avatar/create", "avatar.store": "/avatar", "avatar.destroy": "/avatar",
				"banner.show": "/banner", "banner.edit": "/banner/edit", "banner.update": "/banner", "banner.destroy": "/banner",
				"settings.show": "/settings", "settings.update": "/settings",
				"token.show": "/token", "token.update": "/token", "token.store": "/token", "token.destroy": "/token",
				"photos.thumbnail.show": "",
			},
		},
		{
			name: "options that are not literals hide the resource",
			body: `Route::resource('photos', PhotoController::class)->only($actions);
Route::resource('users', UserController::class, $options);
Route::resource($name, PhotoController::class);
Route::resources($map);
Route::resources([$key => PhotoController::class]);
Route::resource('a', PhotoController::class, [$key => 1]);
Route::resource('b', PhotoController::class)->only(['index', $other]);
Route::resource('c', PhotoController::class)->names('x', 'y');
Route::resource('d', PhotoController::class)->name($action, 'x');
Route::resource('e', PhotoController::class)->as('x', 'y');`,
			want: map[string]string{},
		},
		{
			name: "flags given a value",
			body: `Route::resource('posts.notes', PhotoController::class)->shallow(false)->only('show');
Route::singleton('avatar', PhotoController::class)->creatable(true)->only('create');`,
			want: map[string]string{"posts.notes.show": "", "avatar.create": "/avatar/create"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			routes := readRoutes(t, tt.body)
			want := make([]string, 0, len(tt.want))
			for name := range tt.want {
				want = append(want, name)
			}
			assertRouteNames(t, routes, want...)
			for name, path := range tt.want {
				assertRoute(t, routes, name, path, "")
			}
		})
	}

	routes := readRoutes(t, `Route::resource('photos', PhotoController::class)->name('index', 'gallery');`)
	assertRoute(t, routes, "photos.edit", "", photos+"@edit")
	assertRoute(t, routes, "gallery", "/photos", photos+"@index")
	source := routeTestHeader + `Route::resource('photos', PhotoController::class)->name('index', 'gallery');`
	line := strings.Split(source, "\n")[routes["photos.edit"].Range.Start.Line]
	if got := line[routes["photos.edit"].Range.Start.Character:routes["photos.edit"].Range.End.Character]; got != "photos" {
		t.Errorf("photos.edit range covers %q, want the resource name", got)
	}
	if got := line[routes["gallery"].Range.Start.Character:routes["gallery"].Range.End.Character]; got != "gallery" {
		t.Errorf("gallery range covers %q, want its own name", got)
	}
}

func TestRouteIndexFindsResourceAndGroupNames(t *testing.T) {
	idx := NewRouteIndex("/tmp/project")
	uri := "file:///tmp/project/routes/web.php"
	idx.IndexFile(uri, routeTestHeader+`Route::name('admin.')->group(function () {
    Route::resource('photos', PhotoController::class)->only('index');
});`)
	if idx.Find("admin.photos.index") == nil {
		t.Fatalf("admin.photos.index missing from %v", idx.Names())
	}
	idx.IndexFile(uri, routeTestHeader+`Route::resource('photos', PhotoController::class)->only('index');`)
	if idx.Find("admin.photos.index") != nil || idx.Find("photos.index") == nil {
		t.Fatalf("reindex left %v", idx.Names())
	}
}
