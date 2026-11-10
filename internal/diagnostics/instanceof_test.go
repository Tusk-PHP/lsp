package diagnostics

import (
	"strings"
	"testing"
)

// Members are checked against what an instanceof test says a variable is
// where the test holds, through the same narrowing hover and completion use.
func TestUnknownMemberAfterInstanceof(t *testing.T) {
	got := analyzeUnknowns(t, `<?php
namespace App;

class Post { public function publish(): void {} }
class Thread { public function lock(): void {} }
class Foo { public function fooOnly(): int { return 1; } }
class Bar {}

class Cases
{
    public function run(Post $v, Foo|Bar $x, object $o, Foo|Bar $y): void
    {
        if ($v instanceof Thread) {
            $v->lock();
            $v->publish();
            $v->missing();
        }
        if ($v instanceof Gone) {
            $v->anything();
        }
        if ($x instanceof Foo) { $x->fooOnly(); }
        if ($o instanceof Foo) { $o->fooOnly(); }
        if ($x instanceof Foo && $x->fooOnly() > 0) {}
        if (!$y instanceof Foo) {
            return;
        }
        $y->fooOnly();
        $x->nope();
    }
}
`)
	want := []string{
		"Unknown class 'Gone'",
		"Unknown member 'missing' on 'Post|Thread'",
		"Unknown member 'nope' on 'Foo|Bar'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
