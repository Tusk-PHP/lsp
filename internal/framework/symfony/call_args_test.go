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

func TestTranslationKeyArg(t *testing.T) {
	tests := []struct {
		name    string
		code    string
		ok      bool
		key     string
		partial string
		domain  string
	}{
		{name: "translator trans", code: "$translator->trans('user.pro|file.title')", ok: true, key: "user.profile.title", partial: "user.pro"},
		{name: "t shorthand", code: "t('nav|.home')", ok: true, key: "nav.home", partial: "nav"},
		{name: "TranslatableMessage", code: "new TranslatableMessage('button.sa|ve')", ok: true, key: "button.save", partial: "button.sa"},
		{name: "this trans", code: "$this->trans('validators|.field.required')", ok: true, key: "validators.field.required", partial: "validators"},
		{name: "fully qualified t", code: "\\Symfony\\Component\\Translation\\t('nav|.home')", ok: true, key: "nav.home", partial: "nav"},
		{name: "qualified TranslatableMessage", code: "new \\Symfony\\Component\\Translation\\TranslatableMessage('a|')", ok: true, key: "a", partial: "a"},
		{name: "domain", code: "$translator->trans('user.na|me', [], 'admin')", ok: true, key: "user.name", partial: "user.na", domain: "admin"},
		{name: "split over lines", code: "$translator->trans(\n    'user.na|me',\n    [],\n    'admin'\n)", ok: true, key: "user.name", partial: "user.na", domain: "admin"},
		{name: "unclosed", code: "$translator->trans('user.", ok: true, key: "user.", partial: "user."},
		{name: "get is not t", code: "$container->get('app|.mailer')"},
		{name: "count is not t", code: "count('it|ems')"},
		{name: "a method named t", code: "$helper->t('la|bel')"},
		{name: "static trans", code: "Foo::trans('la|bel')"},
		{name: "another class ending in TranslatableMessage", code: "new MyTranslatableMessage('|x')"},
		{name: "cursor outside the string", code: "$trans|lator->trans('key')"},
		{name: "domain argument", code: "$translator->trans('key', [], 'ad|"},
		{name: "in a comment", code: "// t('nav|"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			arg, ok := callArgAt(t, tt.code)
			var tx *TranslationContext
			if ok {
				tx, ok = TranslationKeyArg(arg)
			}
			if ok != tt.ok {
				t.Fatalf("ok = %v, want %v (%#v)", ok, tt.ok, arg)
			}
			if ok && (tx.Key != tt.key || tx.Partial != tt.partial || tx.Domain != tt.domain) {
				t.Fatalf("got %#v, want key %q partial %q domain %q", tx, tt.key, tt.partial, tt.domain)
			}
		})
	}
}
