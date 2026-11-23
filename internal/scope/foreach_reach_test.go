package scope

import (
	"slices"
	"testing"
)

// A foreach variable is written again at the start of each turn, so a write
// later in the body doesn't reach a read before it; other variables still
// carry a write round the loop.
func TestReachingWritesForeachVariableEachTurn(t *testing.T) {
	tests := []struct {
		name, src, variable string
		want                []int
	}{
		{"destructured foreach variable", `<?php
function f(array $routes) {
    foreach ($routes as $route) {
        /*?*/$route->a();
        [$name, $route] = $route;
    }
}`, "$route", []int{3}},
		{"foreach key", `<?php
function f(array $routes) {
    foreach ($routes as $key => $route) {
        /*?*/echo $key;
        $key = 'x';
    }
}`, "$key", []int{3}},
		{"after the destructuring", `<?php
function f(array $routes) {
    foreach ($routes as $route) {
        [$name, $route] = $route;
        /*?*/$route->a();
    }
}`, "$route", []int{4}},
		{"a variable written before the loop still goes round", `<?php
function f(array $routes) {
    $x = new Foo();
    foreach ($routes as $route) {
        /*?*/$x->a();
        $x = new Bar();
    }
}`, "$x", []int{3, 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _ := reachingLines(t, tt.src, tt.variable)
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWriteSiteDestructure(t *testing.T) {
	src := `<?php
function f(array $pair) {
    $a = 1;
    [$a, $b] = $pair;
    list($a, $c) = $pair;
}`
	doc := Collect(src)
	for _, b := range doc.Bindings {
		if b.Name != "$a" {
			continue
		}
		if b.Site(DeclWrite).Destructure {
			t.Error("the plain assignment isn't a destructuring")
		}
		if len(b.Rebinds) != 2 || !b.Site(0).Destructure || !b.Site(1).Destructure {
			t.Errorf("want two destructuring rebinds, got %+v", b.Rebinds)
		}
		return
	}
	t.Fatal("no binding for $a")
}
