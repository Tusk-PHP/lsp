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
	if sym := idx.Lookup("ArrayIterator::offsetExists"); sym == nil || sym.URI == builtinRegistryURI {
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

func TestAMemberAnotherExtensionAddsIsNotDatedByTheBuildsWithoutIt(t *testing.T) {
	idx := newIndexForPHP("7.4")
	for _, fqn := range []string{"PDO::MYSQL_ATTR_INIT_COMMAND", "PDO::MYSQL_ATTR_SSL_CA", "PDO::PGSQL_ATTR_DISABLE_PREPARES", "PDO::ODBC_ATTR_ASSUME_UTF8"} {
		if idx.Lookup(fqn) == nil {
			t.Errorf("%s, which a PDO driver adds, is known on 7.4", fqn)
		}
	}
}

func TestABuiltinClassBringsItsMembersFromTheVersionThatAddedIt(t *testing.T) {
	cases := []struct{ class, member, since string }{
		{"Dom\\HTMLDocument", "createFromString", "8.4"},
		{"Pdo\\Mysql", "getWarningCount", "8.4"},
		{"BcMath\\Number", "add", "8.4"},
		{"Random\\Randomizer", "getInt", "8.2"},
	}
	old := newIndexForPHP("7.4")
	for _, c := range cases {
		if old.Lookup(c.class+"::"+c.member) != nil {
			t.Errorf("%s::%s is absent on 7.4", c.class, c.member)
		}
		if sym := old.Lookup(c.class); sym != nil && len(sym.Children) > 0 {
			t.Errorf("%s has no members on 7.4: %d", c.class, len(sym.Children))
		}
		if newIndexForPHP(c.since).Lookup(c.class+"::"+c.member) == nil {
			t.Errorf("%s::%s is known on %s", c.class, c.member, c.since)
		}
	}
}

func TestABuiltinClassImplementsTheInterfacesOfTheProjectsVersion(t *testing.T) {
	old := newIndexForPHP("7.4")
	if !old.IsSubtypeOf("SplFixedArray", "Iterator") || old.IsSubtypeOf("SplFixedArray", "IteratorAggregate") {
		t.Errorf("on 7.4 SplFixedArray implements Iterator, not IteratorAggregate: %v", old.Lookup("SplFixedArray").Implements)
	}
	if old.IsSubtypeOf("ArithmeticError", "Stringable") || !old.IsSubtypeOf("ArithmeticError", "Throwable") {
		t.Errorf("on 7.4 ArithmeticError is Throwable, not Stringable: %v", old.Lookup("ArithmeticError").Implements)
	}
	current := newIndexForPHP("8.4")
	if current.IsSubtypeOf("SplFixedArray", "Iterator") || !current.IsSubtypeOf("SplFixedArray", "IteratorAggregate") {
		t.Errorf("from 8.0 SplFixedArray implements IteratorAggregate, not Iterator: %v", current.Lookup("SplFixedArray").Implements)
	}
	if !current.IsSubtypeOf("ArithmeticError", "Stringable") {
		t.Errorf("from 8.0 ArithmeticError is Stringable: %v", current.Lookup("ArithmeticError").Implements)
	}
}

func TestAPropertyOlderBuildsHandledInternallyIsKnownOnThem(t *testing.T) {
	idx := newIndexForPHP("7.4")
	for _, fqn := range []string{"XMLReader::$nodeType", "SoapFault::$faultstring", "DOMNode::$nodeName", "DatePeriod::$start"} {
		if idx.Lookup(fqn) == nil {
			t.Errorf("%s, which PHP 7.4 has without declaring it to reflection, is known on 7.4", fqn)
		}
	}
	if newIndexForPHP("8.2").Lookup("DOMNode::$isConnected") != nil || newIndexForPHP("8.3").Lookup("DOMNode::$isConnected") == nil {
		t.Error("DOMNode::$isConnected, added in 8.3, is dated by the builds that declare DOMNode's properties")
	}
}

func TestBuiltinClassesFollowTheirVersionAndPECLExtension(t *testing.T) {
	cases := []struct {
		fqn, version string
		extensions   []string
		present      bool
	}{
		{"Attribute", "7.4", nil, false},
		{"Attribute", "8.0", nil, true},
		{"IntlListFormatter", "8.4", nil, false},
		{"IntlListFormatter", "8.5", nil, true},
		{"Collator", "7.4", nil, true},
		{"Redis", "8.4", nil, false},
		{"Redis", "8.4", []string{"redis"}, true},
		{"Brotli\\Compress\\Context", "8.4", nil, false},
		{"Brotli\\Compress\\Context", "8.4", []string{"brotli"}, true},
		{"Imagick", "8.4", nil, false},
	}
	for _, c := range cases {
		idx := NewIndex()
		idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: c.version, Extensions: c.extensions})
		if got := idx.Lookup(c.fqn) != nil; got != c.present {
			t.Errorf("%s on PHP %s with %v: present %v, want %v", c.fqn, c.version, c.extensions, got, c.present)
		}
	}
	if a, ok := LookupAvailability("SensitiveParameter"); !ok || a.Since != "8.2" || a.Extension != "" {
		t.Errorf("SensitiveParameter: %+v %v", a, ok)
	}
	if a, ok := LookupAvailability("Random\\Randomizer"); !ok || a.Since != "8.2" {
		t.Errorf("Random\\Randomizer came with the random extension, in PHP's default build from 8.2: %+v %v", a, ok)
	}
	if _, ok := LookupAvailability("Collator"); ok {
		t.Error("Collator is as old as the oldest build with intl: it isn't gated")
	}
	if a, ok := LookupAvailability("RedisCluster"); !ok || a.Extension != "redis" {
		t.Errorf("RedisCluster: %+v %v", a, ok)
	}
}
