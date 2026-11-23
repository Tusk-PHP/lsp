package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A member call with a comment or space after `->` or `::` calls a member,
// not a function.
func TestUnknownFunctionSkipsMemberCallAfterComment(t *testing.T) {
	src := `<?php
$f->/* note */bindTo(null);
$f-> rebind(null);
Foo:: /* x */ make();
missing_function();
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	var got []string
	for _, f := range (&UnknownFunctionRule{}).Check(parser.ParseFile(src), src, idx) {
		got = append(got, f.Message)
	}
	if want := "Unknown function 'missing_function'"; strings.Join(got, ",") != want {
		t.Errorf("got %v, want [%s]", got, want)
	}
}

// The receiver of a member after a nullsafe link is the whole chain, so the
// member is checked on the class the chain gives.
func TestUnknownMemberNullsafeChainReceiver(t *testing.T) {
	post := `<?php
namespace App;
class Post { public function publish(): void {} }
`
	cases := []struct {
		name, use string
		want      []string
	}{
		{"after a nullsafe link", `$a?->post->missing();`, []string{"Unknown member 'missing' on 'Post'"}},
		{"member that exists", `$a?->post->publish();`, nil},
		{"ternary before a variable", `$c ? $a->post->missing() : null;`, []string{"Unknown member 'missing' on 'Post'"}},
	}
	vars := map[string]string{`$a?->post`: `App\Post`, `$a->post`: `App\Post`}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "<?php\nnamespace App\\Http;\n" + tc.use + "\n"
			got := unknownMemberMessages(checkUnknownMembers(t, vars, post, source))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
