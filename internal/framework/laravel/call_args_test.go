package laravel

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
)

func callArgAt(t *testing.T, code string) (sourcectx.CallArg, bool) {
	t.Helper()
	source := "<?php\n" + code
	at := strings.Index(source, "|")
	if at < 0 {
		at = len(source)
	} else {
		source = source[:at] + source[at+1:]
	}
	before := source[:at]
	pos := protocol.Position{Line: strings.Count(before, "\n"), Character: at - strings.LastIndex(before, "\n") - 1}
	ctx := sourcectx.Analyze("file:///a.php", source, pos, phpparser.OnlyGrammar(token.Latest))
	if ctx == nil {
		t.Fatalf("no cursor context in %q", source)
	}
	return ctx.CallArg()
}

func TestIsRouteNameArg(t *testing.T) {
	tests := []struct {
		name      string
		code      string
		wantOK    bool
		wantPart  string
		wantQuote string
	}{
		{"route helper", "route('da", true, "da", "'"},
		{"routeIs", "request()->routeIs('sec", true, "sec", "'"},
		{"to_route helper", "to_route(\"ho", true, "ho", "\""},
		{"static route", "Redirect::route('ho", true, "ho", "'"},
		{"argument on the next line", "route(\n    'da", true, "da", "'"},
		{"no quote yet", "route(", true, "", ""},
		{"past closing paren", "route('home')", false, "", ""},
		{"second argument", "route('home', ", false, "", ""},
		{"not a route call", "config('app", false, "", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			ok = ok && IsRouteNameArg(arg)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (arg.Partial != tt.wantPart || arg.Quote != tt.wantQuote) {
				t.Fatalf("partial %q quote %q, want %q %q", arg.Partial, arg.Quote, tt.wantPart, tt.wantQuote)
			}
		})
	}
}

func TestViewNameArg(t *testing.T) {
	tests := []struct {
		name     string
		code     string
		wantOK   bool
		wantPart string
		wantName string
	}{
		{"view helper", "view('admin.da", true, "admin.da", "admin.da"},
		{"view helper on the next line", "return view(\n    'admin.da", true, "admin.da", "admin.da"},
		{"route view", "Route::view('/home', 'ho", true, "ho", "ho"},
		{"inside a closed name", "return view(\n    'admin.da|shboard',\n    $data\n);", true, "admin.da", "admin.dashboard"},
		{"route view first argument is the URI", "Route::view('/ho", false, "", ""},
		{"no quote", "view(", false, "", ""},
		{"data argument", "view('home', ['a' => '", false, "", ""},
		{"in a comment", "// view('ho", false, "", ""},
		{"a longer non-ASCII name", "préview('ho", false, "", ""},
		{"after a non-ASCII variable", "$größe = view('ho", true, "ho", "ho"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			var view *ViewContext
			if ok {
				view, ok = ViewNameArg(arg)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (view.Partial != tt.wantPart || view.Quote != "'" || view.Name != tt.wantName) {
				t.Fatalf("view = %#v, want partial %q", view, tt.wantPart)
			}
		})
	}
}

func TestTranslationKeyArg(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		ok      bool
		key     string
		partial string
	}{
		{name: "helper", code: "__('messages.we|lcome')", ok: true, key: "messages.welcome", partial: "messages.we"},
		{name: "helper unclosed", code: "{{ __('messages.we", ok: true, key: "messages.we", partial: "messages.we"},
		{name: "trans", code: "trans(\"auth.fa|", ok: true, key: "auth.fa", partial: "auth.fa"},
		{name: "trans choice", code: "trans_choice('messages.count|', $count)", ok: true, key: "messages.count", partial: "messages.count"},
		{name: "key on the next line", code: "trans_choice(\n    'messages.co|unt',\n    $count\n)", ok: true, key: "messages.count", partial: "messages.co"},
		{name: "facade", code: "Lang::get('messages.w|')", ok: true, key: "messages.w", partial: "messages.w"},
		{name: "qualified facade", code: "\\Illuminate\\Support\\Facades\\Lang::choice('a|')", ok: true, key: "a", partial: "a"},
		{name: "translator get", code: "app('translator')->get('messages.w|')", ok: true, key: "messages.w", partial: "messages.w"},
		{name: "translator choice", code: "\\app(\"translator\")\n    ->choice(\n        'a|', $n)", ok: true, key: "a", partial: "a"},
		{name: "another binding", code: "app('cache')->get('a|"},
		{name: "a translator method that takes no key", code: "app('translator')->has('a|"},
		{name: "app without a binding", code: "app()->get('a|"},
		{name: "a binding that is not a string", code: "app($name)->get('a|"},
		{name: "second argument", code: "__('a', ['x' => '|"},
		{name: "other static call", code: "Cache::get('a|"},
		{name: "a method named get", code: "$lang->get('a|"},
		{name: "other function", code: "config('a|"},
		{name: "a bare name", code: "__(FOO|"},
		{name: "in a comment", code: "// __('a|"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			var tx *TranslationStringContext
			if ok {
				tx, ok = TranslationKeyArg(arg)
			}
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (%#v)", ok, tt.ok, arg)
			}
			if ok && (tx.Key != tt.key || tx.Partial != tt.partial) {
				t.Fatalf("got %#v, want key %q partial %q", tx, tt.key, tt.partial)
			}
		})
	}
}

func TestParseConfigCall(t *testing.T) {
	tests := []struct {
		code string
		ok   bool
		want ConfigCall
	}{
		{code: "config()", ok: true, want: ConfigCall{Repository: true}},
		{code: "config('app.name')", ok: true, want: ConfigCall{Key: "app.name"}},
		{code: "\\config(\"app\", 'fallback')", ok: true, want: ConfigCall{Key: "app"}},
		{code: "config($key)", ok: true},
		{code: "$this->config('app')"},
		{code: "Config::get('app')"},
		{code: "configure('app')"},
		{code: "$config"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			src := "<?php\n" + tt.code + ";\n"
			tree := phpparser.Parse(src, token.Latest)
			stmt, ok := tree.Stmts[0].(*ast.ExprStmt)
			if !ok {
				t.Fatalf("%q is not an expression statement", tt.code)
			}
			got, ok := ParseConfigCall(src, stmt.Expr)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("got %#v, %v; want %#v, %v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestIsConfigKeyArg(t *testing.T) {
	tests := []struct {
		code string
		want bool
	}{
		{code: "config('app.na|", want: true},
		{code: "\\config(\n    'app|'\n)", want: true},
		{code: "config('app', '|"},
		{code: "config(['app.name' => '|"},
		{code: "$this->config('|"},
		{code: "route('|"},
	}
	for _, tt := range tests {
		t.Run(tt.code, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			if got := ok && IsConfigKeyArg(arg); got != tt.want {
				t.Fatalf("IsConfigKeyArg = %v, want %v (%#v)", got, tt.want, arg)
			}
		})
	}
}
