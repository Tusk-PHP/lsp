package sourcectx

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func analyzeMarked(t *testing.T, marked string) *CursorContext {
	t.Helper()
	source, pos := cursorIn("<?php\n" + marked)
	ctx := Analyze("file:///a.php", source, pos, phpparser.OnlyGrammar(token.Latest))
	if ctx == nil {
		t.Fatalf("no context for %q", marked)
	}
	return ctx
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
			arg, ok := analyzeMarked(t, tt.code).CallArg()
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
			ctx := analyzeMarked(t, tt.code)
			dim, ok := ctx.StringDim()
			if ok != tt.ok || ok && (dim.Value != tt.value || dim.Partial != tt.partial || dim.Fetch == nil || !slices.Equal(dim.Keys, tt.keys)) {
				t.Fatalf("got %#v, %v", dim, ok)
			}
			if ok && dim.Base.Bounds().Text(ctx.File.Source) != "$a" {
				t.Fatalf("base %q", dim.Base.Bounds().Text(ctx.File.Source))
			}
		})
	}
}

func TestStringArg(t *testing.T) {
	arg, ok := analyzeMarked(t, "f('a|', [], \"b\", $c)").CallArg()
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
	ctx := analyzeMarked(t, "f('a', 1|);")
	arg, _ := ctx.CallArg()
	args := arg.Call.(*ast.Call).Args.Args
	if v, ok := ast.StringValue(ctx.File.Source, args[0].Value); !ok || v != "a" {
		t.Errorf("first argument = %q, %v", v, ok)
	}
	if _, ok := ast.StringValue(ctx.File.Source, args[1].Value); ok {
		t.Error("an integer is not a string")
	}
}
