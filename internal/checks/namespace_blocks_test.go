package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const twoBlocksImportingThing = `<?php
namespace A;
use Lib\One\Thing;
class Helper {}
function a() { return [new Thing(), new Helper()]; }
namespace B;
use Lib\Two\Thing;
function b() { return new Thing(); }
`

const twoBracedBlocksImportingThing = `<?php
namespace A {
use Lib\One\Thing;
class Helper {}
function a() { return [new Thing(), new Helper()]; }
}
namespace B {
use Lib\Two\Thing;
function b() { return new Thing(); }
}
`

func TestUnknownClassResolvesThroughTheNamespaceBlockOfTheName(t *testing.T) {
	sources := map[string]string{"statement": twoBlocksImportingThing, "braced": twoBracedBlocksImportingThing}
	declared := []struct {
		namespace string
		wantLine  map[string]int
	}{
		{`Lib\One`, map[string]int{"statement": 7, "braced": 8}},
		{`Lib\Two`, map[string]int{"statement": 4, "braced": 4}},
	}
	for _, form := range []string{"statement", "braced"} {
		source := sources[form]
		for _, d := range declared {
			t.Run(form+" "+d.namespace, func(t *testing.T) {
				idx := symbols.NewIndex()
				idx.IndexFile("file:///lib.php", "<?php\nnamespace "+d.namespace+";\nclass Thing {}\n")
				idx.IndexFile("file:///test.php", source)
				findings := (&UnknownClassRule{}).Check(parser.ParseFile(source), source, idx)
				if len(findings) != 1 || findings[0].StartLine != d.wantLine[form] || findings[0].Message != "Unknown class 'Thing'" {
					t.Fatalf("with only %s\\Thing declared, findings = %+v; want one unknown Thing on line %d", d.namespace, findings, d.wantLine[form])
				}
			})
		}
	}
}

func TestInvalidOverrideUsesTheNamespaceBlockOfTheClass(t *testing.T) {
	source := `<?php
namespace A;
use Override as Marker;
class Base { public function f() {} }
class Child extends Base {
    #[Marker] public function f() {}
    #[\Override] public function missing() {}
    #[Marker] public const LIMIT = 1;
}
enum Level { #[\Override] public const TOP = 1; }
namespace B;
class Marker {}
class Other { #[Marker] public function g() {} }
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///test.php", source)
	var got []string
	for _, f := range (&InvalidOverrideRule{}).Check(parser.ParseFile(source), source, idx) {
		got = append(got, f.Message)
	}
	want := []string{
		"'missing' has #[\\Override], but there's no such method to override",
		"'LIMIT' has #[\\Override], but there's no such constant to override",
		"'TOP' has #[\\Override], but there's no such constant to override",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestUnusedPrivateReadsTheTraitsOfAClassInAnEarlierNamespaceBlock(t *testing.T) {
	source := `<?php
namespace A;
class Worker {
    use UsesHelper;
    private function helper() {}
    private function unused() {}
}
namespace B;
class Other {}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///trait.php", "<?php\nnamespace A;\ntrait UsesHelper { public function run() { return $this->helper(); } }\n")
	idx.IndexFile("file:///test.php", source)
	findings := (&UnusedPrivateRule{}).Check(parser.ParseFile(source), source, idx)
	if len(findings) != 1 || findings[0].Message != "Private method 'Worker::unused' is never used" {
		t.Errorf("findings = %+v; want only Worker::unused", findings)
	}
}

func TestUnknownMemberFindsClassLikesDeclaredInAnEarlierNamespaceBlock(t *testing.T) {
	source := `<?php
namespace A;
trait Marker {}
enum Mode { case On; public function label() {} }
class Plain { public function known() {} }
function f() { Plain::known(); Plain::missing(); Mode::On; Mode::label(); Mode::Off; Marker::anything(); }
namespace B;
class Other {}
`
	checker := &UnknownMemberRule{TypeResolver: func(string, string, int, int, *parser.FileNode) string { return "" }}
	got := unknownMemberMessages(checker.Check(parser.ParseFile(source), source, symbols.NewIndex()))
	want := []string{"Unknown member 'missing' on 'Plain'", "Unknown member 'Off' on 'Mode'"}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestResolveClassLikeNameWithoutANamespaceCannotTell(t *testing.T) {
	if got, ok := resolveClassLikeName("Thing", 0, 0, &parser.FileNode{}); ok {
		t.Errorf("resolved %q in a file without namespaces; want no answer", got)
	}
}

func TestUnknownClassKeepsImportsAcrossRouteNamespace(t *testing.T) {
	source := `<?php
use App\Http\Controllers\UserController;
use Illuminate\Support\Facades\Route;

Route::namespace('Admin')->group(fn () => Route::get('/', [UserController::class, 'index']));
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///UserController.php", "<?php\nnamespace App\\Http\\Controllers;\nclass UserController {}\n")
	idx.IndexFile("file:///Route.php", "<?php\nnamespace Illuminate\\Support\\Facades;\nclass Route {}\n")
	idx.IndexFile("file:///routes/web.php", source)
	if findings := (&UnknownClassRule{}).Check(parser.ParseFile(source), source, idx); len(findings) != 0 {
		t.Fatalf("findings = %+v, want none", findings)
	}
}
