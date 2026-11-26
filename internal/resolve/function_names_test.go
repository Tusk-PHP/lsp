package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestFunctionNames(t *testing.T) {
	src := `<?php
namespace App;

use Vendor\Lib;
use function Other\helper;
`
	file := parser.ParseFile(src)
	cases := map[string]string{
		`\Uri\WhatWg\url_percent_encode`: `Uri\WhatWg\url_percent_encode`,
		`Lib\run`:                        `Vendor\Lib\run`,
		`Sub\run`:                        `App\Sub\run`,
		`helper`:                         `Other\helper`,
		`HELPER`:                         `Other\helper`,
		`lib\run`:                        `Vendor\Lib\run`,
		`strlen`:                         `App\strlen Global fallback`,
	}
	for name, want := range cases {
		got, ok := FunctionNames(name, file.NamespaceAt(5, 0))
		if !ok {
			t.Fatalf("%s: not resolved", name)
		}
		if name == "strlen" {
			if strings.Join(got, " ") != `App\strlen strlen` {
				t.Errorf("%s: got %v, want the namespaced name then the global one", name, got)
			}
			continue
		}
		if strings.Join(got, " ") != want {
			t.Errorf("%s: got %v, want %s", name, got, want)
		}
	}
}

func TestFunctionNamesNeedAFileAndAName(t *testing.T) {
	if _, ok := FunctionNames("f", nil); ok {
		t.Error("no namespace, no names")
	}
	if _, ok := FunctionNames(" \\", parser.ParseFile("<?php\n").NamespaceAt(0, 0)); ok {
		t.Error("an empty name has no names")
	}
}

func TestImportedName(t *testing.T) {
	uses := []parser.UseNode{{FullName: `Lib\Helpers`, Alias: "Helpers", Kind: "class"}, {FullName: `Lib\run`, Alias: "run", Kind: "function"}}
	for _, c := range []struct{ name, kind, want string }{
		{"", "class", ""},
		{`\Full\Name`, "class", `Full\Name`},
		{`Helpers\go`, "class", `Lib\Helpers\go`},
		{`helpers\go`, "class", `Lib\Helpers\go`},
		{"HELPERS", "class", `Lib\Helpers`},
		{"run", "class", `App\run`},
		{"run", "", `Lib\run`},
	} {
		if got := ImportedName(c.name, "App", uses, c.kind); got != c.want {
			t.Errorf("ImportedName(%q, %q) = %q, want %q", c.name, c.kind, got, c.want)
		}
	}
	if got := ImportedName("run", "", nil, "class"); got != "run" {
		t.Errorf("in the global namespace a name stays as written, got %q", got)
	}
}

func TestCalledFunctionFindsOnlyAFunctionByPHPsRules(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///room.php", "<?php\nnamespace Other;\nclass Room { public function area() {} }\n")
	idx.IndexFile("file:///f.php", "<?php\nnamespace App;\nfunction local() {}\n")
	idx.IndexFile("file:///g.php", "<?php\nfunction helper() {}\n")
	r := NewResolver(idx)
	file := parser.ParseFile("<?php\nnamespace App;\n")
	for name, want := range map[string]string{"local": `App\local`, "HELPER": "helper", `\helper`: "helper"} {
		if fn := r.CalledFunction(name, file.NamespaceAt(2, 0)); fn == nil || fn.FQN != want {
			t.Errorf("CalledFunction(%q) = %+v, want %s", name, fn, want)
		}
	}
	if fn := r.CalledFunction("area", file.NamespaceAt(2, 0)); fn != nil {
		t.Errorf("a method is never a called function, got %+v", fn)
	}
}
