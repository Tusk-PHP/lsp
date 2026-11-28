package laravel

import "testing"

const routeTargetsSource = `<?php

use App\Http\Controllers\{Admin\PostController as Posts, ReportController};
use App\Http;
use function App\helper;
use Illuminate\Support\Facades\Route;

Route::get('/a', [Posts::class, 'index'])->name('a');
Route::get('/b', [ReportController::class, 'show'])->name('b');
Route::get('/c', [Http\Controllers\HomeController::class, 'home'])->name('c');
Route::get('/d', [\App\Other::class, 'go'])->name('d');
Route::get('/e', [Local::class, 'run'])->name('e');
Route::get('/f', 'handler')->name('f');
Route::get('/g', '\App\Legacy@run')->name('g');
Route::get('/h', [$controller, 'index'])->name('h');
Route::get('/i', [Foo::BAR, 'x'])->name('i');
Route::get('/j', [static::class, 'x'])->name('j');
Route::controller($c)->group(function () {
    Route::get('/k', 'index')->name('k');
});
Route::group(['name' => 'ignored', 5 => 'x', 'as' => 'grp.'], function () {
    Route::get('/l', fn () => 1)->name('l');
});
`

func TestRouteTargetsResolveTheirClassNames(t *testing.T) {
	routes := map[string]RouteName{}
	for _, r := range parseRouteDefinitions("file:///tmp/project/routes/web.php", routeTargetsSource) {
		routes[r.Name] = r
	}
	want := map[string][2]string{
		"a":     {"controller", `App\Http\Controllers\Admin\PostController@index`},
		"b":     {"controller", `App\Http\Controllers\ReportController@show`},
		"c":     {"controller", `App\Http\Controllers\HomeController@home`},
		"d":     {"controller", `App\Other@go`},
		"e":     {"controller", `Local@run`},
		"f":     {"string", "handler"},
		"g":     {"controller", `App\Legacy@run`},
		"h":     {"route", "[$controller, 'index']"},
		"i":     {"route", "[Foo::BAR, 'x']"},
		"j":     {"route", "[static::class, 'x']"},
		"k":     {"string", "index"},
		"grp.l": {"route", "fn () => 1"},
	}
	if len(routes) != len(want) {
		t.Fatalf("routes = %v, want %d of them", routeNames(routes), len(want))
	}
	for name, w := range want {
		if r := routes[name]; r.TargetKind != w[0] || r.Target != w[1] {
			t.Errorf("%s: target %s %q, want %s %q", name, r.TargetKind, r.Target, w[0], w[1])
		}
	}
}
