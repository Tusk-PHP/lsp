package types

import (
	"strings"
	"testing"
)

func TestMapClassNames(t *testing.T) {
	rename := func(name string) string {
		switch name {
		case "int", "string", "bool", "array", "list", "null", "T":
			return name
		}
		return `App\` + strings.TrimPrefix(name, `\`)
	}
	tests := []struct {
		raw, want string
		changed   bool
	}{
		{"Post", `App\Post`, true},
		{"?Post", `?App\Post`, true},
		{"Collection<int, Post>", `App\Collection<int, App\Post>`, true},
		{"array{id: int, post?: Post, 0: list<Tag>}", `array{id: int, post?: App\Post, 0: list<App\Tag>}`, true},
		{"Post[]", `App\Post[]`, true},
		{"Post[Key]", `App\Post[App\Key]`, true},
		{"Closure(Post, int): Tag", `Closure(App\Post, int): App\Tag`, true},
		{"callable(int): bool", "callable(int): bool", false},
		{"Closure<U of Base>(U): U", `Closure<U of App\Base>(U): U`, true},
		{"Closure<U>(U): Closure<V>(U, V): V", `Closure<U>(U): Closure<V>(U, V): V`, false},
		{"($x is Post ? Tag : null)", `($x is App\Post ? App\Tag : null)`, true},
		{"(A&B)|null", `(App\A & App\B) | null`, true},
		{"'literal'|int", "'literal' | int", false},
		{"($x is int ? string : null)", "($x is int ? string : null)", false},
		{"int[]", "int[]", false},
		{"Closure<U of int>(U): U", "Closure<U of int>(U): U", false},
	}
	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			expr := ParseTypeExpr(tt.raw)
			before := expr.String()
			got, changed := MapClassNames(expr, rename)
			if got.String() != tt.want || changed != tt.changed {
				t.Errorf("MapClassNames(%q) = %q, %v, want %q, %v", tt.raw, got.String(), changed, tt.want, tt.changed)
			}
			if expr.String() != before {
				t.Errorf("MapClassNames changed its input to %q", expr.String())
			}
		})
	}
}

func TestCompactString(t *testing.T) {
	for raw, want := range map[string]string{
		"A | null":             "A|null",
		"(A & B) | ?C":         "(A&B)|?C",
		"?(A|B)":               "?(A|B)",
		"Collection<int, A|B>": "Collection<int, A | B>",
		"array{a: int}":        "array{a: int}",
	} {
		if got := ParseTypeExpr(raw).CompactString(); got != want {
			t.Errorf("CompactString(%q) = %q, want %q", raw, got, want)
		}
	}
}
