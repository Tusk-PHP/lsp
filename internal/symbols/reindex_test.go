package symbols

import (
	"slices"
	"testing"
)

func symbolFQNs(syms []*Symbol) []string {
	fqns := make([]string, len(syms))
	for i, s := range syms {
		fqns[i] = s.FQN
	}
	return fqns
}

func TestReindexKeepsNameNamespaceAndFoldIndexesInOrder(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nnamespace App;\nfunction first() {}\nclass Shared {}\n")
	idx.IndexFile("file:///b.php", "<?php\nnamespace App;\nfunction second() {}\nconst THIRD = 3;\nfunction fourth() {}\n")
	idx.IndexFile("file:///c.php", "<?php\nnamespace Other;\nfunction second() {}\nclass shared {}\n")

	idx.IndexFile("file:///b.php", "<?php\nnamespace App;\nfunction fourth() {}\nfunction fifth() {}\n")

	if got, want := symbolFQNs(idx.GetNamespaceMembers("App")), []string{"App\\Shared", "App\\first", "App\\fourth", "App\\fifth"}; !slices.Equal(got, want) {
		t.Fatalf("namespace App after re-index = %v, want %v", got, want)
	}
	if got, want := symbolFQNs(idx.LookupByName("second")), []string{"Other\\second"}; !slices.Equal(got, want) {
		t.Fatalf("second() after re-index = %v, want %v", got, want)
	}
	if got := idx.LookupByName("THIRD"); got != nil {
		t.Fatalf("a constant the file no longer declares is still found by name: %v", symbolFQNs(got))
	}
	if sym := idx.LookupFold("app\\FIFTH", nil); sym == nil || sym.FQN != "App\\fifth" {
		t.Fatalf("a function added by the re-index is found ignoring case: %+v", sym)
	}

	idx.IndexFile("file:///a.php", "<?php\n")
	if got, want := symbolFQNs(idx.GetNamespaceMembers("App")), []string{"App\\fourth", "App\\fifth"}; !slices.Equal(got, want) {
		t.Fatalf("namespace App after emptying a.php = %v, want %v", got, want)
	}
	if sym := idx.LookupFold("other\\SHARED", nil); sym == nil || sym.FQN != "Other\\shared" {
		t.Fatalf("a class of another namespace is still found ignoring case: %+v", sym)
	}
}

func TestPromotedDeclarationKeepsItsNamesIndexed(t *testing.T) {
	for _, tc := range []struct {
		name      string
		a, b      string
		namespace []string
		byName    map[string][]string
		byPrefix  map[string][]string
	}{
		{
			name:      "class declared in two files",
			a:         "<?php\nnamespace App;\nclass Dup { public function onlyInA() {} public function both() {} }\n",
			b:         "<?php\nnamespace App;\nclass Dup { public function both() {} public function onlyInB() {} }\n",
			namespace: []string{"App\\Dup"},
			byName:    map[string][]string{"Dup": {"App\\Dup"}, "onlyInA": {"App\\Dup::onlyInA"}, "both": {"App\\Dup::both"}, "onlyInB": nil},
			byPrefix:  map[string][]string{"onlyIn": {"App\\Dup::onlyInA"}},
		},
		{
			name:      "member removed and promoted back in the same pass",
			a:         "<?php namespace App; interface Foo { function a(); }",
			b:         "<?php namespace App; if (true) { class Foo { function a() {} } } else { class Foo { function c() {} } }",
			namespace: []string{"App\\Foo"},
			byName:    map[string][]string{"Foo": {"App\\Foo"}, "a": {"App\\Foo::a"}, "c": nil},
			byPrefix:  map[string][]string{"a": {"App\\Foo::a"}},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			idx := NewIndex()
			idx.IndexFile("file:///a.php", tc.a)
			idx.IndexFile("file:///b.php", tc.b)

			idx.IndexFile("file:///b.php", "<?php\n")

			if got := symbolFQNs(idx.GetNamespaceMembers("App")); !slices.Equal(got, tc.namespace) {
				t.Fatalf("namespace App = %v, want %v", got, tc.namespace)
			}
			for name, want := range tc.byName {
				got := idx.LookupByName(name)
				if !slices.Equal(symbolFQNs(got), want) && !(want == nil && got == nil) {
					t.Errorf("LookupByName(%q) = %v, want %v", name, symbolFQNs(got), want)
				}
				for _, s := range got {
					if s.URI != "file:///a.php" {
						t.Errorf("LookupByName(%q) returns the removed declaration from %s", name, s.URI)
					}
				}
			}
			for prefix, want := range tc.byPrefix {
				var got []string
				for _, s := range idx.SearchByPrefix(prefix) {
					if s.URI == "file:///a.php" {
						got = append(got, s.FQN)
					}
				}
				if !slices.Equal(got, want) {
					t.Errorf("SearchByPrefix(%q) from a.php = %v, want %v", prefix, got, want)
				}
			}
		})
	}
}
