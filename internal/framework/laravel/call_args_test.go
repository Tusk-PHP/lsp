package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func callArgAt(code string) (ast.CallArg, bool) {
	source := "<?php\n" + code
	return ast.CallArgAt(parser.ParseTree(source, token.Latest), source, uint32(len(source)))
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
			arg, ok := callArgAt(tt.code)
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
	}{
		{"view helper", "view('admin.da", true, "admin.da"},
		{"view helper on the next line", "return view(\n    'admin.da", true, "admin.da"},
		{"route view", "Route::view('/home', 'ho", true, "ho"},
		{"route view first argument is the URI", "Route::view('/ho", false, ""},
		{"no quote", "view(", false, ""},
		{"data argument", "view('home', ['a' => '", false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(tt.code)
			var view *ViewContext
			if ok {
				view, ok = ViewNameArg(arg)
			}
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (view.Partial != tt.wantPart || view.Quote != "'") {
				t.Fatalf("view = %#v, want partial %q", view, tt.wantPart)
			}
		})
	}
}
