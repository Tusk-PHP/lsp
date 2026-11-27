package laravel

import (
	"strconv"
	"strings"
	"testing"
)

func benchmarkRouteSource(groups int) string {
	var b strings.Builder
	b.WriteString("<?php\n\nnamespace App;\n\nuse App\\Http\\Controllers\\PhotoController;\nuse App\\Http\\Controllers\\UserController;\nuse Illuminate\\Support\\Facades\\Route;\n\n")
	for i := 0; i < groups; i++ {
		n := strconv.Itoa(i)
		b.WriteString("Route::prefix('area" + n + "')->name('area" + n + ".')->middleware(['auth', 'verified'])->group(function () {\n")
		b.WriteString("    Route::get('/', [UserController::class, 'index'])->name('home');\n")
		b.WriteString("    Route::post('/users', [UserController::class, 'store'])->name('users.store');\n")
		b.WriteString("    Route::match(['get', 'post'], '/contact', [UserController::class, 'contact'])->name('contact');\n")
		b.WriteString("    Route::view('/about', 'about')->name('about');\n")
		b.WriteString("    Route::resource('photos', PhotoController::class)->except(['destroy']);\n")
		b.WriteString("    Route::group(['as' => 'admin.', 'prefix' => 'admin'], function () {\n")
		b.WriteString("        Route::get('/stats', fn () => view('stats', ['items' => [1, 2, 3]]))->name('stats');\n")
		b.WriteString("        Route::apiResource('photos.comments', PhotoController::class);\n")
		b.WriteString("    });\n")
		b.WriteString("});\n\n")
	}
	return b.String()
}

func BenchmarkParseRouteDefinitions(b *testing.B) {
	src := benchmarkRouteSource(50)
	b.SetBytes(int64(len(src)))
	b.ReportAllocs()
	for b.Loop() {
		parseRouteDefinitions("file:///project/routes/web.php", src)
	}
}

func BenchmarkRouteIndexReindexFile(b *testing.B) {
	idx := NewRouteIndex("/project")
	var web strings.Builder
	web.WriteString("<?php\n\nuse Illuminate\\Support\\Facades\\Route;\n\n")
	for i := 0; i < 10; i++ {
		n := strconv.Itoa(i)
		idx.IndexFile("file:///project/routes/area"+n+".php", benchmarkRouteSource(5))
		web.WriteString("Route::prefix('p" + n + "')->name('p" + n + ".')->group(base_path('routes/area" + n + ".php'));\n")
	}
	idx.IndexFile("file:///project/routes/web.php", web.String())
	edited := benchmarkRouteSource(5)
	b.ReportAllocs()
	for b.Loop() {
		idx.IndexFile("file:///project/routes/area3.php", edited)
	}
}
