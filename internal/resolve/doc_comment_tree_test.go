package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/scope"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const docCommentTreeSource = `<?php
namespace App;
use Other\Named;
class User { public function name(): string { return ''; } }
class Repo { public function each(callable $f): void {} }
function make() { return null; }
function annotated(Repo $repo) {
    /** @var User $u */
    // The user is cached for the request,
    // so this reads it from the cache
    // instead of asking the database,
    // which is slow on the first call
    // and fast on every one after it.
    $u = make();
    echo $u; // var after a comment
    $repo->each(
        /** @param User $p */
        #[Pure]
        function ($p) {
            echo $p; // param after an attribute
        }
    );
    $repo->each(function (User /* the user */ $q) {
        echo $q; // typed param with a comment
    });
    $repo->each(function (User|Repo $both) {
        echo $both; // union param
    });
}
function strings() {
    $note = '@var User $s here';
    echo $s; // var in a string
}
function before() { /** @var User $w */ $w = make(); }
function after() {
    echo $w; // var in the function before
}
function plain() {
    /**
     * @var User $h
     */
    $x = 1;
    /* the repository */
    $h = new Repo();
    echo $h; // after a plain comment
}
function statics() {
    /** @var User $st */
    static $st = null;
    echo $st; // static var
}
function standalone() {
    /** @var User $t */ /** @var Repo $r */
    echo $t; // standalone var
}
`

func docCommentTreeFixture(t *testing.T) (*Resolver, *parser.FileNode, []string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///docs.php", docCommentTreeSource)
	return NewResolver(idx), parser.ParseFile(docCommentTreeSource, token.Latest), SplitLines(docCommentTreeSource)
}

// Doc comments come from the syntax tree: the one right before a statement
// or a closure, wherever a line break, comment or attribute puts it, and
// never text in a string or a doc comment of another function.
func TestDocCommentsAreReadFromTheSyntaxTree(t *testing.T) {
	r, file, lines := docCommentTreeFixture(t)
	for _, tc := range []struct{ marker, variable, typed, plain string }{
		{"var after a comment", "$u", `App\User`, `App\User`},
		{"param after an attribute", "$p", `App\User`, `App\User`},
		{"typed param with a comment", "$q", `App\User`, `App\User`},
		{"union param", "$both", "User|Repo", "User|Repo"},
		{"var in a string", "$s", "", ""},
		{"var in the function before", "$w", "", ""},
		{"standalone var", "$t", `App\User`, `App\User`},
		{"static var", "$st", `App\User`, `App\User`},
		{"after a plain comment", "$h", `App\Repo`, `App\Repo`},
	} {
		pos := textpos.Position{Line: markedLine(t, lines, tc.marker), Character: 13}
		if got := r.ResolveVariableTypeTyped(tc.variable, lines, pos, file).String(); got != tc.typed {
			t.Errorf("%s: typed %s = %q, want %q", tc.marker, tc.variable, got, tc.typed)
		}
		if got := r.ResolveVariableType(tc.variable, lines, pos, file); got != tc.plain {
			t.Errorf("%s: plain %s = %q, want %q", tc.marker, tc.variable, got, tc.plain)
		}
	}
}

func TestClosureDocStopsAtTheMemberItIsIn(t *testing.T) {
	src := "<?php\n/** @param int $x */\nclass C {\n    const F = static function ($x) {};\n}\n/** @param int $y */\nf(fn ($y) => $y);\n"
	tree := newSyntaxTree(src, parser.ParseTree(src, token.Latest))
	closure, doc, ok := tree.closureAt(uint32(strings.Index(src, "($x)")))
	if !ok || len(closure.params) != 1 || doc != (ast.Span{}) {
		t.Errorf("a closure in a class constant takes no doc from the class: %v, %q", ok, tree.text(doc))
	}
	if _, doc, ok := tree.closureAt(uint32(strings.Index(src, "($y)"))); !ok || tree.text(doc) != "/** @param int $y */" {
		t.Errorf("a closure in a statement takes the statement's doc: %v, %q", ok, tree.text(doc))
	}
	if _, _, ok := tree.closureAt(0); ok {
		t.Error("no closure at the start of the file")
	}
}

func TestDocLookupsOutsideTheSourceFindNothing(t *testing.T) {
	r, file, lines := docCommentTreeFixture(t)
	tree := r.syntaxTree(lines, file)
	outside := textpos.Position{Line: len(lines) + 1}
	if got := r.ResolveVariableType("$t", lines, outside, file); got != "" {
		t.Errorf("ResolveVariableType past the end = %q", got)
	}
	if _, ok := tree.assignmentStartingLine(len(lines)+1, "$u"); ok {
		t.Error("assignmentStartingLine found an assignment past the end")
	}
	site := scope.WriteSite{At: outside}
	if _, ok := (variableScope{write: &site}).assignedValue(tree, outside.Line, "$u"); ok {
		t.Error("assignedValue found a write past the end")
	}
	for _, at := range []textpos.Position{outside, {Line: 1}} {
		param := &scope.Binding{Name: "$p", Decl: textpos.Range{Start: at}}
		if got := r.closureParamType(param, lines, file); !got.IsEmpty() {
			t.Errorf("closureParamType at %v = %q", at, got.String())
		}
	}
}
