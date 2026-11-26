package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestAssignmentsAreTypedFromTheSyntaxTree(t *testing.T) {
	src := `<?php
namespace App;
class User { public function name(): string { return ''; } }
class Query {
    public function first(): User { return new User(); }
    public function where(string $sql, string $op = '='): Query { return $this; }
}
class Repo {
    public function find(string $q): Query { return new Query(); }
    /** @return Query */
    public function each(callable $f) { return new Query(); }
}
/** @template T */
class Box {
    /** @param T $value */
    public function __construct($value, string $label = '') {}
}
class Holder {
    public function run(Repo $repo, User $user): void {
        $paren = $repo->find(
            ')',
        )->first();
        echo $paren; // paren in a string
        $comma = new Box(/* , */ $user);
        echo $comma; // comma in a comment
        $commented = $repo
            // the first match
            ->find('x')
            ->first();
        echo $commented; // comment in a chain
        $heredoc = $repo->find(<<<SQL
            name = ')'
            SQL)->first();
        echo $heredoc; // heredoc argument
        $nested = $repo->each(function ($a) {
            $inner = function ($b) {
                return fn ($c) => ')';
            };
            return $inner;
        })->first();
        echo $nested; // nested closures
        $piped = $user |> function ($u) use ($repo): Query { return $repo->find(''); };
        echo $piped; // closure with use in a pipe
        $event = $repo;
        $kept = $event = $repo->find('x');
        echo $event; // chained assignment
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
		{"$paren", "paren in a string", `App\User`},
		{"$comma", "comma in a comment", `App\Box<App\User>`},
		{"$commented", "comment in a chain", `App\User`},
		{"$heredoc", "heredoc argument", `App\User`},
		{"$nested", "nested closures", `App\User`},
		{"$piped", "closure with use in a pipe", `App\Query`},
		{"$event", "chained assignment", `App\Query`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

func BenchmarkAssignedVariableAfterEdit(b *testing.B) {
	var sb strings.Builder
	sb.WriteString("<?php\nnamespace App;\nclass User { public function name(): string { return ''; } }\nclass Repo { public function find(string $q): User { return new User(); } }\nclass Holder {\n")
	for i := range 300 {
		sb.WriteString("    public function m" + itoa(i) + "(Repo $repo): void {\n        $user = $repo->find('a, b')\n            ->name();\n        $other = new User();\n        echo $user;\n    }\n")
	}
	sb.WriteString("    public function run(Repo $repo): void {\n        $found = $repo->find('x');\n        echo $found;\n    }\n}\n")
	src := sb.String()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	lines := SplitLines(src)
	pos := textpos.Position{Line: len(lines) - 4, Character: 15}
	edits := [2][]string{lines, append(append([]string{}, lines[:len(lines)-1]...), " ")}
	files := [2]*parser.FileNode{parser.ParseFile(src, token.Latest), parser.ParseFile(strings.Join(edits[1], "\n"), token.Latest)}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		if got := r.ResolveVariableTypeTyped("$found", edits[i%2], pos, files[i%2]).FQN; got != `App\User` {
			b.Fatalf("$found = %q", got)
		}
	}
}
