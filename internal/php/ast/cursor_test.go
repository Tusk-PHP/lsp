package ast_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

// cursorTree parses "<?php\n" followed by marked, with the cursor where
// marked has a | (or a ‸, for code that holds a |).
func cursorTree(t *testing.T, marked string) (*ast.File, string, uint32) {
	t.Helper()
	marker := "|"
	if strings.Contains(marked, "‸") {
		marker = "‸"
	}
	before, after, ok := strings.Cut(marked, marker)
	if !ok {
		t.Fatalf("no cursor in %q", marked)
	}
	src := "<?php\n" + before + after
	return parser.Parse(src, token.Latest), src, uint32(len("<?php\n") + len(before))
}

func callArgAt(t *testing.T, marked string) (ast.CallArg, bool) {
	t.Helper()
	file, src, off := cursorTree(t, marked)
	return ast.CallArgAt(file, src, off)
}

func TestCallArg(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		ok      bool
		want    ast.CallArg
		inArray bool
	}{
		{name: "function", code: "\\config('a|", ok: true, want: ast.CallArg{Function: "config", Value: ast.ArgString, Partial: "a", Text: "a", Quote: "'"}},
		{name: "callable variable", code: "$fn('a|')", ok: true, want: ast.CallArg{Value: ast.ArgString, Partial: "a", Text: "a", Quote: "'"}},
		{name: "dynamic method", code: "$x->$m('a|')", ok: true, want: ast.CallArg{Value: ast.ArgString, Partial: "a", Text: "a", Quote: "'"}},
		{name: "static method", code: "\\App\\Foo::bar(b|)", ok: true, want: ast.CallArg{Method: "bar", Class: `App\Foo`, Static: true, Value: ast.ArgName, Partial: "b", Text: "b"}},
		{name: "constructor", code: "new Foo(\n    'ab|c'\n)", ok: true, want: ast.CallArg{Class: "Foo", New: true, Value: ast.ArgString, Partial: "ab", Text: "abc", Quote: "'"}},
		{name: "second argument", code: "f('a', '|')", ok: true, want: ast.CallArg{Function: "f", Index: 1, Value: ast.ArgString, Quote: "'"}},
		{name: "before the first argument", code: "f(|'a')", ok: true, want: ast.CallArg{Function: "f"}},
		{name: "before a later argument", code: "f('a', |'b')", ok: true, want: ast.CallArg{Function: "f", Index: 1}},
		{name: "plain part of an interpolated string", code: "f(\"a|$b\")", ok: true, want: ast.CallArg{Function: "f", Value: ast.ArgString, Partial: "a", Text: "a$b", Quote: `"`}},
		{name: "unclosed interpolated string", code: "f(\"ab|", ok: true, want: ast.CallArg{Function: "f", Value: ast.ArgString, Partial: "ab", Text: "ab", Quote: `"`}},
		{name: "array after a skipped slot", code: "f([, 'a|'])", ok: true, inArray: true, want: ast.CallArg{Function: "f", Value: ast.ArgString, Partial: "a", Text: "a", Quote: "'"}},
		{name: "array after its last item", code: "f(['a', |])", ok: true, inArray: true, want: ast.CallArg{Function: "f"}},
		{name: "space before a later argument", code: "f('a', | 'b')", ok: true, want: ast.CallArg{Function: "f", Index: 1}},
		{name: "after the last argument and a comma", code: "f('a', |)", ok: true, want: ast.CallArg{Function: "f", Index: 1}},
		{name: "integer", code: "f(1|)"},
		{name: "after interpolation", code: "f(\"a$b|\")"},
		{name: "heredoc", code: "f(<<<EOT\na|"},
		{name: "between arguments without a comma", code: "f('a' |'b')"},
		{name: "before a comma", code: "f('a' |, 'b')"},
		{name: "keyed array", code: "f(['k' => 'v', '|"},
		{name: "closure", code: "f(fn() => 1|)"},
		{name: "outside any call", code: "$a = 'x|';"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (%#v)", ok, tt.ok, arg)
			}
			if !ok {
				return
			}
			if arg.Call == nil || arg.InArray != tt.inArray {
				t.Fatalf("call %v, in array %v", arg.Call, arg.InArray)
			}
			got := ast.CallArg{Function: arg.Function, Method: arg.Method, Class: arg.Class, Static: arg.Static, New: arg.New,
				Index: arg.Index, Value: arg.Value, Partial: arg.Partial, Text: arg.Text, Quote: arg.Quote}
			if got != tt.want {
				t.Fatalf("got %#v, want %#v", got, tt.want)
			}
		})
	}
}

func TestStringDim(t *testing.T) {
	tests := []struct {
		code    string
		ok      bool
		value   ast.ArgValue
		partial string
		keys    []string
	}{
		{code: "$a[|", ok: true, value: ast.ArgEmpty},
		{code: "$a['k|", ok: true, value: ast.ArgString, partial: "k"},
		{code: "$a['b']['k|']", ok: true, value: ast.ArgString, partial: "k", keys: []string{"b"}},
		{code: "$a['b']\n    ['c']\n    ['k|", ok: true, value: ast.ArgString, partial: "k", keys: []string{"b", "c"}},
		{code: "$a[$b]['k|"},
		{code: "$a[FOO|"},
		{code: "$a[1|]"},
		{code: "$a|"},
		{code: "$a['k']|"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			file, src, off := cursorTree(t, tt.code)
			dim, ok := ast.StringDimAt(file, src, off)
			if ok != tt.ok || ok && (dim.Value != tt.value || dim.Partial != tt.partial || dim.Fetch == nil || !slices.Equal(dim.Keys, tt.keys)) {
				t.Fatalf("got %#v, %v", dim, ok)
			}
			if ok && dim.Base.Bounds().Text(src) != "$a" {
				t.Fatalf("base %q", dim.Base.Bounds().Text(src))
			}
		})
	}
}

func TestStringArg(t *testing.T) {
	arg, ok := callArgAt(t, "f('a|', [], \"b\", $c)")
	if !ok {
		t.Fatal("no call argument")
	}
	for i, want := range map[int]string{0: "a", 2: "b"} {
		if got, ok := arg.StringArg(i); !ok || got != want {
			t.Errorf("argument %d = %q, %v", i, got, ok)
		}
	}
	for _, i := range []int{1, 3, 4} {
		if got, ok := arg.StringArg(i); ok {
			t.Errorf("argument %d = %q", i, got)
		}
	}
}

func TestLastSegment(t *testing.T) {
	for name, want := range map[string]string{`A\B\Lang`: "Lang", "Lang": "Lang", "": ""} {
		if got := ast.LastSegment(name); got != want {
			t.Errorf("ast.LastSegment(%q) = %q", name, got)
		}
	}
}

func TestStringValue(t *testing.T) {
	_, src, _ := cursorTree(t, "f('a', 1|);")
	arg, _ := callArgAt(t, "f('a', 1|);")
	args := arg.Call.(*ast.Call).Args.Args
	if v, ok := ast.StringValue(src, args[0].Value); !ok || v != "a" {
		t.Errorf("first argument = %q, %v", v, ok)
	}
	if _, ok := ast.StringValue(src, args[1].Value); ok {
		t.Error("an integer is not a string")
	}
}

func TestNameSlot(t *testing.T) {
	tests := []struct {
		name string
		code string
		ok   bool
		want ast.NameSlot
	}{
		{name: "new", code: "$a = new |", ok: true, want: ast.NameSlot{Kind: ast.NameNew}},
		{name: "new right after the keyword", code: "$a = new|", ok: true, want: ast.NameSlot{Kind: ast.NameNew}},
		{name: "new with a name", code: "$a = new Fo|", ok: true, want: ast.NameSlot{Kind: ast.NameNew, Partial: "Fo"}},
		{name: "new on the next line", code: "return new\n    App\\Fo|", ok: true, want: ast.NameSlot{Kind: ast.NameNew, Partial: `App\Fo`}},
		{name: "new inside a call split over lines", code: "$this->add(\n    new Us|\n);", ok: true, want: ast.NameSlot{Kind: ast.NameNew, Partial: "Us"}},
		{name: "new in the arguments", code: "new Foo(|)"},
		{name: "new before the keyword", code: "$a = n|ew Foo"},
		{name: "inside the keyword of a bare new", code: "$a = n|ew"},
		{name: "new from a variable", code: "new $cla|ss"},
		{name: "anonymous class", code: "new class {|}"},
		{name: "new in a comment", code: "// new Fo|"},
		{name: "use", code: "use |", ok: true, want: ast.NameSlot{Kind: ast.NameUse}},
		{name: "use with a name", code: `use App\Mo|`, ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: `App\Mo`}},
		{name: "use function", code: `use function App\he|`, ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: `App\he`}},
		{name: "group use split over lines", code: "use App\\{\n    Models\\Us|", ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: `App\Models\Us`}},
		{name: "group use after a comma", code: "use App\\{Foo, |};", ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: `App\`}},
		{name: "group use right after a comma", code: "use App\\{Foo,|", ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: `App\`}},
		{name: "group use prefix", code: "use Ap|\\{Foo};", ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: "Ap"}},
		{name: "after the use statement", code: "use App\\Foo;|"},
		{name: "use in a comment", code: "// use App|"},
		{name: "trait use", code: "class A {\n    use Has|\n}", ok: true, want: ast.NameSlot{Kind: ast.NameUse, Partial: "Has"}},
		{name: "trait use after a comma", code: "class A {\n    use HasA, |;\n}", ok: true, want: ast.NameSlot{Kind: ast.NameUse}},
		{name: "trait use adaptations", code: "class A {\n    use HasA { fo| }\n}"},
		{name: "after the trait use", code: "class A {\n    use HasA;|\n}"},
		{name: "closure use", code: "function () use ($a|) {};"},
		{name: "attribute", code: "#[|", ok: true, want: ast.NameSlot{Kind: ast.NameAttribute}},
		{name: "attribute with a name", code: "#[Ch|", ok: true, want: ast.NameSlot{Kind: ast.NameAttribute, Partial: "Ch"}},
		{name: "second attribute", code: "#[Unique(), Ro|", ok: true, want: ast.NameSlot{Kind: ast.NameAttribute, Partial: "Ro"}},
		{name: "attribute split over lines", code: "#[\n    Ro|\n]\nfunction f() {}", ok: true, want: ast.NameSlot{Kind: ast.NameAttribute, Partial: "Ro"}},
		{name: "attribute on a method", code: "class A {\n    #[Ov|\n    public function f() {}\n}", ok: true, want: ast.NameSlot{Kind: ast.NameAttribute, Partial: "Ov"}},
		{name: "attribute arguments", code: "#[Route('/x|')]\nfunction f() {}"},
		{name: "after the attribute", code: "#[Foo]|\nfunction f() {}"},
		{name: "attribute in a comment", code: "// #[Fo|"},
		{name: "pipe", code: "$a = $x |> ‸", ok: true, want: ast.NameSlot{Kind: ast.NamePipe}},
		{name: "pipe on the next line", code: "$a = $x\n    |>\n    ‸", ok: true, want: ast.NameSlot{Kind: ast.NamePipe}},
		{name: "pipe with a name", code: "$a = $x |> st‸"},
		{name: "before the pipe", code: "$a = $x‸ |> "},
		{name: "or", code: "$a = $x | ‸"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			file, src, off := cursorTree(t, tt.code)
			slot, ok := ast.NameSlotAt(file, src, off)
			if ok != tt.ok || slot != tt.want {
				t.Fatalf("got %#v, %v; want %#v, %v", slot, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestMemberAccessAt(t *testing.T) {
	for _, c := range []struct {
		code, receiver string
		static         bool
	}{
		{"$a->b|;", "$a", false},
		{"$a->b(|);", "", false},
		{"$a->b()->c|;", "$a->b()", false},
		{"$a?->b|;", "$a", false},
		{"$a->|", "$a", false},
		{"A::b|();", "A", true},
		{"A::$b|;", "A", true},
		{"A::B|;", "A", true},
		{"A::|", "A", true},
		{"$a|;", "", false},
	} {
		file, src, off := cursorTree(t, c.code)
		receiver, _, static := ast.MemberAccess(ast.MemberAccessAt(file, off))
		got := ""
		if receiver != nil {
			got = receiver.Bounds().Text(src)
		}
		if got != c.receiver || receiver != nil && static != c.static {
			t.Errorf("MemberAccessAt(%q) receiver %q static %v, want %q %v", c.code, got, static, c.receiver, c.static)
		}
	}
}

func TestCallAt(t *testing.T) {
	file, src, off := cursorTree(t, "f(1, g(|2));")
	call, args := ast.CallAt(file, off)
	if call == nil || call.Bounds().Text(src) != "g(2)" || len(args.Args) != 1 {
		t.Errorf("CallAt = %v, %v", call, args)
	}
}

func TestReceiverCall(t *testing.T) {
	for _, c := range []struct {
		code, function, first string
		ok                    bool
	}{
		{"app('translator')->get('a|');", "app", "translator", true},
		{"app()->get('a|');", "", "", false},
		{"app($x)->get('a|');", "app", "", false},
		{"$fn('x')->get('a|');", "", "x", false},
		{"$x->get('a|');", "", "", false},
	} {
		arg, ok := callArgAt(t, c.code)
		if !ok {
			t.Fatalf("no call argument in %q", c.code)
		}
		function, first, ok := arg.ReceiverCall()
		if function != c.function || first != c.first || ok != c.ok {
			t.Errorf("ReceiverCall(%q) = %q, %q, %v; want %q, %q, %v", c.code, function, first, ok, c.function, c.first, c.ok)
		}
	}
}
