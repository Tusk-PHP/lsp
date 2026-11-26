package symfony

import (
	"strings"
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
)

func callArgAt(t *testing.T, code string) (sourcectx.CallArg, bool) {
	t.Helper()
	source := "<?php\n" + code
	lines := strings.Split(source, "\n")
	pos := protocol.Position{Line: len(lines) - 1, Character: len(lines[len(lines)-1])}
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
		wantOk    bool
		wantPart  string
		wantQuote string
	}{
		{name: "generateUrl single quote", code: "$this->generateUrl('product_", wantOk: true, wantPart: "product_", wantQuote: "'"},
		{name: "generate no quote yet", code: "$router->generate(prod", wantOk: true, wantPart: "prod"},
		{name: "redirectToRoute on the next line", code: "$this->redirectToRoute(\n    'pro", wantOk: true, wantPart: "pro", wantQuote: "'"},
		{name: "second arg should not match", code: "$this->redirectToRoute('product_show', "},
		{name: "static generate is not the router", code: "Foo::generate('pro"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			ok = ok && IsRouteNameArg(arg)
			if ok != tt.wantOk {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOk)
			}
			if ok && (arg.Partial != tt.wantPart || arg.Quote != tt.wantQuote) {
				t.Fatalf("partial %q quote %q, want %q %q", arg.Partial, arg.Quote, tt.wantPart, tt.wantQuote)
			}
		})
	}
}

func TestIsCommandNameArg(t *testing.T) {
	tests := []struct {
		name string
		code string
		want bool
	}{
		{"find", "$app->find('cache:", true},
		{"has before a quote", "$this->application->has(", true},
		{"get on the next line", "$app\n    ->get('ca", true},
		{"escaped name", `$app->find('a\`, false},
		{"variable in the name", `$app->find("a$`, false},
		{"second argument", "$app->get('a', ", false},
		{"static", "App::find('a", false},
		{"other method", "$app->run('a", false},
		{"a bare name", "$app->find(cache", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			if got := ok && IsCommandNameArg(arg); got != tt.want {
				t.Fatalf("IsCommandNameArg = %v, want %v (%#v)", got, tt.want, arg)
			}
		})
	}
}
