package parser

import (
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"testing"
)

func TestResolveImportedName(t *testing.T) {
	file := ParseFile(`<?php
namespace App;
use Vendor\Lib\Thing, Vendor\Lib as L;
use function Vendor\helper as Thingy;
`, token.Latest)
	for name, want := range map[string]string{
		"Thing":    `Vendor\Lib\Thing`,
		"L\\Sub":   `Vendor\Lib\Sub`,
		"\\Global": "Global",
		"Local":    `App\Local`,
		"Thingy":   `App\Thingy`,
		"":         "",
	} {
		if got := file.ResolveClassName(name); got != want {
			t.Errorf("%q: %q, want %q", name, got, want)
		}
	}
	if got := ResolveImportedName("Thingy", "", file.Uses, "function"); got != `Vendor\helper` {
		t.Errorf("function import: %q", got)
	}
	if got := ResolveImportedName("Plain", "", nil, "class"); got != "Plain" {
		t.Errorf("global code: %q", got)
	}
}

func TestResolveImportedNameMatchesAliasesIgnoringCase(t *testing.T) {
	uses := []UseNode{{FullName: `Lib\Helpers`, Alias: "Helpers", Kind: "class"}, {FullName: `Lib\run`, Alias: "run", Kind: "function"}}
	for _, c := range []struct{ name, kind, want string }{
		{`helpers\go`, "class", `Lib\Helpers\go`},
		{"HELPERS", "class", `Lib\Helpers`},
		{"RUN", "function", `Lib\run`},
		{"run", "class", `App\run`},
	} {
		if got := ResolveImportedName(c.name, "App", uses, c.kind); got != c.want {
			t.Errorf("ResolveImportedName(%q, %q) = %q, want %q", c.name, c.kind, got, c.want)
		}
	}
}

func TestStringLiteral(t *testing.T) {
	for in, want := range map[string]string{`'a'`: "a", `"b"`: "b", `''`: ""} {
		if got, ok := StringLiteral(in); !ok || got != want {
			t.Errorf("%s: %q %v", in, got, ok)
		}
	}
	for _, in := range []string{"", "'", `'a"`, "abc", "1"} {
		if _, ok := StringLiteral(in); ok {
			t.Errorf("%q is not a string literal", in)
		}
	}
}

func TestStringLiteralEscapes(t *testing.T) {
	for in, want := range map[string]string{
		`'App\\Foo'`:                         `App\Foo`,
		`'it\'s \n'`:                         `it's \n`,
		`"App\\Foo"`:                         `App\Foo`,
		`"a\tb\$c\"d\e\f\v"`:                 "a\tb$c\"d\x1b\f\v",
		`"\101\x41\x4\u{1F600}\q\u{zz}\u{1"`: "AA\x04😀\\q\\u{zz}\\u{1",
		`"end\"`:                             `end\`,
	} {
		if got, ok := StringLiteral(in); !ok || got != want {
			t.Errorf("%s: %q, want %q", in, got, want)
		}
	}
}

func TestDeclaredClasses(t *testing.T) {
	src := "<?php\nnamespace A;\nclass One {}\nnamespace B { class Two {} interface I {} }\nnamespace { class Three {} }\n"
	got := DeclaredClasses(src, ParseTree(src, token.Latest))
	for _, fqn := range []string{`A\One`, `B\Two`, "Three"} {
		if got[fqn] == nil {
			t.Errorf("%s missing from %v", fqn, got)
		}
	}
	if len(got) != 3 {
		t.Errorf("got %d classes", len(got))
	}
}
