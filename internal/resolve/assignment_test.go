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
        $elvis = $repo->find('x') ?: $user;
        echo $elvis; // elvis
        $coalesced = $repo->find('x') ?? $user;
        echo $coalesced; // coalesce
        $fromNull = null ?? $repo->find('x');
        echo $fromNull; // coalesce from null
        $ternary = $user ? $user : null;
        echo $ternary; // ternary with null
        $unknown = $missing ?? $user;
        echo $unknown; // coalesce with an unknown side
    }
}
`
	at, _ := assignedTypes(t, src)
	for _, c := range []struct{ v, marker, want string }{
		{"$paren", "paren in a string", `App\User`},
		{"$comma", "comma in a comment", `App\Box<App\User>`},
		{"$commented", "comment in a chain", `App\User`},
		{"$heredoc", "heredoc argument", `App\User`},
		{"$nested", "nested closures", `App\User`},
		{"$piped", "closure with use in a pipe", `App\Query`},
		{"$event", "chained assignment", `App\Query`},
		{"$elvis", "elvis", `App\Query|App\User`},
		{"$coalesced", "coalesce", `App\Query|App\User`},
		{"$fromNull", "coalesce from null", `App\Query`},
		{"$ternary", "ternary with null", `?App\User`},
		{"$unknown", "coalesce with an unknown side", ``},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

// assignedTypes indexes src and returns the typed and the plain type of a
// variable read on the line that ends with `// marker`.
func assignedTypes(t *testing.T, src string) (typed, plain func(v, marker string) string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	read := func(v, marker string) textpos.Position {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				return textpos.Position{Line: i, Character: strings.Index(l, v) + 2}
			}
		}
		t.Fatalf("no line marked %q", marker)
		return textpos.Position{}
	}
	typed = func(v, marker string) string {
		return r.ResolveVariableTypeTyped(v, lines, read(v, marker), file).FullString()
	}
	plain = func(v, marker string) string {
		return r.ResolveVariableType(v, lines, read(v, marker), file)
	}
	return typed, plain
}

func TestAssignmentValueForms(t *testing.T) {
	src := `<?php
namespace App;
class User { public ?User $friend = null; public static ?User $current = null; const LABEL = 'user'; public function name(): string { return ''; } }
class Query { public function first(): User { return new User(); } public static function make(): Query { return new Query(); } }
class Repo {
    public Query $query;
    public function find(string $q = ''): Query { return new Query(); }
    /** @return list<User> */
    public function all(): array { return []; }
}
/**
 * @template TKey
 * @template TValue
 */
class Collection {
    /** @param array<TKey, TValue> $items */
    public function __construct($items = []) {}
}
/** @template T */
class Factory {
    /** @param class-string<T> $class */
    public function __construct(string $class) {}
}
/**
 * @template TKey
 * @template TValue
 */
class Bag {
    public function __construct(array $items) {}
}
/**
 * @template TKey
 * @template TValue
 */
class Pouch {}
/** @template T of object */
class Box {
    /** @param class-string<T>|T $cls */
    public function __construct(string|object $cls) {}
}
function helper(): User { return new User(); }
function pair(User $a, User $b): User { return $a; }
function describe(User $u): Query { return new Query(); }
class Holder {
    public function defaults($made = new User(), $limit = NEWLINE_LIMIT): void {
        echo $made; // parameter created by its default
        echo $limit; // parameter with a constant default
    }
    /** @return \Generator<int, int, User, void> */
    public function gen(): \Generator {
        $sent = yield 1;
        echo $sent; // yield
    }
    /** @param list<User> $users */
    public function run(Repo $repo, User $user, array $users, bool $c, $fn): void {
        $invoked = $user |> $repo->find(...);
        echo $invoked; // pipe to a first-class callable
        $staticInvoked = $user |> Query::make(...);
        echo $staticInvoked; // pipe to a static first-class callable
        $arrow = $user |> fn ($u): Query => $repo->find('');
        echo $arrow; // pipe to a typed arrow function
        $arrowBody = $user |> fn ($u) => new Query();
        echo $arrowBody; // pipe to an arrow function
        $copy = [...$users];
        echo $copy; // spread of one variable
        $mixed = [...$users, ...$repo->all()];
        echo $mixed; // spread of a call
        $empty = [];
        echo $empty; // empty array
        $anon = new class {};
        echo $anon; // anonymous class
        $cls = User::class;
        $dynamic = new $cls();
        echo $dynamic; // new from a class-string variable
        $made = new Factory(User::class);
        echo $made; // class-string constructor argument
        $collected = new Collection([new User()]);
        echo $collected; // array constructor argument
        $named = new Collection(items: [new User()]);
        echo $named; // named constructor argument
        $orThrow = $user ?? throw new \Exception();
        echo $orThrow; // coalesce or throw
        $elvisThrow = $user ?: throw new \Exception();
        echo $elvisThrow; // elvis or throw
        $cloned = clone($user, ['friend' => null]);
        echo $cloned; // clone with
        static $cached = new User(), $other = new Query();
        echo $cached; // first static
        echo $other; // second static
        $friendOf = $repo->all()[0];
        echo $friendOf; // offset of a call
        $friend = $user->friend->name();
        echo $friend; // property chain
        $pick = $c ? $users[0] : new Query();
        echo $pick; // ternary of an offset and new
        $flag = $c ? true : null;
        echo $flag; // ternary of constants
        $literal = $c ? 'a' : 1;
        echo $literal; // ternary of literals
        $call = $c ? helper() : $fn();
        echo $call; // ternary of calls
        $member = $c ? $user->$fn : User::$current;
        echo $member; // ternary of a dynamic property
        $static = $c ? User::$current : Query::make();
        echo $static; // ternary of static members
        $const = $c ? User::LABEL : null;
        echo $const; // ternary of a class constant
        $function = helper();
        echo $function; // function call
        $closure = helper(...);
        echo $closure; // first-class callable function
        $selfMade = new static();
        echo $selfMade; // new static
        $boxed = new Box(User::class);
        echo $boxed; // class-string or object constructor argument
        $bagged = new Bag([new User()]);
        echo $bagged; // untyped array constructor argument
        $pouched = new Pouch([new User()]);
        echo $pouched; // array argument without a constructor
        $writeOnly = $users[];
        echo $writeOnly; // offset without a key
        $unknownSide = $c ? $missing : $user;
        echo $unknownSide; // ternary with an unknown side
        $pipedValue = $user |> $fn;
        echo $pipedValue; // pipe to a variable
        $partial = pair($user, ?);
        echo $partial; // partial application
        $restPartial = pair($user, ...);
        echo $restPartial; // partial application of the rest
        $quoted = new Box('App\\User');
        echo $quoted; // quoted class name constructor argument
        $pipedFunction = $user |> describe(...);
        echo $pipedFunction; // pipe to a first-class callable function
        $anonSide = $c ? new class {} : $user;
        echo $anonSide; // ternary with an anonymous class
        $keylessSide = $c ? $users[] : $user;
        echo $keylessSide; // ternary with an offset without a key
        $cb = function () {
            $inner = new User();
        };
        echo $inner; // assigned only in a closure
        $cast = (int) $fn;
        echo $cast; // cast
        $long = array($fn);
        echo $long; // long array of an unknown value
        $className = $fn::class;
        echo $className; // class of a value
        $named = User::class;
        echo $named; // class name
        $tooDeep = $c ? $users[0][0][0][0][0][0][0][0][0][0][0][0][0] : null;
        echo $tooDeep; // offsets past the depth limit
    }
}
`
	typed, plain := assignedTypes(t, src)
	for _, c := range []struct{ v, marker, want string }{
		{"$sent", "yield", `App\User`},
		{"$invoked", "pipe to a first-class callable", `App\Query`},
		{"$staticInvoked", "pipe to a static first-class callable", `App\Query`},
		{"$arrow", "pipe to a typed arrow function", `App\Query`},
		{"$arrowBody", "pipe to an arrow function", `App\Query`},
		{"$copy", "spread of one variable", `list<App\User>`},
		{"$empty", "empty array", `array`},
		{"$anon", "anonymous class", ``},
		{"$made", "class-string constructor argument", `App\Factory<App\User>`},
		{"$collected", "array constructor argument", `App\Collection<int, App\User>`},
		{"$named", "named constructor argument", `App\Collection<int, App\User>`},
		{"$orThrow", "coalesce or throw", `App\User`},
		{"$elvisThrow", "elvis or throw", `App\User`},
		{"$cloned", "clone with", `App\User`},
		{"$cached", "first static", `App\User`},
		{"$other", "second static", `App\Query`},
		{"$friendOf", "offset of a call", `App\User`},
		{"$friend", "property chain", `string`},
		{"$pick", "ternary of an offset and new", `App\User|App\Query`},
		{"$static", "ternary of static members", `App\User|App\Query|null`},
		{"$function", "function call", `App\User`},
		{"$dynamic", "new from a class-string variable", `App\User`},
		{"$flag", "ternary of constants", `?bool`},
		{"$literal", "ternary of literals", `string|int`},
		{"$call", "ternary of calls", ``},
		{"$member", "ternary of a dynamic property", ``},
		{"$const", "ternary of a class constant", ``},
		{"$boxed", "class-string or object constructor argument", `App\Box<App\User>`},
		{"$bagged", "untyped array constructor argument", `App\Bag<int, App\User>`},
		{"$pouched", "array argument without a constructor", `App\Pouch<int, App\User>`},
		{"$writeOnly", "offset without a key", ``},
		{"$unknownSide", "ternary with an unknown side", ``},
		{"$pipedValue", "pipe to a variable", ``},
		{"$partial", "partial application", `Closure(App\User): App\User`},
		{"$restPartial", "partial application of the rest", `Closure(App\User): App\User`},
		{"$tooDeep", "offsets past the depth limit", ``},
		{"$cast", "cast", `int`},
		{"$long", "long array of an unknown value", `array`},
		{"$className", "class of a value", `class-string`},
		{"$named", "class name", `class-string<App\User>`},
		{"$quoted", "quoted class name constructor argument", `App\Box<App\User>`},
		{"$pipedFunction", "pipe to a first-class callable function", `App\Query`},
		{"$mixed", "spread of a call", `array`},
		{"$anonSide", "ternary with an anonymous class", ``},
		{"$keylessSide", "ternary with an offset without a key", ``},
		{"$made", "parameter created by its default", `App\User`},
		{"$limit", "parameter with a constant default", ``},
	} {
		if got := typed(c.v, c.marker); got != c.want {
			t.Errorf("typed %s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$invoked", "pipe to a first-class callable", `App\Query`},
		{"$pick", "ternary of an offset and new", `App\User|App\Query`},
		{"$orThrow", "coalesce or throw", `App\User`},
		{"$made", "class-string constructor argument", `App\Factory`},
		{"$selfMade", "new static", `App\Holder`},
		{"$function", "function call", `App\User`},
		{"$closure", "first-class callable function", `Closure`},
		{"$inner", "assigned only in a closure", ``},
	} {
		if got := plain(c.v, c.marker); got != c.want {
			t.Errorf("plain %s (%s) = %q, want %q", c.v, c.marker, got, c.want)
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

func TestSyntaxTreeOffsetClampsToTheLine(t *testing.T) {
	src := "<?php\n$a = 1;\n$b"
	tree := newSyntaxTree(src, parser.ParseTree(src, token.Latest))
	for _, c := range []struct {
		pos  textpos.Position
		want uint32
	}{
		{textpos.Position{Line: 1, Character: 2}, 8},
		{textpos.Position{Line: 1, Character: 40}, 13},
		{textpos.Position{Line: 2, Character: 40}, uint32(len(src))},
		{textpos.Position{Line: 9}, uint32(len(src))},
	} {
		if got := tree.offset(c.pos); got != c.want {
			t.Errorf("offset(%+v) = %d, want %d", c.pos, got, c.want)
		}
	}
}

func TestAssignmentAfterAnEditTheFileNodeMissed(t *testing.T) {
	before := "<?php\nnamespace App;\nclass User {}\nfunction run(): void {\n    echo 1;\n}\n"
	after := strings.Replace(before, "    echo 1;", "    $user = new User();\n    echo $user;", 1)
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", after)
	r := NewResolver(idx)
	stale := parser.ParseFile(before, token.Latest)
	lines := SplitLines(after)
	if got := r.ResolveVariableTypeTyped("$user", lines, textpos.Position{Line: 5, Character: 11}, stale).FQN; got != `App\User` {
		t.Errorf("$user = %q, want App\\User", got)
	}
}
