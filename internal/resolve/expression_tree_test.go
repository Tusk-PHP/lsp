package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const expressionTreeSource = `<?php
namespace App;
class User { public function name(): string { return ''; } }
class Special extends User { public function special(): Query { return new Query(); } }
class Query { public function first(): User { return new User(); } }
class Repo {
    public function find(string $q): Query { return new Query(); }
    public function make(): User { return new User(); }
    /** @param callable(User): mixed $f */
    public function each(string $label, callable $f): void {}
    public static function create(): static { return new static(); }
}
class Child extends Repo {
    public function run(Repo $repo): void {
        echo $repo; // receivers
        if (($r = $repo->make()) instanceof Special && $r->special()) {
            echo $r; // assigned in the condition
        }
        $s = $repo->make();
        $ok = $s instanceof Special && $s->special();
        $repo->each(')', function ($u) {
            echo $u; // closure after a paren in a string
        });
        if ($s->name() && $s instanceof Special) {
            $q = $s->special();
            $repo->each('x', static function () use ($q, $s) {
                $s->special();
                echo $s; // captured where it was narrowed
            });
        }
    }
}
/**
 * @template T
 * @param T $value
 * @return T
 */
function pick(string $label, $value) { return $value; }
`

func expressionTreeFixture(t *testing.T) (*Resolver, *parser.FileNode, []string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///tree.php", expressionTreeSource)
	return NewResolver(idx), parser.ParseFile(expressionTreeSource, token.Latest), SplitLines(expressionTreeSource)
}

func markedLine(t *testing.T, lines []string, marker string) int {
	t.Helper()
	for i, l := range lines {
		if strings.HasSuffix(l, "// "+marker) {
			return i
		}
	}
	t.Fatalf("no line marked %q", marker)
	return -1
}

// Arguments and chains are split by the syntax tree, so strings, escapes,
// comments and heredocs inside them don't move where they end.
func TestExpressionsAreTypedFromTheSyntaxTree(t *testing.T) {
	r, file, lines := expressionTreeFixture(t)
	pos := textpos.Position{Line: markedLine(t, lines, "receivers"), Character: 8}
	ctx := ExprContext{Lines: lines, Pos: pos, File: file}
	for expr, want := range map[string]string{
		`pick('a\\', new User())`:                                 `App\User`,
		`pick('a, b', new User())`:                                `App\User`,
		"$repo->find(')')->first()":                               `App\User`,
		"$repo->find(/* ) */ 'x')->first()":                       `App\User`,
		"$repo\n    // the first\n    ->find('x')\n    ->first()": `App\User`,
		"$repo->find(<<<SQL\n    a = ')'\n    SQL)->first()":      `App\User`,
		"(clone $repo)->make()":                                   `App\User`,
		"static::create()":                                        `App\Child`,
	} {
		if got := r.exprType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%q) = %q, want %q", expr, got, want)
		}
	}
}

// A chain is typed link by link, so its length isn't bounded by how deeply
// nested expressions are followed: a Filament panel or a query builder chains
// dozens of calls.
func TestLongChainsAreTyped(t *testing.T) {
	r, file, lines := expressionTreeFixture(t)
	pos := textpos.Position{Line: markedLine(t, lines, "receivers"), Character: 8}
	ctx := ExprContext{Lines: lines, Pos: pos, File: file}
	links := 4 * maxExprDepth
	var b strings.Builder
	b.WriteString("Repo::create()")
	for range links {
		b.WriteString("\n    ->create()")
	}
	for expr, want := range map[string]string{
		b.String():                      `App\Repo`,
		b.String() + "->make()->name()": "string",
		b.String() + "->find(pick('q', $repo->find('x')->first()->name()))->first()": `App\User`,
	} {
		if got := r.exprType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%.40q…) = %q, want %q", expr, got, want)
		}
	}
}

// The receiver of the member access at the cursor is read from the file's
// tree, also while the access is still being typed.
func TestReceiverAt(t *testing.T) {
	r, _, _ := expressionTreeFixture(t)
	for _, c := range []struct {
		code, want     string
		static, caller bool
	}{
		{code: "$repo->", want: `App\Repo`},
		{code: "$repo->fi", want: `App\Repo`},
		{code: "$repo->find('x')\n            // comment\n            ->", want: `App\Query`},
		{code: "Repo::", want: `App\Repo`, static: true},
		{code: "parent::", want: `App\Repo`, static: true, caller: true},
		{code: "$repo->find(')')->first()->", want: `App\User`},
	} {
		src := strings.Replace(expressionTreeSource, "echo $repo; // receivers", c.code+"\n", 1)
		file := parser.ParseFile(src, token.Latest)
		end := strings.Index(src, c.code) + len(c.code)
		pos := textpos.Position{Line: strings.Count(src[:end], "\n"), Character: end - strings.LastIndex(src[:end], "\n") - 1}
		recv, ok := r.ReceiverAt(src, pos, file)
		if !ok || recv.Type.FullString() != c.want || recv.Static != c.static || recv.ForwardsCaller != c.caller {
			t.Errorf("ReceiverAt after %q = %+v, %v; want %s static=%v caller=%v", c.code, recv, ok, c.want, c.static, c.caller)
		}
	}
}

// A test on an assignment narrows the variable assigned, in the rest of the
// condition and in the block it guards; a closure passed after an argument
// with a parenthesis in a string still takes its parameter's type.
func TestNarrowingAndClosuresOnTheSyntaxTree(t *testing.T) {
	r, file, lines := expressionTreeFixture(t)
	at := func(v, marker string) string {
		line := markedLine(t, lines, marker)
		return r.ResolveVariableTypeTyped(v, lines, textpos.Position{Line: line, Character: strings.Index(lines[line], v) + 1}, file).FullString()
	}
	if got := at("$r", "assigned in the condition"); got != `App\Special` {
		t.Errorf("$r in the if block = %q, want App\\Special", got)
	}
	if got := at("$u", "closure after a paren in a string"); got != `App\User` {
		t.Errorf("closure parameter = %q, want App\\User", got)
	}
	if got := at("$s", "captured where it was narrowed"); got != `App\Special` {
		t.Errorf("captured variable = %q, want App\\Special", got)
	}
	use := strings.Index(expressionTreeSource, "use ($q, $s)") + len("use ($q, ")
	usePos := textpos.Position{Line: strings.Count(expressionTreeSource[:use], "\n")}
	usePos.Character = use - strings.LastIndex(expressionTreeSource[:use], "\n")
	if got := r.ResolveVariableTypeTyped("$s", lines, usePos, file).FullString(); got != `App\Special` {
		t.Errorf("variable in the use clause = %q, want App\\Special", got)
	}
	call := strings.Index(expressionTreeSource, "$s->special()")
	pos := textpos.Position{Line: strings.Count(expressionTreeSource[:call], "\n")}
	pos.Character = strings.Index(lines[pos.Line], "$s->special()") + 1
	if got := r.ResolveVariableTypeTyped("$s", lines, pos, file).FullString(); got != `App\Special` {
		t.Errorf("$s after && = %q, want App\\Special", got)
	}
	pos.Character = strings.Index(lines[pos.Line], "->special") + 2
	if got := r.ReceiverTypeAt(expressionTreeSource, pos, file).FullString(); got != `App\Special` {
		t.Errorf("receiver of special() = %q, want App\\Special", got)
	}
}

func cursorAfter(src, code string) textpos.Position {
	end := strings.Index(src, code) + len(code)
	return textpos.Position{Line: strings.Count(src[:end], "\n"), Character: end - strings.LastIndex(src[:end], "\n") - 1}
}

func TestReceiverAtEdges(t *testing.T) {
	r, _, _ := expressionTreeFixture(t)
	for _, c := range []struct {
		code, want string
		ok, static bool
	}{
		{code: "$repo->   ", want: `App\Repo`, ok: true},
		{code: "Repo ::", want: `App\Repo`, ok: true, static: true},
		{code: "Repo::create", want: `App\Repo`, ok: true, static: true},
		{code: "Repo::$items", want: `App\Repo`, ok: true, static: true},
		{code: "Repo::LIMIT", want: `App\Repo`, ok: true, static: true},
		{code: "Repo::(", ok: false},
		{code: "  ::", ok: false},
		{code: "echo 1 +", ok: false},
		{code: "$repo", ok: false},
		{code: "Repo::'x'", ok: false},
		{code: "Repo::1", ok: false},
		{code: "Repo::[", ok: false},
		{code: "Repo::-", ok: false},
		{code: "Repo::$", want: `App\Repo`, ok: true, static: true},
	} {
		src := strings.Replace(expressionTreeSource, "echo $repo; // receivers", c.code+"\n", 1)
		recv, ok := r.ReceiverAt(src, cursorAfter(src, c.code), parser.ParseFile(src, token.Latest))
		if ok != c.ok || recv.Type.FullString() != c.want || recv.Static != c.static {
			t.Errorf("ReceiverAt after %q = %+v, %v; want %q static=%v ok=%v", c.code, recv, ok, c.want, c.static, c.ok)
		}
	}
	if _, ok := r.ReceiverAt(expressionTreeSource, textpos.Position{}, nil); ok {
		t.Error("ReceiverAt without a file found a receiver")
	}
}

func TestCallTypeAt(t *testing.T) {
	r, _, _ := expressionTreeFixture(t)
	src := strings.Replace(expressionTreeSource, "echo $repo; // receivers", "Repo::create(); $repo->items;", 1)
	file := parser.ParseFile(src, token.Latest)
	for _, c := range []struct{ code, want string }{
		{"$s = $repo->make", `App\User`},
		{"Repo::create", `App\Repo`},
		{"Repo::create(", ""},
		{"$repo->items", ""},
		{"public static function create", ""},
	} {
		if got := r.CallTypeAt(src, cursorAfter(src, c.code), file).FullString(); got != c.want {
			t.Errorf("CallTypeAt after %q = %q, want %q", c.code, got, c.want)
		}
	}
	if got := r.CallTypeAt(src, cursorAfter(src, "$s = $repo->make"), nil); !got.IsEmpty() {
		t.Errorf("CallTypeAt without a file = %q", got.FullString())
	}
}

func TestNodeTypeTypesANodeOfTheFilesTree(t *testing.T) {
	r, file, _ := expressionTreeFixture(t)
	var call ast.Expr
	ast.Walk(file.Tree, func(n ast.Node) bool {
		if m, ok := n.(*ast.MethodCall); ok && call == nil && strings.HasPrefix(m.Bounds().Text(expressionTreeSource), "$repo->make()") {
			call = m
		}
		return call == nil
	})
	if call == nil {
		t.Fatal("no $repo->make() call in the fixture")
	}
	if got := r.NodeType(file, call).FullString(); got != `App\User` {
		t.Errorf("NodeType($repo->make()) = %q, want App\\User", got)
	}
}
