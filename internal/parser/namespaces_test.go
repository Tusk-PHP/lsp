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

func TestNamespaceAtFindsTheBlockAroundAPosition(t *testing.T) {
	file := ParseFile("<?php\nnamespace A;\nuse X\\Dep;\nf(); namespace B; g();\n")
	for _, c := range []struct {
		line, character int
		want            string
	}{
		{0, 3, ""},
		{1, 0, "A"},
		{3, 0, "A"},
		{3, 4, "A"},
		{3, 5, "B"},
		{3, 18, "B"},
	} {
		if got := file.NamespaceAt(c.line, c.character); got == nil || got.Name != c.want {
			t.Errorf("NamespaceAt(%d, %d) = %+v, want %q", c.line, c.character, got, c.want)
		}
	}
	if got := file.NamespaceAt(3, 0).Uses; len(got) != 1 || got[0].FullName != `X\Dep` {
		t.Errorf("A's imports = %+v, want X\\Dep", got)
	}
	braced := ParseFile("<?php\nnamespace A {\n    f();\n}\nnamespace {\n    g();\n}\n")
	if got := braced.NamespaceAt(2, 4); got == nil || got.Name != "A" {
		t.Errorf("inside the braced block A: %+v", got)
	}
	if got := braced.NamespaceAt(5, 4); got == nil || got.Name != "" || got == braced.NamespaceAt(0, 0) {
		t.Errorf("inside the braced global block: %+v, want its own global namespace", got)
	}
	if got := (*FileNode)(nil).NamespaceAt(0, 0); got != nil {
		t.Errorf("no file has no namespace, got %+v", got)
	}
	if got := file.NamespaceAt(-1, 0); got != nil {
		t.Errorf("a position before the file has no namespace, got %+v", got)
	}
}

func TestNamespaceAsAMemberOrRelativeNameStartsNoBlock(t *testing.T) {
	for _, src := range []string{
		"<?php\nnamespace App;\nuse X\\Dep;\nRoute::namespace('Admin')->group(fn () => 1);\nclass Foo {}\n",
		"<?php\nnamespace App;\nuse X\\Dep;\n$r?->namespace('x');\n$r->namespace('y');\nclass Foo {}\n",
		"<?php\nnamespace App;\nuse X\\Dep;\nnamespace\\helper();\nclass Foo {}\n",
	} {
		file := ParseFile(src)
		if len(file.Namespaces) != 2 {
			t.Errorf("%q: %d namespaces, want the global one and App", src, len(file.Namespaces))
		}
		ns := file.Classes[0].Namespace
		if ns == nil || ns.Name != "App" || len(ns.Uses) != 1 || file.Classes[0].FullName != `App\Foo` {
			t.Errorf("%q: Foo's namespace = %+v, want App importing X\\Dep", src, ns)
		}
	}
}

func TestIsNamespaceDeclaration(t *testing.T) {
	for src, want := range map[string]bool{
		"<?php namespace App;":               true,
		"<?php namespace App\\Http;":         true,
		"<?php namespace { }":                true,
		"<?php Route::namespace('Admin');":   false,
		"<?php $r->namespace('x');":          false,
		"<?php $r?->namespace('x');":         false,
		"<?php $r-> /* c */ namespace('x');": false,
		"<?php namespace\\helper();":         false,
		"<?php namespace":                    false,
	} {
		toks := Tokenize(src)
		at := -1
		for i, tok := range toks {
			if tok.Kind == TokenNamespace {
				at = i
			}
		}
		if got := IsNamespaceDeclaration(toks, at); got != want {
			t.Errorf("IsNamespaceDeclaration(%q) = %v, want %v", src, got, want)
		}
	}
}
