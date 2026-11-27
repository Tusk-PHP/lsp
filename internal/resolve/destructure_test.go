package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// A variable written by a destructuring has whatever element it takes, which
// isn't known; it doesn't keep the type of an earlier write, as the foreach
// variable's before `[$name, $route] = $route;`.
func TestDestructuredVariableDoesNotKeepEarlierType(t *testing.T) {
	src := `<?php
namespace App;
class Route { public function compile(): void {} }
class Prefix {}
class Holder {
    /** @param array<Prefix> $routes */
    public function run(array $routes): void {
        foreach ($routes as $route) {
            echo $route; // before
            [$name, $route] = $route;
            echo $route; // destructured
        }
        foreach ($routes as $other) {
            list($a, $other) = $other;
            echo $other; // list
        }
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
		{"$route", "before", `App\Prefix`},
		{"$route", "destructured", ``},
		{"$other", "list", ``},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

func TestDestructuredVariablesTakeTheElementType(t *testing.T) {
	src := `<?php
namespace App;
class User {}
class Team {}
class UsageMaintenance {
    /** @return array{0: \DateTimeImmutable, 1: \DateTimeImmutable} */
    private function utcWindowForLocalDay(): array { return []; }
    /** @return array{user: User, team: Team} */
    private function pair(): array { return []; }
    /** @return list<array{User, Team}> */
    private function rows(): array { return []; }
    /** @return array<string, User> */
    private function byName(): array { return []; }
    /** @return list<string> */
    private function split(string $separator, string $string): array { return []; }
    public function run(string $id, string $period): void {
        [$windowStart, $windowEnd] = $this->utcWindowForLocalDay();
        echo $windowEnd; // shape by position
        $parts = $this->split('//', $id);
        list( $theme, $template_id ) = $parts;
        echo $template_id; // list of strings
        ['team' => $team, 'user' => $user] = $this->pair();
        echo $team; // shape by key
        [, $second] = $this->utcWindowForLocalDay();
        echo $second; // skipped item
        [[$firstUser, $firstTeam]] = $this->rows();
        echo $firstTeam; // nested
        [$named] = $this->byName();
        echo $named; // array value type
        foreach ($this->rows() as [$rowUser, $rowTeam]) {
            echo $rowTeam; // foreach destructuring
        }
        foreach ($this->rows() as $i => list($keyedUser)) {
            echo $keyedUser; // foreach with a key and list()
        }
        [$startDate, $endDate] = match ($period) {
            'year' => [$this->format('a'), 'b'],
            'month' => ['c', 'd'],
            default => throw new \InvalidArgumentException(),
        };
        echo $endDate; // destructured match
        /** @var Team $docTeam */
        [$docTeam] = $this->byName();
        echo $docTeam; // named @var
        ["user" => $quoted] = $this->pair();
        echo $quoted; // double-quoted key
        $queue = $this->rows();
        while ($queue) {
            [$head, $queue] = $queue;
        }
        echo $head; // destructuring its own right side
        [$unknownA, $unknownB] = $missing;
        echo $unknownB; // unknown right side
    }
    private function format(string $s): string { return $s; }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	at := func(v, marker string) (string, string) {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				pos := textpos.Position{Line: i, Character: strings.Index(l, v) + 2}
				return r.ResolveVariableTypeTyped(v, lines, pos, file).FullString(), r.ResolveVariableType(v, lines, pos, file)
			}
		}
		t.Fatalf("no line marked %q", marker)
		return "", ""
	}
	for _, c := range []struct{ v, marker, want, plain string }{
		{"$windowEnd", "shape by position", `DateTimeImmutable`, `DateTimeImmutable`},
		{"$template_id", "list of strings", `string`, `string`},
		{"$team", "shape by key", `App\Team`, `App\Team`},
		{"$second", "skipped item", `DateTimeImmutable`, `DateTimeImmutable`},
		{"$firstTeam", "nested", `App\Team`, `App\Team`},
		{"$named", "array value type", `App\User`, `App\User`},
		{"$rowTeam", "foreach destructuring", `App\Team`, `App\Team`},
		{"$keyedUser", "foreach with a key and list()", `App\User`, `App\User`},
		{"$endDate", "destructured match", `string`, `string`},
		{"$docTeam", "named @var", `App\Team`, `App\Team`},
		{"$quoted", "double-quoted key", `App\User`, `App\User`},
		{"$head", "destructuring its own right side", ``, ``},
		{"$unknownB", "unknown right side", ``, ``},
	} {
		got, plain := at(c.v, c.marker)
		if got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
		if plain != c.plain {
			t.Errorf("plain %s (%s) = %q, want %q", c.v, c.marker, plain, c.plain)
		}
	}
}
