package analyzer

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupReflectionAnalyzer(t *testing.T) (*Analyzer, string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	src := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}
`
	idx.IndexFile("file:///app/MyModel.php", src)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)
	return a, src
}

// TestDefinitionChainReflectionNewInstance verifies that go-to-definition on
// customMethod after $r->newInstance()->customMethod() lands at MyModel::customMethod,
// not nil — confirming that the analyzer chain walker propagates the bound T.
func TestDefinitionChainReflectionNewInstance(t *testing.T) {
	a, _ := setupReflectionAnalyzer(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$r = new \ReflectionClass(MyModel::class);
$r->newInstance()->customMethod();
`
	pos := charPosOf(t, source, "customMethod", "->newInstance()->customMethod")
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition location for customMethod via newInstance() chain, got nil")
	}
	if !strings.Contains(loc.URI, "MyModel.php") {
		t.Errorf("expected URI to contain MyModel.php, got %s", loc.URI)
	}
}

// TestDefinitionForeachArrayElementType verifies that go-to-definition on a method
// of a foreach loop variable resolves to the correct user-defined class when the
// iterable has a typed @return annotation.
func TestDefinitionForeachArrayElementType(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
namespace App;

class MyClass {
    public function customMethod(): string {}
}
`
	idx.IndexFile("file:///app/MyClass.php", src)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	source := `<?php
namespace App;

/** @return MyClass[] */
function getThings(): array {}

foreach (getThings() as $thing) {
    $thing->customMethod();
}
`
	pos := charPosOf(t, source, "customMethod", "$thing->customMethod")
	loc := a.FindDefinition("file:///test.php", source, pos)
	if loc == nil {
		t.Fatal("expected definition location for $thing->customMethod() in foreach loop, got nil")
	}
	if !strings.Contains(loc.URI, "MyClass.php") {
		t.Errorf("expected URI to contain MyClass.php, got %s", loc.URI)
	}

}
