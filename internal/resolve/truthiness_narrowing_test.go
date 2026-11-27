package resolve

import "testing"

// A test on the variable itself, `if ($x)`, `if (!$x) return;` or `$x && …`,
// takes null and false out of it where it holds; where it fails, $x could
// still be ”, 0 or [], so nothing is taken out.
func TestTruthinessNarrowing(t *testing.T) {
	src := `<?php
namespace App;
interface Store {}
class Order {}
class Refund {}
/** @return false|Order|Refund */
function find_order(int $id) {}
function run(?Store $store, bool $b, ?Store $g, ?Store $w, ?Store $e, ?Store $q, string|null $s, ?Store $c): void {
    if ($store) {
        echo $store; // then
    } else {
        echo $store; // else
    }
    if ($b) {
        echo $b; // bool
    }
    $order = find_order(1);
    if (!$order) {
        return;
    }
    echo $order; // guard
    if (!$g) {
        throw new \Exception();
    }
    echo $g; // throw guard
    $ok = $w && $w; // same line and
    $t = $q ? $q : null; // ternary
    while ($e) {
        echo $e; // while
    }
    if ($s) {
        echo $s; // string
    }
    switch (true) {
        case $c:
            echo $c; // case
            break;
    }
}
`
	at := narrowedAt(t, src)
	for _, c := range []struct{ v, marker, want string }{
		{"$store", "then", `App\Store`},
		{"$store", "else", `App\Store|null`},
		{"$b", "bool", `true`},
		{"$order", "guard", `App\Order|App\Refund`},
		{"$g", "throw guard", `App\Store`},
		{"$w", "same line and", `App\Store`},
		{"$q", "ternary", `App\Store`},
		{"$e", "while", `App\Store`},
		{"$s", "string", `string`},
		{"$c", "case", `App\Store`},
	} {
		if got := at(c.v, c.marker); got != c.want {
			t.Errorf("%s at %q: got %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}
