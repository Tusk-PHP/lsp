package analyzer

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDocumentLinks(t *testing.T) {
	existing := map[string]bool{
		"/app/bootstrap/app.php":   true,
		"/app/vendor/autoload.php": true,
		"/app/src/helpers.php":     true,
		"/app/routes/web.php":      true,
		"/app/resources/views":     true,
		"/app/src/Http/x.php":      true,
	}
	exists := func(path string) bool { return existing[path] }
	source := `<?php
require_once __DIR__ . '/../vendor/autoload.php';
include(dirname(__DIR__) . "/bootstrap/app.php");
require dirname(__DIR__, 1) . '/src/helpers.php';
require __DIR__ . '/Http/x.php' ?>
<?php
require 'relative.php';
require __DIR__ . '/missing.php';
require $base . '/x.php';
require __DIR__ . '/x.php' . $suffix;
require dirname(__DIR__, 0) . '/x.php';
require dirname(__DIR__, $n) . '/x.php';
require dirname(__DIR__;
require (__DIR__ . '/Http/x.php';
require __DIR__ .
require dirname . '/x.php';
$web = base_path('routes/web.php');
$views = resource_path('views');
$none = base_path($path);
$missing = app_path('Nope.php');
$called = base_path;
/**
 * @see https://laravel.com/docs/routing, and
 * http://example.com/a?b=1.
 */
`
	a, _ := setupRenameAnalyzer(nil)
	a.framework = "laravel"
	links := a.DocumentLinks("file:///app/src/index.php", source, "/app", exists)

	assertLinks(t, source, links, []foundLink{
		{1, `__DIR__ . '/../vendor/autoload.php'`, "file:///app/vendor/autoload.php"},
		{2, `(dirname(__DIR__) . "/bootstrap/app.php")`, "file:///app/bootstrap/app.php"},
		{3, `dirname(__DIR__, 1) . '/src/helpers.php'`, "file:///app/src/helpers.php"},
		{4, `__DIR__ . '/Http/x.php'`, "file:///app/src/Http/x.php"},
		{16, `'routes/web.php'`, "file:///app/routes/web.php"},
		{17, `'views'`, "file:///app/resources/views"},
		{22, `https://laravel.com/docs/routing`, "https://laravel.com/docs/routing"},
		{23, `http://example.com/a?b=1`, "http://example.com/a?b=1"},
	})

	a.framework = "symfony"
	for _, l := range a.DocumentLinks("file:///app/src/index.php", source, "/app", exists) {
		if strings.Contains(l.Target, "routes/web.php") {
			t.Error("Laravel's path helpers only link in a Laravel project")
		}
	}
	if got := a.DocumentLinks("file:///app/src/index.php", "<?php\n", "/app", exists); got == nil || len(got) != 0 {
		t.Errorf("no links is an empty list: %#v", got)
	}
}

type foundLink struct {
	line   int
	text   string
	target string
}

func assertLinks(t *testing.T, source string, links []protocol.DocumentLink, want []foundLink) {
	t.Helper()
	lines := strings.Split(source, "\n")
	got := make([]foundLink, 0, len(links))
	for _, l := range links {
		if l.Range.Start.Line != l.Range.End.Line {
			t.Fatalf("link spans lines: %+v", l)
		}
		got = append(got, foundLink{l.Range.Start.Line, lines[l.Range.Start.Line][l.Range.Start.Character:l.Range.End.Character], l.Target})
	}
	if len(got) != len(want) {
		t.Fatalf("got %d links, want %d:\n%+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d: got %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestDocumentLinksSkipMethodsAndNamespacedFunctions(t *testing.T) {
	existing := map[string]bool{"/app": true, "/app/config": true, "/app/routes/web.php": true, "/app/storage/logs": true}
	source := `<?php
$this->storage_path('logs');
$this?->storage_path('logs');
Foo::base_path('routes/web.php');
function config_path() {}
App\Support\base_path('routes/web.php');
$this->require(__DIR__ . '/x.php');
$a = \base_path('routes/web.php');
$b = config_path();
$c = base_path( );
$d = storage_path();
$e = public_path();
`
	a, _ := setupRenameAnalyzer(nil)
	a.framework = "laravel"
	links := a.DocumentLinks("file:///app/src/x.php", source, "/app", func(path string) bool { return existing[path] })
	assertLinks(t, source, links, []foundLink{
		{7, `'routes/web.php'`, "file:///app/routes/web.php"},
		{8, `config_path()`, "file:///app/config"},
		{9, `base_path( )`, "file:///app"},
	})
}

func TestDocumentLinksForDirnameFileAndDirectorySeparator(t *testing.T) {
	if filepath.Separator != '/' {
		t.Skip("the expected paths use a slash separator")
	}
	existing := map[string]bool{"/app/config/app.php": true, "/app/src/x.php": true, "/app/src/sub/y.php": true}
	source := `<?php
require \dirname(__DIR__) . '/config/app.php';
require dirname(__FILE__) . '/sub/y.php';
require __DIR__ . DIRECTORY_SEPARATOR . 'sub' . DIRECTORY_SEPARATOR . 'y.php';
require \dirname(__FILE__, 2) . \DIRECTORY_SEPARATOR . 'config/app.php';
require __FILE__;
require Foo\dirname(__DIR__) . '/config/app.php';
`
	a, _ := setupRenameAnalyzer(nil)
	links := a.DocumentLinks("file:///app/src/x.php", source, "", func(path string) bool { return existing[path] })
	assertLinks(t, source, links, []foundLink{
		{1, `\dirname(__DIR__) . '/config/app.php'`, "file:///app/config/app.php"},
		{2, `dirname(__FILE__) . '/sub/y.php'`, "file:///app/src/sub/y.php"},
		{3, `__DIR__ . DIRECTORY_SEPARATOR . 'sub' . DIRECTORY_SEPARATOR . 'y.php'`, "file:///app/src/sub/y.php"},
		{4, `\dirname(__FILE__, 2) . \DIRECTORY_SEPARATOR . 'config/app.php'`, "file:///app/config/app.php"},
		{5, `__FILE__`, "file:///app/src/x.php"},
	})
}

func TestDocumentLinkCommentURLEnds(t *testing.T) {
	source := `<?php
// https://en.wikipedia.org/wiki/Foo_(bar).
// (see https://example.com/y)
/** @see https://example.com/x*/
/* https://example.com/a_(b_(c)) and https://example.com/d)) */
`
	a, _ := setupRenameAnalyzer(nil)
	links := a.DocumentLinks("file:///app/x.php", source, "", func(string) bool { return false })
	assertLinks(t, source, links, []foundLink{
		{1, `https://en.wikipedia.org/wiki/Foo_(bar)`, "https://en.wikipedia.org/wiki/Foo_(bar)"},
		{2, `https://example.com/y`, "https://example.com/y"},
		{3, `https://example.com/x`, "https://example.com/x"},
		{4, `https://example.com/a_(b_(c))`, "https://example.com/a_(b_(c))"},
		{4, `https://example.com/d`, "https://example.com/d"},
	})
}

func TestDocumentLinkColumnsAreUTF16(t *testing.T) {
	a, _ := setupRenameAnalyzer(nil)
	source := "<?php\n// Véase 😀 https://a.example/x\n/* é\n ü https://b.example */\n"
	links := a.DocumentLinks("file:///app/x.php", source, "", func(string) bool { return false })
	want := []protocol.Range{
		{Start: protocol.Position{Line: 1, Character: 12}, End: protocol.Position{Line: 1, Character: 31}},
		{Start: protocol.Position{Line: 3, Character: 3}, End: protocol.Position{Line: 3, Character: 20}},
	}
	if len(links) != len(want) {
		t.Fatalf("got %+v", links)
	}
	for i := range want {
		if links[i].Range != want[i] {
			t.Errorf("link %d: got %+v, want %+v", i, links[i].Range, want[i])
		}
	}
}

func TestDocumentLinkPositionsInsideMultilineTokens(t *testing.T) {
	a, _ := setupRenameAnalyzer(nil)
	source := "<?php\n/*\n  see https://a.example/x\n*/\n"
	links := a.DocumentLinks(symbols.PathToURI("/p/f.php"), source, "", func(string) bool { return false })
	want := protocol.Range{Start: protocol.Position{Line: 2, Character: 6}, End: protocol.Position{Line: 2, Character: 25}}
	if len(links) != 1 || links[0].Range != want {
		t.Errorf("got %+v, want %+v", links, want)
	}
}

func BenchmarkDocumentLinks(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\n")
	for i := 0; i < 1000; i++ {
		src.WriteString("// see https://example.com/docs\nrequire __DIR__ . '/../vendor/autoload.php';\n$x = strlen('abc') + count([1, 2, 3]);\n$y = $x . \"suffix\";\nfunction f" + strings.Repeat("x", i%7) + "() { return 1; }\n")
	}
	a, _ := setupRenameAnalyzer(nil)
	exists := func(string) bool { return true }
	source := src.String()
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		a.DocumentLinks("file:///app/src/index.php", source, "/app", exists)
	}
}

func TestPathExprNeedsTokens(t *testing.T) {
	if _, ok := (&pathExpr{}).term(); ok {
		t.Error("an empty expression has no value")
	}
}
