package parser

import "testing"

func TestClassDeclaredInBlockIsConditional(t *testing.T) {
	source := `<?php
namespace Symfony\Bridge\PhpUnit;

class_alias('Real', 'Symfony\Bridge\PhpUnit\Listener');

if (false) {
    class Listener {}
}

class Plain {}
`
	result := New().Parse(source)
	want := map[string]bool{"Listener": true, "Plain": false}
	if len(result.Classes) != len(want) {
		t.Fatalf("want %d classes, got %+v", len(want), result.Classes)
	}
	for _, cls := range result.Classes {
		if cls.Conditional != want[cls.Name] {
			t.Errorf("%s: Conditional = %v, want %v", cls.Name, cls.Conditional, want[cls.Name])
		}
	}
}

func TestClassInBracedNamespaceIsNotConditional(t *testing.T) {
	source := `<?php
namespace MongoDB {
    if (!class_exists(Client::class, false)) {
        class Client {}
    }
    class Plain {}
}
namespace Other {
    class AfterNamespace {}
}
`
	want := map[string]bool{"Client": true, "Plain": false, "AfterNamespace": false}
	file := ParseFile(source)
	if len(file.Classes) != len(want) {
		t.Fatalf("want %d classes, got %+v", len(want), file.Classes)
	}
	for _, cls := range file.Classes {
		if cls.Conditional != want[cls.Name] {
			t.Errorf("%s: Conditional = %v, want %v", cls.Name, cls.Conditional, want[cls.Name])
		}
	}
}
