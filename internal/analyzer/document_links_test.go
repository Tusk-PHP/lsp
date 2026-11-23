package analyzer

import (
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

	type found struct {
		line   int
		text   string
		target string
	}
	lines := strings.Split(source, "\n")
	var got []found
	for _, l := range links {
		if l.Range.Start.Line != l.Range.End.Line {
			t.Fatalf("link spans lines: %+v", l)
		}
		got = append(got, found{l.Range.Start.Line, lines[l.Range.Start.Line][l.Range.Start.Character:l.Range.End.Character], l.Target})
	}
	want := []found{
		{1, `__DIR__ . '/../vendor/autoload.php'`, "file:///app/vendor/autoload.php"},
		{2, `(dirname(__DIR__) . "/bootstrap/app.php")`, "file:///app/bootstrap/app.php"},
		{3, `dirname(__DIR__, 1) . '/src/helpers.php'`, "file:///app/src/helpers.php"},
		{4, `__DIR__ . '/Http/x.php'`, "file:///app/src/Http/x.php"},
		{16, `'routes/web.php'`, "file:///app/routes/web.php"},
		{17, `'views'`, "file:///app/resources/views"},
		{22, `https://laravel.com/docs/routing`, "https://laravel.com/docs/routing"},
		{23, `http://example.com/a?b=1`, "http://example.com/a?b=1"},
	}
	if len(got) != len(want) {
		t.Fatalf("got %d links:\n%+v", len(got), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("link %d: got %+v, want %+v", i, got[i], want[i])
		}
	}

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
