package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// literalParts writes each part of rt by its value where it's a literal, so
// a test can tell 'desc' from string and false from bool.
func literalParts(rt ResolvedType) string {
	parts := rt.Parts()
	names := make([]string, 0, len(parts)+1)
	for _, p := range parts {
		switch {
		case p.Literal != "":
			names = append(names, p.Literal)
		case p.FQN != "" && p.FQN != "null":
			names = append(names, p.FQN)
		}
	}
	if rt.Nullable || rt.FQN == "null" {
		names = append(names, "null")
	}
	return strings.Join(names, "|")
}

func narrowedAt(t *testing.T, src string) func(v, marker string) string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	return func(v, marker string) string {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				col := strings.LastIndex(l, v)
				return literalParts(r.ResolveVariableTypeTyped(v, lines, textpos.Position{Line: i, Character: col + 1}, file))
			}
		}
		t.Fatalf("no line marked %q", marker)
		return ""
	}
}

// `$x === lit` and `$x !== lit` narrow $x where they hold, with lit null,
// true, false, a number or a string, on either side; `==` doesn't.
func TestStrictComparisonNarrowing(t *testing.T) {
	src := `<?php
namespace App;
class User {}
/**
 * @param 'desc'|int $dir
 * @param 1|string $level
 */
function run(?User $u, string|false $s, bool $b, $dir, $level, ?User $g, ?User $l, string $any, ?User $w): void {
    if ($u !== null) {
        echo $u; // not null
    } else {
        echo $u; // else null
    }
    if (null === $u) {
        echo $u; // yoda null
    }
    if ($s === false) {
        echo $s; // is false
    }
    if ($s !== false) {
        echo $s; // not false
    }
    if ($b === true) {
        echo $b; // is true
    }
    if ($b !== true) {
        echo $b; // not true
    }
    if ($dir === 'desc') {
        echo $dir; // is desc
    }
    if ($dir !== "desc") {
        echo $dir; // not desc
    }
    if ($level !== 1) {
        echo $level; // not one
    }
    if ($any === 'x') {
        echo $any; // string is x
    }
    if ($g === null) {
        return;
    }
    echo $g; // guard
    if ($l == null) {
        return;
    }
    echo $l; // loose
    $ok = $w !== null && $w; // same line and
    switch (true) {
        case $dir === 'desc':
            echo $dir; // case
            break;
    }
}
`
	at := narrowedAt(t, src)
	for _, c := range []struct{ v, marker, want string }{
		{"$u", "not null", `App\User`},
		{"$u", "else null", `null`},
		{"$u", "yoda null", `null`},
		{"$s", "is false", `false`},
		{"$s", "not false", `string`},
		{"$b", "is true", `true`},
		{"$b", "not true", `false`},
		{"$dir", "is desc", `'desc'`},
		{"$dir", "not desc", `int`},
		{"$level", "not one", `string`},
		{"$any", "string is x", `'x'`},
		{"$g", "guard", `App\User`},
		{"$l", "loose", `App\User|null`},
		{"$w", "same line and", `App\User`},
		{"$dir", "case", `'desc'`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s at %q: got %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

func TestLiteralsEqual(t *testing.T) {
	for _, c := range []struct {
		a, b         ResolvedType
		equal, known bool
	}{
		{ResolvedType{FQN: "string", Literal: "'a'"}, ResolvedType{FQN: "string", Literal: `"a"`}, true, true},
		{ResolvedType{FQN: "string", Literal: "'a'"}, ResolvedType{FQN: "string", Literal: `'b'`}, false, true},
		{ResolvedType{FQN: "string", Literal: "'a'"}, ResolvedType{FQN: "string", Literal: `"\x61"`}, false, false},
		{ResolvedType{FQN: "int", Literal: "1"}, ResolvedType{FQN: "int", Literal: "2"}, false, true},
		{ResolvedType{FQN: "int", Literal: "1"}, ResolvedType{FQN: "int", Literal: "0x1"}, false, false},
		{ResolvedType{FQN: "int", Literal: "1"}, ResolvedType{FQN: "string", Literal: "'1'"}, false, true},
	} {
		if equal, known := literalsEqual(c.a, c.b); equal != c.equal || known != c.known {
			t.Errorf("literalsEqual(%s, %s) = %v, %v; want %v, %v", c.a.Literal, c.b.Literal, equal, known, c.equal, c.known)
		}
	}
}
