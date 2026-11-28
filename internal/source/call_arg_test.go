package sourcectx

import (
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
		{name: "function", code: "\\config('a|", ok: true, want: ast.CallArg{Function: "config", Value: ast.ArgString, Partial: "a", Quote: "'"}},
		{name: "callable variable", code: "$fn('a|')", ok: true, want: ast.CallArg{Value: ast.ArgString, Partial: "a", Quote: "'"}},
		{name: "dynamic method", code: "$x->$m('a|')", ok: true, want: ast.CallArg{Value: ast.ArgString, Partial: "a", Quote: "'"}},
		{name: "static method", code: "Foo::bar(b|)", ok: true, want: ast.CallArg{Method: "bar", Static: true, Value: ast.ArgName, Partial: "b"}},
		{name: "second argument", code: "f('a', '|')", ok: true, want: ast.CallArg{Function: "f", Index: 1, Value: ast.ArgString, Quote: "'"}},
		{name: "before the first argument", code: "f(|'a')", ok: true, want: ast.CallArg{Function: "f"}},
		{name: "before a later argument", code: "f('a', |'b')", ok: true, want: ast.CallArg{Function: "f", Index: 1}},
		{name: "plain part of an interpolated string", code: "f(\"a|$b\")", ok: true, want: ast.CallArg{Function: "f", Value: ast.ArgString, Partial: "a", Quote: `"`}},
		{name: "array after a skipped slot", code: "f([, 'a|'])", ok: true, inArray: true, want: ast.CallArg{Function: "f", Value: ast.ArgString, Partial: "a", Quote: "'"}},
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
			arg.Call, arg.Receiver, arg.InArray = nil, nil, false
			if arg != tt.want {
				t.Fatalf("got %#v, want %#v", arg, tt.want)
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
	}{
		{code: "$a[|", ok: true, value: ast.ArgEmpty},
		{code: "$a['k|", ok: true, value: ast.ArgString, partial: "k"},
		{code: "$a['b']['k|']", ok: true, value: ast.ArgString, partial: "k"},
		{code: "$a[FOO|"},
		{code: "$a[1|]"},
		{code: "$a|"},
		{code: "$a['k']|"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			dim, ok := analyzeMarked(t, tt.code).StringDim()
			if ok != tt.ok || ok && (dim.Value != tt.value || dim.Partial != tt.partial || dim.Fetch == nil) {
				t.Fatalf("got %#v, %v", dim, ok)
			}
		})
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
