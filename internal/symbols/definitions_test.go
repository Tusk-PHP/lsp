package symbols

import (
	"slices"
	"testing"
)

func definitionURIs(defs []*Symbol) []string {
	uris := make([]string, 0, len(defs))
	for _, d := range defs {
		uris = append(uris, d.URI)
	}
	slices.Sort(uris)
	return uris
}

func TestDefinitionsKeepEveryDeclarationOfAName(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nif (!function_exists('h')) { function h($a) {} }\n")
	idx.IndexFile("file:///b.php", "<?php\nif (!function_exists('h')) { function h($b) {} }\n")
	if got := definitionURIs(idx.Definitions("h")); !slices.Equal(got, []string{"file:///a.php", "file:///b.php"}) {
		t.Fatalf("both declarations of h are definitions: %v", got)
	}

	idx.IndexFile("file:///a.php", "<?php\nif (!function_exists('h')) { function h($a, $c) {} }\n")
	if got := definitionURIs(idx.Definitions("h")); !slices.Equal(got, []string{"file:///a.php", "file:///b.php"}) {
		t.Fatalf("re-indexing one declaration keeps the other: %v", got)
	}
	if sym := idx.Lookup("h"); sym == nil || sym.URI != "file:///a.php" {
		t.Fatalf("the last declaration indexed is the one Lookup returns: %+v", sym)
	}

	idx.IndexFile("file:///b.php", "<?php\nif (!function_exists('h')) { function h($b) {} }\n")
	idx.IndexFile("file:///b.php", "<?php\n")
	if sym := idx.Lookup("h"); sym == nil || sym.URI != "file:///a.php" || len(sym.Params) != 2 {
		t.Fatalf("removing the current declaration brings back the other one: %+v", sym)
	}
	if got := definitionURIs(idx.Definitions("h")); !slices.Equal(got, []string{"file:///a.php"}) {
		t.Fatalf("the removed declaration is no longer a definition: %v", got)
	}
	if got := idx.LookupByName("h"); len(got) != 1 {
		t.Fatalf("h is still found by name: %v", got)
	}

	idx.IndexFile("file:///a.php", "<?php\n")
	if idx.Lookup("h") != nil || idx.Definitions("h") != nil {
		t.Fatal("h is gone once no file declares it")
	}
}

func TestRemovingAnEarlierDeclarationKeepsTheCurrentOne(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nclass Dup { public function __construct($a) {} public function only() {} }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup { public function __construct($b) {} }\n")
	idx.IndexFile("file:///a.php", "<?php\n")
	if sym := idx.Lookup("Dup"); sym == nil || sym.URI != "file:///b.php" {
		t.Fatalf("removing the file of a replaced declaration keeps the current one: %+v", sym)
	}
	if sym := idx.Lookup("Dup::__construct"); sym == nil || sym.URI != "file:///b.php" {
		t.Fatalf("and its members: %+v", sym)
	}
	if idx.Lookup("Dup::only") != nil {
		t.Fatal("a member only the removed declaration had is gone")
	}
}

func TestPromotedClassGetsBackItsMembersAndParent(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///base.php", "<?php\nclass Base { public function __construct($base) {} }\nclass Other {}\n")
	idx.IndexFile("file:///a.php", "<?php\nclass Dup extends Base { public function a() {} }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup extends Other { public function b() {} }\n")
	idx.IndexFile("file:///b.php", "<?php\n")
	if sym := idx.Lookup("Dup::a"); sym == nil {
		t.Fatal("the promoted declaration's members are found again")
	}
	if got := idx.GetInheritanceChain("Dup"); !slices.Contains(got, "Base") || slices.Contains(got, "Other") {
		t.Fatalf("the promoted declaration's parent is the one it declares: %v", got)
	}
}

func TestDefinitionMembersFollowEachDeclaration(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///base.php", "<?php\nclass Base { public function __construct($base) {} }\n")
	idx.IndexFile("file:///a.php", "<?php\nclass Dup extends Base {}\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup { public function __construct($own) {} }\n")
	constructors := map[string]string{}
	for _, def := range idx.Definitions("Dup") {
		for _, m := range idx.DefinitionMembers(def) {
			if m.Name == "__construct" {
				constructors[def.URI] = m.Params[0].Name
				break
			}
		}
	}
	if constructors["file:///a.php"] != "$base" || constructors["file:///b.php"] != "$own" {
		t.Fatalf("each declaration has its own constructor, inherited or declared: %v", constructors)
	}
}

func TestReindexingADuplicateClassDoesNotGrowItsInterfaces(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", "<?php\nclass Dup implements Countable { use T; }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup implements Countable { use T; }\n")
	for range 3 {
		idx.IndexFile("file:///a.php", "<?php\nclass Dup implements Countable { use T; }\n")
	}
	if got := idx.GetImplementedInterfaces("Dup"); !slices.Equal(got, []string{"Countable"}) {
		t.Fatalf("interfaces: %v", got)
	}
	if got := idx.TraitsOf("Dup"); !slices.Equal(got, []string{"T"}) {
		t.Fatalf("traits: %v", got)
	}
}

func TestEachDeclarationHasItsOwnTraitsAndInterfaces(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///t.php", "<?php\ninterface I {}\ntrait T { public function m($x) {} }\nclass P { public function m($a) {} }\n")
	idx.IndexFile("file:///a.php", "<?php\nclass Dup implements I { use T; }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup extends P {}\n")
	params := map[string]string{}
	for _, def := range idx.Definitions("Dup") {
		for _, m := range idx.DefinitionMembers(def) {
			if m.Name == "m" {
				params[def.URI] = m.Params[0].Name
				break
			}
		}
	}
	if params["file:///a.php"] != "$x" || params["file:///b.php"] != "$a" {
		t.Fatalf("each declaration takes m from its own trait or parent: %v", params)
	}
	if got := idx.TraitsOf("Dup"); len(got) != 0 {
		t.Fatalf("the current declaration uses no trait: %v", got)
	}
	if got := idx.GetImplementedInterfaces("Dup"); len(got) != 0 {
		t.Fatalf("the current declaration implements no interface: %v", got)
	}
	if got := idx.Implementations("I"); slices.Contains(got, "Dup") {
		t.Fatalf("the current declaration doesn't implement I: %v", got)
	}
}

func TestPromotedDeclarationReplacesTheRemovedOnesTraitsAndInterfaces(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///t.php", "<?php\ninterface I {}\ntrait T { public function fromTrait() {} }\nclass P { public function fromParent() {} }\n")
	idx.IndexFile("file:///b.php", "<?php\nclass Dup extends P {}\n")
	idx.IndexFile("file:///a.php", "<?php\nclass Dup implements I { use T; }\n")
	if got := memberNames(idx, "Dup"); got["fromTrait"] == 0 || got["fromParent"] != 0 {
		t.Fatalf("the current declaration has its trait's members and not the other's parent's: %v", got)
	}

	idx.IndexFile("file:///a.php", "<?php\n")
	if got := memberNames(idx, "Dup"); got["fromTrait"] != 0 || got["fromParent"] == 0 {
		t.Fatalf("the remaining declaration has its parent's members and not the removed one's trait's: %v", got)
	}
	if got := idx.TraitsOf("Dup"); len(got) != 0 {
		t.Fatalf("traits: %v", got)
	}
	if got := idx.GetImplementedInterfaces("Dup"); len(got) != 0 {
		t.Fatalf("interfaces: %v", got)
	}
	if got := idx.Implementations("I"); slices.Contains(got, "Dup") {
		t.Fatalf("Dup no longer implements I: %v", got)
	}
	if got := idx.Implementations("P"); !slices.Contains(got, "Dup") {
		t.Fatalf("Dup extends P: %v", got)
	}
}

func TestLookupStandaloneSkipsMembersOfOtherClasses(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///room.php", "<?php\nnamespace Other;\nclass Room { public function area() {} const SIZE = 1; }\n")
	idx.IndexFile("file:///plan.php", "<?php\nnamespace App;\nclass Plan { public function area() {} }\nconst SIZE = 2;\n")
	if got := idx.LookupStandalone("area", ""); got != nil {
		t.Errorf("outside a class a bare name reaches no member, got %+v", got)
	}
	if got := idx.LookupStandalone("area", `App\Plan`); got == nil || got.FQN != `App\Plan::area` {
		t.Errorf("inside a class it reaches that class's own member, got %+v", got)
	}
	if got := idx.LookupStandalone(`App\SIZE`, `App\Plan`); got == nil || got.FQN != `App\SIZE` {
		t.Errorf("a qualified name is looked up by its last part, got %+v", got)
	}
}
