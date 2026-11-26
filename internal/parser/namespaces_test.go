package parser

import "testing"

func TestParseFileGivesEachDeclarationItsNamespace(t *testing.T) {
	file := ParseFile(`<?php
namespace A;
use X\Dep;
class Foo {}
function fa() {}
const CA = 1;
namespace B;
use Y\Other;
class Bar {}
interface IB {}
trait TB {}
enum EB {}
function fb() { return new class {}; }
`)
	if file == nil || len(file.Classes) != 2 || len(file.AnonymousClasses) != 1 {
		t.Fatalf("ParseFile = %+v, want two classes and one anonymous class", file)
	}
	a, b := file.Classes[0].Namespace, file.Classes[1].Namespace
	if a == nil || a.Name != "A" || len(a.Uses) != 1 || a.Uses[0].FullName != `X\Dep` {
		t.Errorf("Foo's namespace = %+v, want A importing X\\Dep", a)
	}
	if b == nil || b.Name != "B" || len(b.Uses) != 1 || b.Uses[0].FullName != `Y\Other` {
		t.Errorf("Bar's namespace = %+v, want B importing Y\\Other", b)
	}
	if len(file.Functions) != 2 || file.Functions[0].Namespace != a || file.Functions[1].Namespace != b || file.Constants[0].Namespace != a {
		t.Errorf("fa and CA are in %+v and %+v, want A", file.Functions[0].Namespace, file.Constants[0].Namespace)
	}
	if file.Interfaces[0].Namespace != b || file.Traits[0].Namespace != b || file.Enums[0].Namespace != b || file.AnonymousClasses[0].Namespace != b {
		t.Errorf("IB, TB, EB and the anonymous class are not all in B")
	}
	if file.Classes[0].FullName != `A\Foo` || file.Classes[1].FullName != `B\Bar` {
		t.Errorf("FullNames = %q, %q, want A\\Foo and B\\Bar", file.Classes[0].FullName, file.Classes[1].FullName)
	}
	if len(file.Uses) != 2 {
		t.Errorf("file.Uses = %+v, want both imports", file.Uses)
	}
}

func TestParseFileWithoutNamespaceUsesTheGlobalOne(t *testing.T) {
	file := ParseFile("<?php\nuse X\\Dep;\nclass Foo {}\n")
	ns := file.Classes[0].Namespace
	if ns == nil || ns.Name != "" || len(ns.Uses) != 1 {
		t.Errorf("Foo's namespace = %+v, want the global one importing X\\Dep", ns)
	}
}
