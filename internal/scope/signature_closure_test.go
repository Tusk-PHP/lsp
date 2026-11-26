package scope

import (
	"slices"
	"testing"
)

const afterSortBy = "$between = 1;\nfunction after($later) {\n    $copy = $later;\n}\n$tail = 1;\n"

type signatureCase struct {
	name   string
	source string
	outer  ScopeKind
	params []string
	noArgs bool
	nested bool
}

func TestClosureInASignatureKeepsItsOwnBody(t *testing.T) {
	runSignatureCases(t, []signatureCase{
		{
			name:   "closure in a parameter default",
			source: "<?php\nfunction sortBy(array $items, Closure $cmp = static function ($a, $b) { return $a <=> $b; }) {\n    $sorted = $items;\n}\n" + afterSortBy,
			outer:  ScopeFunction, params: []string{"$items", "$cmp"},
		},
		{
			name:   "closure in an attribute before a function",
			source: "<?php\n#[Sorter(static function ($a, $b) { return $a <=> $b; })]\nfunction sortBy(array $items) {\n    $sorted = $items;\n}\n" + afterSortBy,
			outer:  ScopeFunction, params: []string{"$items"},
		},
		{
			name:   "closure in an attribute before a method",
			source: "<?php\nclass Box {\n    #[Sorter(static function ($a, $b) { return $a <=> $b; })]\n    public function sortBy(array $items) {\n        $sorted = $items;\n    }\n}\n" + afterSortBy,
			outer:  ScopeMethod, params: []string{"$items"},
		},
		{
			name:   "closure in an attribute on a parameter",
			source: "<?php\nfunction sortBy(#[Sorter(static function ($a, $b) { return $a <=> $b; })] array $items) {\n    $sorted = $items;\n}\n" + afterSortBy,
			outer:  ScopeFunction, params: []string{"$items"},
		},
		{
			name:   "closure in a nested function's signature",
			source: "<?php\nfunction outer() {\n    function sortBy(array $items, Closure $cmp = static function ($a, $b) { return $a <=> $b; }) {\n        $sorted = $items;\n    }\n    $back = 1;\n}\n" + afterSortBy,
			outer:  ScopeFunction, params: []string{"$items", "$cmp"}, nested: true,
		},
	})
}

func TestArrowFunctionInASignatureKeepsItsOwnBody(t *testing.T) {
	runSignatureCases(t, []signatureCase{
		{
			name:   "arrow function in a parameter default",
			source: "<?php\nfunction sortBy(array $items, Closure $cmp = static fn($a, $b) => $a <=> $b) {\n    $sorted = $items;\n}\n" + afterSortBy,
			outer:  ScopeFunction, params: []string{"$items", "$cmp"},
		},
		{
			name:   "arrow function without parameters in a default",
			source: "<?php\nfunction sortBy(array $items, Closure $cmp = static fn() => 1 <=> 2) {\n    $sorted = $items;\n}\n" + afterSortBy,
			outer:  ScopeFunction, params: []string{"$items", "$cmp"}, noArgs: true,
		},
	})
}

func runSignatureCases(t *testing.T, cases []signatureCase) {
	t.Helper()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			source := tc.source
			doc := Collect(source)

			inner := doc.ScopeAt(at(t, source, "<=>"))
			if inner == nil || inner.Kind != ScopeClosure {
				t.Fatalf("scope of the closure's body = %+v, want a closure", inner)
			}
			if !tc.noArgs {
				if a := doc.BindingAt(at(t, source, "$a <=>")); a == nil || a.Kind != BindingParameter || a.Scope != inner {
					t.Errorf("$a in the closure's body = %+v, want the closure's parameter", a)
				}
			}

			outer := doc.ScopeAt(at(t, source, "$sorted"))
			if outer == nil || outer.Kind != tc.outer || outer.Name != "sortBy" {
				t.Fatalf("scope of sortBy's body = %+v, want sortBy", outer)
			}
			var params []string
			for _, b := range outer.Bindings {
				if b.Kind == BindingParameter {
					params = append(params, b.Name)
				}
			}
			if !slices.Equal(params, tc.params) {
				t.Errorf("sortBy's parameters = %v, want %v", params, tc.params)
			}
			if items := doc.BindingAt(at(t, source, "$items;")); items == nil || items.Kind != BindingParameter || items.Scope != outer {
				t.Errorf("$items in sortBy's body = %+v, want sortBy's parameter", items)
			}

			if tc.nested {
				if back := doc.ScopeAt(at(t, source, "$back")); back == nil || back.Name != "outer" {
					t.Errorf("scope after the nested function = %+v, want outer", back)
				}
			}

			if next := doc.ScopeAt(at(t, source, "$copy")); next == nil || next.Name != "after" {
				t.Errorf("scope of the next function's body = %+v, want after", next)
			}
			for _, top := range []string{"$between", "$tail"} {
				if s := doc.ScopeAt(at(t, source, top)); s == nil || s.Kind != ScopeFile {
					t.Errorf("scope of %s = %+v, want the file", top, s)
				}
			}
		})
	}
}
