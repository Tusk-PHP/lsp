package resolve

import "testing"

func TestIdentityNarrowingEdges(t *testing.T) {
	src := `<?php
namespace App;
class User {}
class Holder {
    public function run(?User $u, ?User $v, $y): void {
        $hex = 0x10;
        if ($hex === 16) {
            echo $hex; // hex against decimal
        }
        $octal = 010;
        if ($octal !== 8) {
            echo $octal; // octal against decimal
        }
        $f = 1.5;
        if ($f === 2.5) {
            echo $f; // floats
        }
        if ($u) {
            echo $u; // truthiness
        }
        if ($v instanceof User && $y === null) {
            echo $v; // identity on another variable
        }
        if ($u === $y) {
            echo $u; // identity against a variable
        }
    }
    public function other(?User $w, $y): void {
        if ($y) {
        } elseif ($w) {
            echo $w; // truthiness in an elseif
        }
    }
}
`
	typed, _ := assignedTypes(t, src)
	for _, c := range []struct{ v, marker, want string }{
		{"$hex", "hex against decimal", `int`},
		{"$octal", "octal against decimal", `int`},
		{"$f", "floats", `float`},
		{"$u", "truthiness", `App\User`},
		{"$v", "identity on another variable", `App\User`},
		{"$u", "identity against a variable", `?App\User`},
		{"$w", "truthiness in an elseif", `App\User`},
	} {
		if got := typed(c.v, c.marker); got != c.want {
			t.Errorf("%s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

func TestLiteralsWhoseValuesArentRead(t *testing.T) {
	r := NewResolver(nil)
	for _, c := range []struct{ subject, target ResolvedType }{
		{ResolvedType{FQN: "int", Literal: "0x10"}, ResolvedType{FQN: "int", Literal: "16"}},
		{ResolvedType{FQN: "int", Literal: "010"}, ResolvedType{FQN: "int", Literal: "8"}},
		{ResolvedType{FQN: "int", Literal: "1_000"}, ResolvedType{FQN: "int", Literal: "1000"}},
		{ResolvedType{FQN: "float", Literal: "1.5"}, ResolvedType{FQN: "float", Literal: "2.5"}},
	} {
		if got := r.typeMatches(c.subject, c.target); got != matchUnknown {
			t.Errorf("typeMatches(%s, %s) = %v, want unknown", c.subject.Literal, c.target.Literal, got)
		}
	}
}
