package checks

import (
	"strings"
	"testing"
)

func TestUnknownMemberExistenceGuards(t *testing.T) {
	decl := `<?php
namespace App;
class Note { public string $title = ''; public $n; public function get() {} }
class Other {}
`
	vars := map[string]string{
		"$v":       `App\Note`,
		"$w":       `App\Note`,
		"$this":    `App\Note`,
		"$this->n": `App\Note`,
		"static":   `App\Note`,
		"self":     `App\Note`,
	}
	cases := []struct {
		name string
		body string
		want []string
	}{
		{"method_exists in an if", "if (method_exists($v, 'toArray')) {\nreturn $v->toArray(1);\n}", nil},
		{"early return", "if (! method_exists($v, 'toArray')) {\nreturn;\n}\nreturn $v->toArray();", nil},
		{"ternary", "return method_exists($v, 'toArray') ? $v->toArray() : null;", nil},
		{"and chain", "return \\method_exists($v, 'toArray') && $v->toArray();", nil},
		{"spaced arguments", "if (method_exists( $v , \"toArray\" )) { $v->toArray(); }", nil},
		{"is_callable", "if (is_callable([$v, 'toArray'])) { $v->toArray(); }", nil},
		{"property_exists", "if (property_exists($v, 'extra')) { return $v->extra; }", nil},
		{"on $this", "if (method_exists($this, 'toArray')) { $this->toArray(); }", nil},
		{"on a property of $this", "if (method_exists($this->n, 'failed')) {\n$this->n->failed();\n}", nil},
		{"nullsafe access", "if (method_exists($v, 'toArray')) { $v?->toArray(); }", nil},
		{"static::class", "if (method_exists(static::class, 'boot')) { static::boot(); }", nil},
		{"$x::class", "if (method_exists($v::class, 'toArray')) { $v->toArray(); }", nil},
		{"method names ignore case", "if (method_exists($v, 'TOARRAY')) { $v->toArray(); }", nil},
		{"guard in a closure of the same method", "$f = function () use ($v) { return method_exists($v, 'toArray'); };\nif ($f()) { $v->toArray(); }", nil},

		{"property names don't ignore case", "if (property_exists($v, 'Extra')) { return $v->extra; }", []string{"Unknown member 'extra' on 'Note'"}},
		{"a different member", "if (method_exists($v, 'toArray')) { $v->toMail(); }", []string{"Unknown member 'toMail' on 'Note'"}},
		{"a different receiver", "if (method_exists($v, 'toArray')) { $w->toArray(); }", []string{"Unknown member 'toArray' on 'Note'"}},
		{"a variable that only starts the same", "if (method_exists($v, 'toArray')) { $vv->toArray(); $w->toArray(); }", []string{"Unknown member 'toArray' on 'Note'"}},
		{"before the guard", "$v->toArray();\nif (method_exists($v, 'toArray')) {}", []string{"Unknown member 'toArray' on 'Note'"}},
		{"method_exists doesn't cover properties", "if (method_exists($v, 'extra')) { return $v->extra; }", []string{"Unknown member 'extra' on 'Note'"}},
		{"property_exists doesn't cover calls", "if (property_exists($v, 'toArray')) { $v->toArray(); }", []string{"Unknown member 'toArray' on 'Note'"}},
		{"a name in a variable", "if (method_exists($v, $m)) { $v->toArray(); }", []string{"Unknown member 'toArray' on 'Note'"}},
		{"a computed receiver", "if (method_exists($v->get(), 'toArray')) { $v->toArray(); }", []string{"Unknown member 'toArray' on 'Note'"}},
		{"in a comment", "// method_exists($v, 'toArray')\n$v->toArray();", []string{"Unknown member 'toArray' on 'Note'"}},
		{"in a string", "$s = \"method_exists($v, 'toArray')\";\n$v->toArray();", []string{"Unknown member 'toArray' on 'Note'"}},
		{"a method of that name", "if ($v->method_exists($v, 'toArray')) { $v->toArray(); }", []string{"Unknown member 'method_exists' on 'Note'", "Unknown member 'toArray' on 'Note'"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := "<?php\nnamespace App\\Http;\nclass Channel {\npublic function send() {\n" + tc.body + "\n}\n}\n"
			got := unknownMemberMessages(checkUnknownMembers(t, vars, decl, source))
			if strings.Join(got, ",") != strings.Join(tc.want, ",") {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

// A guard answers only for the function or method it is in.
func TestUnknownMemberGuardStaysInItsMethod(t *testing.T) {
	decl := "<?php\nnamespace App;\nclass Note {}\n"
	source := `<?php
namespace App\Http;
class Channel {
    public function guarded($v) {
        if (method_exists($v, 'toArray')) {
            return $v->toArray();
        }
    }
    public function unguarded($v) {
        return $v->toArray();
    }
}
function helper($v) {
    if (method_exists($v, 'toMail')) { $v->toMail(); }
}
function other($v) {
    $v->toMail();
}
`
	got := checkUnknownMembers(t, map[string]string{"$v": `App\Note`}, decl, source)
	want := []string{"Unknown member 'toArray' on 'Note'", "Unknown member 'toMail' on 'Note'"}
	if strings.Join(unknownMemberMessages(got), ",") != strings.Join(want, ",") {
		t.Fatalf("got %v, want %v", unknownMemberMessages(got), want)
	}
	if got[0].StartLine != 9 || got[1].StartLine != 16 {
		t.Errorf("want findings on lines 9 and 16, got %d and %d", got[0].StartLine, got[1].StartLine)
	}
}

func TestUnknownMemberOwnersListedOnce(t *testing.T) {
	decl := `<?php
namespace App;
class Foo { public function fooOnly() {} }
class Bar {}
`
	source := `<?php
namespace App;
class C {
    public function f(Foo|Bar $x, object $o): void {
        if ($x instanceof Foo) { $x->fooOnly(); }
        if ($o instanceof Foo) { $o->fooOnly(); }
        if ($x instanceof Foo && $x instanceof Bar) {}
        $x->nope();
    }
}
`
	got := unknownMemberMessages(checkUnknownMembers(t, map[string]string{"$x": `App\Foo|App\Bar`, "$o": `App\Bar`}, decl, source))
	want := []string{"Unknown member 'nope' on 'Foo|Bar'"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("got %v, want %v", got, want)
	}
}
