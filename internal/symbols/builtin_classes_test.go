package symbols

import (
	"slices"
	"testing"
)

func newIndexForPHP(version string) *Index {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: version})
	return idx
}

func TestBuiltinInterfacesAndClassesHaveTheirMembers(t *testing.T) {
	idx := newIndexForPHP("8.4")
	count := idx.Lookup("Countable::count")
	if count == nil || count.Kind != KindMethod || count.ParentFQN != "Countable" || count.ReturnType != "int" || !count.IsAbstract {
		t.Fatalf("Countable::count: %+v", count)
	}
	if iface := idx.Lookup("Countable"); iface == nil || iface.Kind != KindInterface {
		t.Errorf("Countable is an interface: %+v", iface)
	}
	offsetGet := idx.Lookup("ArrayAccess::offsetGet")
	if offsetGet == nil || offsetGet.Kind != KindMethod || offsetGet.ParentFQN != "ArrayAccess" {
		t.Errorf("ArrayAccess::offsetGet: %+v", offsetGet)
	}
	for _, fqn := range []string{"JsonSerializable::jsonSerialize", "IteratorAggregate::getIterator", "Stringable::__toString", "PDO::prepare", "PDOStatement::fetch", "Random\\Randomizer::getInt"} {
		if idx.Lookup(fqn) == nil {
			t.Errorf("%s is known", fqn)
		}
	}
	if sym := idx.Lookup("ArrayIterator::count"); sym == nil || sym.URI == builtinRegistryURI {
		t.Errorf("a member the hand-written stubs declare keeps their declaration: %+v", sym)
	}
}

func TestBuiltinMembersFollowThePHPVersion(t *testing.T) {
	if newIndexForPHP("8.3").Lookup("PDO::connect") != nil {
		t.Error("PDO::connect, added in PHP 8.4, is absent on 8.3")
	}
	if newIndexForPHP("8.4").Lookup("PDO::connect") == nil {
		t.Error("PDO::connect is present on 8.4")
	}
	if newIndexForPHP("7.4").Lookup("ReflectionClass::export") == nil {
		t.Error("ReflectionClass::export is present on 7.4")
	}
	if newIndexForPHP("8.0").Lookup("ReflectionClass::export") != nil {
		t.Error("ReflectionClass::export, removed in PHP 8.0, is absent on 8.0")
	}
}

func TestAClassImplementingABuiltinInterfaceHasItsMemberAbove(t *testing.T) {
	idx := newIndexForPHP("8.4")
	idx.IndexFile("file:///Bag.php", "<?php\nnamespace App;\nclass Bag implements \\Countable {\n    public function count(): int { return 0; }\n}\nabstract class Box implements \\Countable {}\n")
	if got := idx.ClassMembersNamed("Countable", "count"); len(got) != 1 || got[0].FQN != "Countable::count" {
		t.Errorf("Countable declares count(): %v", got)
	}
	if got := idx.ClassMembersNamed("App\\Box", "count"); len(got) != 1 || got[0].FQN != "Countable::count" {
		t.Errorf("Box inherits Countable::count: %v", got)
	}
	if !slices.ContainsFunc(idx.GetImplementors("Countable"), func(s *Symbol) bool { return s.FQN == "App\\Bag" }) {
		t.Error("Bag implements Countable")
	}
}

func TestAHandWrittenStubKeepsItsShapeAndGainsTheGeneratedMembers(t *testing.T) {
	idx := newIndexForPHP("8.4")
	fiber := idx.Lookup("Fiber")
	if fiber == nil || fiber.URI == builtinRegistryURI {
		t.Fatalf("Fiber comes from its hand-written stub: %+v", fiber)
	}
	if idx.Lookup("Fiber::getCurrent") == nil {
		t.Error("a member only the generated table has is added to the hand-written class")
	}
	if HasStubbedMembers(idx.Lookup("Countable")) {
		t.Error("Countable is known from the generated table alone")
	}
	if !HasStubbedMembers(fiber) {
		t.Error("Fiber's own stub declares members")
	}

	iface := idx.Lookup("DateTimeInterface")
	if iface == nil || iface.Kind != KindInterface || iface.URI == builtinRegistryURI {
		t.Fatalf("DateTimeInterface comes from its hand-written stub: %+v", iface)
	}
	byName := map[string]*Symbol{}
	for _, child := range iface.Children {
		if byName[child.Name] != nil {
			t.Errorf("DateTimeInterface lists %s once", child.Name)
		}
		byName[child.Name] = child
	}
	if m := byName["format"]; m == nil || m.URI == builtinRegistryURI {
		t.Errorf("a member the stub declares keeps the stub's declaration: %+v", m)
	}
	if m := byName["__serialize"]; m == nil || m.URI != builtinRegistryURI {
		t.Errorf("a member only the generated table has joins the stubbed interface: %+v", m)
	}
}

func TestAStubbedInterfaceKeepsItsTemplates(t *testing.T) {
	idx := newIndexForPHP("8.4")
	iface := idx.Lookup("ArrayAccess")
	if iface == nil || iface.URI == builtinRegistryURI || len(iface.Templates) == 0 {
		t.Errorf("ArrayAccess keeps the generics its stub declares: %+v", iface)
	}
	if m := idx.Lookup("ArrayAccess::offsetGet"); m == nil || m.URI == builtinRegistryURI {
		t.Errorf("ArrayAccess::offsetGet is the stub's: %+v", m)
	}
}

func TestABuiltinSwappedInUnderAProjectClassLeavesItsDocAlone(t *testing.T) {
	idx := newIndexForPHP("8.3")
	app := "file:///app/ArrayObject.php"
	idx.IndexFile(app, "<?php\nclass ArrayObject { public function mine(): void {} }\n")
	if err := idx.ReplaceBuiltins(BuiltinProfile{PHPVersion: "8.2"}); err != nil {
		t.Fatal(err)
	}
	if sym := idx.Lookup("ArrayObject"); sym == nil || sym.URI != app || sym.DocComment != "" {
		t.Errorf("the project's ArrayObject keeps its own empty doc: %+v", sym)
	}
}

func TestLaterStubLayersAddToABuiltinTrait(t *testing.T) {
	idx := NewIndex()
	idx.IndexFileWithSource("builtin://core/php-7.4.php", "<?php\ntrait Greets { public function hello(): string {} }\n", SourceBuiltin)
	idx.IndexFileWithSource("builtin://core/php-8.0.php", "<?php\ntrait Greets { public function bye(): string {} }\n", SourceBuiltin)
	trait := idx.Lookup("Greets")
	if trait == nil || trait.Kind != KindTrait {
		t.Fatalf("Greets: %+v", trait)
	}
	var names []string
	for _, child := range trait.Children {
		names = append(names, child.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, []string{"bye", "hello"}) {
		t.Errorf("the 8.0 layer adds bye() to the 7.4 layer's trait: %v", names)
	}
}

func TestAStubThatDeclaresNoMembersLeavesTheClassOpen(t *testing.T) {
	idx := NewIndex()
	idx.IndexFileWithSource("builtin://ext/php-7.4.php", "<?php\nclass Opaque {}\nclass Child extends \\Opaque {}\n", SourceBuiltin)
	if HasStubbedMembers(idx.Lookup("Opaque")) {
		t.Error("a stub that lists no members can't show one is missing")
	}
	if !HasStubbedMembers(idx.Lookup("Child")) {
		t.Error("a stub that names a parent declares the class's shape")
	}
}
