package checks

import (
	"strconv"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A method a framework calls by convention isn't reported on the class that
// leaves it to its subclasses, or on those subclasses.
func TestUnknownMemberConventionMethod(t *testing.T) {
	base := `<?php
namespace Fw;
class Notification { public function locale($l) {} }
`
	sub := `<?php
namespace App;
class Invoice extends \Fw\Notification {}
`
	convention := func(class, method string) bool {
		return class == `Fw\Notification` && strings.EqualFold(method, "toMail")
	}
	cases := []struct {
		name, typ, use string
		want           []string
	}{
		{"on the base class", `Fw\Notification`, `$v->toMail($n);`, nil},
		{"on a subclass", `App\Invoice`, `$v->toMail($n);`, nil},
		{"another method is still checked", `Fw\Notification`, `$v->toSms($n);`, []string{"Unknown member 'toSms' on 'Notification'"}},
		{"a property of the same name is still checked", `Fw\Notification`, `$v->toMail;`, []string{"Unknown member 'toMail' on 'Notification'"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			idx := symbols.NewIndex()
			for i, src := range []string{base, sub} {
				idx.IndexFileWithSource("file:///c"+strconv.Itoa(i)+".php", src, symbols.SourceProject)
			}
			source := "<?php\nnamespace App\\Http;\n" + tc.use + "\n"
			checker := &UnknownMemberRule{
				TypeResolver: func(expr, _ string, _, _ int, _ *parser.FileNode) string {
					return map[string]string{"$v": tc.typ}[expr]
				},
				ConventionMethod: convention,
			}
			got := unknownMemberMessages(checker.Check(parser.ParseFile(source), source, idx))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
