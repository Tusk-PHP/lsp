package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestMatchIsTypedAsTheUnionOfItsArms(t *testing.T) {
	src := `<?php
namespace App;
class User {}
class Team {}
class Repo { public function user(): User { return new User(); } }
class Node { public function parent(): Node { return $this; } }
class StatisticsComponent {
    public string $interval = 'day';
    public function run(Repo $repo, User $user, Team $team, int $n): void {
        $interval = match ($this->interval) {
            'day' => 'P1D',
            'month' => 'P1M',
            default => throw new \InvalidArgumentException(sprintf('Interval "%s" is not supported.', $this->interval)),
        };
        echo $interval; // string arms
        $owner = match (true) {
            $n > 1 => $team,
            default => $repo->user(),
        };
        echo $owner; // class arms
        $mixed = match ($n) {
            1, 2 => $user,
            3 => null,
            default => 0,
        };
        echo $mixed; // nullable union
        $unknown = match ($n) {
            1 => $user,
            default => $missing,
        };
        echo $unknown; // an untyped arm
        $never = match ($n) {
            default => throw new \LogicException(),
        };
        echo $never; // only throws
        $state = $user;
        foreach ([1, 2] as $step) {
            $state = match ($step) {
                1 => $state,
                default => $team,
            };
        }
        echo $state; // loop write passing itself through
        $node = new Node();
        foreach ([1, 2] as $step) {
            $node = match ($step) {
                1 => $node->parent(),
                default => $node,
            };
        }
        echo $node; // loop write reading itself in an arm
        $arg = match ($n) { 1 => 1, default => 2.5 } + 1;
        echo $arg; // match as an operand
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	at := func(v, marker string) string {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				return r.ResolveVariableTypeTyped(v, lines, textpos.Position{Line: i, Character: strings.Index(l, v) + 2}, file).FullString()
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$interval", "string arms", `string`},
		{"$owner", "class arms", `App\Team|App\User`},
		{"$mixed", "nullable union", `App\User|int|null`},
		{"$unknown", "an untyped arm", ``},
		{"$never", "only throws", ``},
		{"$state", "loop write passing itself through", `App\User|App\Team`},
		{"$node", "loop write reading itself in an arm", `App\Node`},
		{"$arg", "match as an operand", `int|float`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
	for i, l := range lines {
		if strings.HasSuffix(l, "// string arms") {
			if got := r.ResolveVariableType("$interval", lines, textpos.Position{Line: i, Character: 15}, file); got != "string" {
				t.Errorf("plain type of $interval = %q, want %q", got, "string")
			}
		}
	}
}
