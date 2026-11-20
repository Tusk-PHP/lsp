package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func undefinedIn(src string) []string {
	var got []string
	for _, f := range (&UndefinedVariableRule{}).Check(parser.ParseFile(src), src, nil) {
		got = append(got, f.Message)
	}
	return got
}

func TestPlainViewTopLevelVariablesComeFromTheIncluder(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		// Laravel's console components: the file starts as HTML.
		{"starts with html", `<div class="mt-<?php echo $marginTop ?>"><?php echo $title ?></div>`, nil},
		// WordPress themes: PHP first, then HTML after a close tag.
		{"html after close tag", "<?php\nget_header(); ?>\n<p><?php echo $post->ID; ?></p>\n", nil},
		// Functions and closures in a view are still checked.
		{"function in a view", "<?php function f() { return $missing; } ?>\n<p><?php echo $title ?></p>", []string{"Undefined variable $missing"}},
		// A script with no output isn't a view.
		{"php only", "<?php\necho $missing;\n", []string{"Undefined variable $missing"}},
		{"close tag then only whitespace", "<?php\necho $missing;\n?>\n", []string{"Undefined variable $missing"}},
		// A class file that ends with some HTML isn't a view either.
		{"class with html", "<?php\nclass A {}\necho $missing;\n?>\n<p>x</p>", []string{"Undefined variable $missing"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := undefinedIn(c.src); strings.Join(got, "|") != strings.Join(c.want, "|") {
				t.Errorf("got %q, want %q", got, c.want)
			}
		})
	}
}
