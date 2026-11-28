package symbols

import (
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
)

func TestCompareFoldMatchesFoldName(t *testing.T) {
	words := []string{
		"", "a", "A", "ab", "aB", "Ab", "abc", "b", "B", "_x", "x_", "Z", "[", "`", "@",
		"Über", "über", "uber", "Ünter", "İstanbul", "istanbul", "K", "K", "k", "ß", "SS", "é", "E",
		"Collection", "collection", "Collect", "CollectionInterface", "Colléction",
	}
	sign := func(c int) int { return max(-1, min(1, c)) }
	for _, a := range words {
		for _, b := range words {
			want := strings.Compare(FoldName(a), FoldName(b))
			if got := compareFold(a, b); sign(got) != want {
				t.Errorf("compareFold(%q, %q) = %d, want %d", a, b, got, want)
			}
			lp := FoldName(b)
			if got, want := hasFoldPrefix(a, lp), strings.HasPrefix(FoldName(a), lp); got != want {
				t.Errorf("hasFoldPrefix(%q, %q) = %v, want %v", a, lp, got, want)
			}
		}
	}
}

// TestReindexWithSameNamesKeepsSortedListsCurrent covers the editor's common
// case: a file is re-indexed after an edit inside a method body. No name
// appears or disappears, so the next search must not rebuild anything.
func TestReindexWithSameNamesKeepsSortedListsCurrent(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nnamespace A;\nclass Foo { public function bar() { return 1; } }\n")
	idx.IndexFile("file:///b.php", "<?php\nnamespace B;\nclass Baz { public function bar() {} }\n")
	idx.WarmSearchIndexes()

	idx.IndexFile("file:///a.php", "<?php\nnamespace A;\nclass Foo { public function bar() { return 2; } }\n")
	if idx.sortedNames.needsUpdate() || idx.sortedFQNs.needsUpdate() {
		t.Fatal("re-indexing a file with the same names must leave the sorted lists current")
	}

	// A name only this file had is removed and added back: still current.
	idx.IndexFile("file:///a.php", "<?php\nnamespace A;\nclass Foo { public function bar() { return 3; } }\n")
	if idx.sortedNames.needsUpdate() || idx.sortedFQNs.needsUpdate() {
		t.Fatal("re-indexing must cancel a removal with the matching addition")
	}
}

func TestSortedListsFollowRenamesWithoutRebuilding(t *testing.T) {
	idx := NewIndex()
	for i := range 20 {
		idx.IndexFile(fmt.Sprintf("file:///f%d.php", i), fmt.Sprintf("<?php\nnamespace N%d;\nclass C%d { public function m%d() {} }\n", i, i, i))
	}
	idx.WarmSearchIndexes()

	idx.IndexFile("file:///f3.php", "<?php\nnamespace N3;\nclass Renamed { public function fresh() {} }\n")
	if !idx.sortedNames.built || !idx.sortedFQNs.built {
		t.Fatal("a rename should be merged into the sorted lists, not drop them")
	}
	if got := sortedNamesOf(idx.SearchByPrefix("fre")); !slices.Equal(got, []string{"fresh"}) {
		t.Errorf("SearchByPrefix(fre) = %v, want [fresh]", got)
	}
	if got := idx.SearchByPrefix("m3"); len(got) != 0 {
		t.Errorf("SearchByPrefix(m3) = %v, want nothing after the rename", sortedNamesOf(got))
	}
	if syms, _ := idx.SearchByFQNPrefix(`N3\`); !slices.Equal(sortedNamesOf(syms), []string{"Renamed"}) {
		t.Errorf(`SearchByFQNPrefix(N3\) = %v, want [Renamed]`, sortedNamesOf(syms))
	}
	assertSortedListsMatchRebuild(t, idx)
}

// TestSortedListsMatchRebuildUnderRandomEdits indexes, edits and removes files
// with names that differ in case, share prefixes and use non-ASCII letters,
// searching in between, and checks after every step that the sorted lists
// hold exactly what a rebuild from scratch would.
func TestSortedListsMatchRebuildUnderRandomEdits(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	stems := []string{"Coll", "coll", "COLL", "Über", "über", "Item", "item", "K", "Kelvin", "a", "B"}
	name := func() string {
		return stems[rng.IntN(len(stems))] + stems[rng.IntN(len(stems))] + fmt.Sprint(rng.IntN(6))
	}
	file := func() string {
		var b strings.Builder
		fmt.Fprintf(&b, "<?php\nnamespace %s;\n", name())
		for range 1 + rng.IntN(3) {
			fmt.Fprintf(&b, "class %s {\n", name())
			for range rng.IntN(4) {
				fmt.Fprintf(&b, "  public function %s() {}\n  public $%s;\n", name(), name())
			}
			b.WriteString("}\n")
		}
		fmt.Fprintf(&b, "function %s() {}\n", name())
		return b.String()
	}

	idx := NewIndex()
	for step := range 3000 {
		uri := fmt.Sprintf("file:///f%d.php", rng.IntN(40))
		switch rng.IntN(10) {
		case 0:
			idx.RemoveFile(uri)
		case 1, 2:
			idx.SearchByPrefix(stems[rng.IntN(len(stems))])
			idx.SearchByFQNPrefix(stems[rng.IntN(len(stems))])
		default:
			idx.IndexFile(uri, file())
		}
		if step%25 == 0 {
			assertSortedListsMatchRebuild(t, idx)
			if t.Failed() {
				t.Fatalf("lists diverged at step %d", step)
			}
		}
	}
	assertSortedListsMatchRebuild(t, idx)
}

// assertSortedListsMatchRebuild brings both lists up to date and compares
// them with lists rebuilt from the index.
func assertSortedListsMatchRebuild(t *testing.T, idx *Index) {
	t.Helper()
	idx.WarmSearchIndexes()
	idx.mu.Lock()
	defer idx.mu.Unlock()
	names, fqns := idx.sortedNames.keys, idx.sortedFQNs.keys

	var fresh Index
	fresh.nameIndex, fresh.symbols = idx.nameIndex, idx.symbols
	fresh.updateSortedNamesLocked()
	fresh.updateSortedFQNsLocked()
	if !slices.Equal(names, fresh.sortedNames.keys) {
		t.Errorf("sorted names differ from a rebuild:\n got %q\nwant %q", names, fresh.sortedNames.keys)
	}
	if !slices.Equal(fqns, fresh.sortedFQNs.keys) {
		t.Errorf("sorted FQNs differ from a rebuild:\n got %q\nwant %q", fqns, fresh.sortedFQNs.keys)
	}
}

func sortedNamesOf(syms []*Symbol) []string {
	out := make([]string, 0, len(syms))
	for _, s := range syms {
		out = append(out, s.Name)
	}
	slices.Sort(out)
	return out
}

func TestPrefixSearchFoldsASCIIOnly(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nnamespace Ns;\nclass Äpfel {}\nclass Apfel {}\n")

	var names []string
	for _, sym := range idx.SearchByPrefix("äp") {
		names = append(names, sym.Name)
	}
	if len(names) != 0 {
		t.Errorf("SearchByPrefix(äp) = %v, want nothing: PHP keeps Ä and ä apart", names)
	}
	syms, _ := idx.SearchByFQNPrefix("ns\\ap")
	if len(syms) != 1 || syms[0].Name != "Apfel" {
		t.Errorf("SearchByFQNPrefix(ns\\ap) = %v, want only Apfel", syms)
	}
	syms, _ = idx.SearchByFQNPrefix("NS\\Äp")
	if len(syms) != 1 || syms[0].Name != "Äpfel" {
		t.Errorf("SearchByFQNPrefix(NS\\Äp) = %v, want only Äpfel", syms)
	}
}
