package laravel

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/scope"
)

const joinCallMarker = "/*call*/"

func joinedAtMarker(t *testing.T, body string, withScope bool) bool {
	t.Helper()
	source := "<?php\nfunction run($flag, $items) {\n" + body + "\n}\n"
	at := strings.Index(source, joinCallMarker)
	if at < 0 {
		t.Fatalf("no %s in %q", joinCallMarker, body)
	}
	source = source[:at] + source[at+len(joinCallMarker):]
	line := strings.Count(source[:at], "\n")
	col := at - strings.LastIndex(source[:at], "\n") - 1
	var doc *scope.Document
	if withScope {
		doc = scope.Collect(source)
	}
	return BuilderMayBeJoined(parser.Tokenize(source), doc, line, col)
}

func TestBuilderMayBeJoined(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"join earlier in the chain", `Post::query()->join('users', 'a', '=', 'b')/*call*/->where('email', 1);`, true},
		{"join later in the chain", `Post::query()/*call*/->where('email', 1)->leftJoin('users', 'a', '=', 'b');`, true},
		{"chain over lines", "Post::query()\n    ->crossJoin('users')\n    /*call*/->where('email', 1);", true},
		{"nullsafe chain", `$items?->JOINSUB($sub, 'u', 'u.id')?/*call*/->where('email', 1);`, true},
		{"qualified class", `\App\Models\Post::query()->rightJoin('users', 'a', '=', 'b')/*call*/->where('email', 1);`, true},
		{"new in parentheses", `(new Post())->newQuery()->joinLateral($sub, 'u')/*call*/->where('email', 1);`, true},
		{"function call head", `query()->join('users', 'a', '=', 'b')/*call*/->where('email', 1);`, true},
		{"subscript head", `$items[0]->join('users', 'a', '=', 'b')/*call*/->where('email', 1);`, true},
		{"static call", `Post::join('users', 'a', '=', 'b')/*call*/::where('email', 1);`, true},
		{"no join", `Post::query()->where('title', 1)/*call*/->where('email', 1);`, false},
		{"join as a function", `join(',', $items);` + "\n" + `Post::query()/*call*/->where('email', 1);`, false},
		{"join as a property", `$items->join;` + "\n" + `$items/*call*/->where('email', 1);`, false},
		{"variable joined before", "$q = Post::query();\n$q->join('users', 'a', '=', 'b');\n$q/*call*/->where('email', 1);", true},
		{"variable assigned a join", "$q = Post::query()->join('users', 'a', '=', 'b');\n$q/*call*/->where('email', 1);", true},
		{"variable joined after a where", "$q = Post::query();\n$q->where('title', 1)->leftJoinWhere('users', 'a', '=', 'b');\n$q/*call*/->where('email', 1);", true},
		{"variable reassigned after the join", "$q = Post::query()->join('users', 'a', '=', 'b');\n$q = Post::query();\n$q/*call*/->where('email', 1);", false},
		{"variable maybe reassigned after the join", "$q = Post::query()->join('users', 'a', '=', 'b');\nif ($flag) {\n    $q = Post::query();\n}\n$q/*call*/->where('email', 1);", true},
		{"variable joined after the call", "$q = Post::query();\n$q/*call*/->where('email', 1);\n$q->join('users', 'a', '=', 'b');", false},
		{"another variable joined", "$q = Post::query();\n$r = Post::query()->join('users', 'a', '=', 'b');\n$q/*call*/->where('email', 1);", false},
		{"variable passed on or compared", "$q = Post::query();\nconsume($q);\n$x = $q instanceof Builder;\n$y = $q ? 1 : 2;\n$q/*call*/->where('email', 1);", false},
		{"unclosed call", "$q = Post::query();\n$q->where('title';\n$q/*call*/->where('email', 1);", false},
		{"unmatched parenthesis", `x)/*call*/->where('email', 1);`, true},
		{"operator after operator", "$q = Post::query();\n$q->->join('users');\n$q/*call*/->where('email', 1);", false},
		{"not an operator", `Post::query()->join('users', 'a', '=', 'b')->/*call*/where('email', 1);`, false},
		{"leading backslash only", `\/*call*/->where('email', 1);`, true},
		{"namespace-relative name", `namespace\Post::join('users', 'a')/*call*/->where('email', 1);`, true},
	}
	for _, c := range cases {
		if got := joinedAtMarker(t, c.body, true); got != c.want {
			t.Errorf("%s: BuilderMayBeJoined = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuilderMayBeJoinedOnAReceiverItCannotFollow(t *testing.T) {
	cases := []struct {
		name string
		body string
		want bool
	}{
		{"property of $this", `$this->query/*call*/->where('email', 1);`, true},
		{"property of a variable", `$holder->query/*call*/->where('email', 1);`, true},
		{"property after a call", `Post::query()->builder/*call*/->where('email', 1);`, true},
		{"static property", `Holder::$query/*call*/->where('email', 1);`, true},
		{"array element", `$builders['post']/*call*/->where('email', 1);`, true},
		{"element of a call result", `Post::query()->where('title', 1)[0]/*call*/->where('email', 1);`, true},
		{"function call result", `postQuery()/*call*/->where('email', 1);`, true},
		{"new in parentheses", `(new Post())/*call*/->where('email', 1);`, true},
		{"clone in parentheses", `(clone $q)/*call*/->where('email', 1);`, true},
		{"static keyword without a class", `static/*call*/->where('email', 1);`, true},
		{"static call", `Post/*call*/::where('email', 1);`, false},
		{"qualified static call", `\App\Models\Post/*call*/::where('email', 1);`, false},
		{"model chain", `Post::query()->where('title', 1)/*call*/->where('email', 1);`, false},
		{"late static binding", `static::query()/*call*/->where('email', 1);`, false},
		{"class held in a variable", `$model::query()/*call*/->where('email', 1);`, false},
		{"method chain on $this", `$this->posts()/*call*/->where('email', 1);`, false},
		{"local variable", "$q = Post::query();\n$q/*call*/->where('email', 1);", false},
		{"nullsafe chain on a variable", "$q = Post::query();\n$q?->where('title', 1)?/*call*/->where('email', 1);", false},
	}
	for _, c := range cases {
		if got := joinedAtMarker(t, c.body, true); got != c.want {
			t.Errorf("%s: BuilderMayBeJoined = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestBuilderMayBeJoinedWithoutScopesReadsOnlyTheChain(t *testing.T) {
	body := "$q = Post::query()->join('users', 'a', '=', 'b');\n$q/*call*/->where('email', 1);"
	if joinedAtMarker(t, body, false) {
		t.Error("without scopes, a join on the variable elsewhere is not found")
	}
	if !joinedAtMarker(t, `$q->join('users', 'a', '=', 'b')/*call*/->where('email', 1);`, false) {
		t.Error("a join in the call's own chain is found without scopes")
	}
}

func TestBuilderMayBeJoinedOutsideTheTokens(t *testing.T) {
	tokens := parser.Tokenize("<?php\n$q->where('email', 1);\n")
	if BuilderMayBeJoined(tokens, nil, 99, 0) {
		t.Error("a position after the last token is no call")
	}
}

func TestBuilderMayBeJoinedOnTokensCutShort(t *testing.T) {
	arrow := parser.Token{Kind: parser.TokenArrow, Value: "->", Column: 2}
	variable := parser.Token{Kind: parser.TokenVariable, Value: "$q"}
	if !BuilderMayBeJoined([]parser.Token{{Kind: parser.TokenArrow, Value: "->"}}, nil, 0, 0) {
		t.Error("an operator with nothing before it has a receiver that can't be followed")
	}
	if BuilderMayBeJoined([]parser.Token{variable, arrow}, nil, 0, 2) {
		t.Error("a chain that ends at an operator is no joined builder")
	}
}
