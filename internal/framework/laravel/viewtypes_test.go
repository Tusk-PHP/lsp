package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestViewVariableTypesKeepUnionsInsideGenericsAndParentheses(t *testing.T) {
	passed := map[string]string{
		"$a": "array<int|string, User>|null",
		"$b": "Collection<int, A|B>",
		"$c": "(A&B)|null",
		"$d": "array<int, Collection<int, A|B>>",
	}
	const controller = "file:///app/app/Http/Controllers/HomeController.php"
	const src = "<?php\nreturn view('home', ['a' => $a, 'b' => $b, 'c' => $c, 'd' => $d]);"
	d := NewViewData("/app")
	d.IndexFile(controller, src)
	vt := &ViewTypes{
		Data: d,
		Read: func(uri string) string {
			if uri == controller {
				return src
			}
			return ""
		},
		Resolve: func(expr, _ string, _ *parser.FileNode, _, _ int) string {
			return passed[expr]
		},
	}
	want := map[string]string{
		"a": "array<int | string, User>|null",
		"b": "Collection<int, A | B>",
		"c": "(A & B)|null",
		"d": "array<int, Collection<int, A | B>>",
	}
	got := map[string]string{}
	for _, v := range vt.For("file:///app/resources/views/home.blade.php").Vars {
		got[v.Name] = v.Type
	}
	for name, w := range want {
		if got[name] != w {
			t.Errorf("$%s passed as %q: type %q, want %q", name, passed["$"+name], got[name], w)
		}
	}
}

func TestQualifyResolvesOnlyTheNamesOfAUnion(t *testing.T) {
	const uri = "file:///app/app/View/Components/Card.php"
	vt := &ViewTypes{Read: func(string) string {
		return "<?php\nnamespace App\\View\\Components;\nuse App\\Models\\User;\n"
	}}
	for typ, want := range map[string]string{
		"User|null":                    `App\Models\User|null`,
		"?User":                        `?App\Models\User`,
		`\Other\Thing|Local`:           `Other\Thing|App\View\Components\Local`,
		"array<int|string, User>|User": `array<int | string, User>|App\Models\User`,
		"(User&Countable)|null":        "(User & Countable)|null",
	} {
		st := &typingState{visiting: map[string]bool{}, files: map[string]siteFile{}}
		if got := vt.qualify(typ, uri, st); got != want {
			t.Errorf("qualify(%q) = %q, want %q", typ, got, want)
		}
	}
}
