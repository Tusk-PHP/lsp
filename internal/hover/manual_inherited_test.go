package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// setupInheritedProvider builds an in-memory index with a Parent/Child class
// hierarchy where Parent declares inheritedMethod and a property, and Child
// extends Parent but adds its own ownMethod.  Both classes are SourceBuiltin
// so that PHPManualURL emits links.
func setupInheritedProvider(t *testing.T) *Provider {
	t.Helper()
	idx := symbols.NewIndex()

	// Index Parent with inheritedMethod, an inherited property, and a constant.
	idx.IndexFileWithSource("builtin://php/core/parent.php", `<?php
class ParentClass {
    /** Doc for inheritedMethod */
    public function inheritedMethod(): void {}
    /** Doc for inheritedProp */
    public string $inheritedProp = '';
    const PARENT_CONST = 'value';
}
`, symbols.SourceBuiltin)

	// Index Child extending Parent with its own ownMethod.
	// Uses a distinct URI so the parent symbols are not evicted.
	idx.IndexFileWithSource("builtin://php/core/child.php", `<?php
class ChildClass extends ParentClass {
    public function ownMethod(): void {}
}
`, symbols.SourceBuiltin)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	return NewProvider(idx, ca, "none")
}

// TestHoverInheritedMethodURLUsesReceiverClass verifies that hovering an
// inherited method produces a PHP Manual link pointing at the receiver class
// (ChildClass) rather than the declaring class (ParentClass).
func TestHoverInheritedMethodURLUsesReceiverClass(t *testing.T) {
	p := setupInheritedProvider(t)

	source := `<?php
$c = new ChildClass();
$c->inheritedMethod();
`
	pos := charPosOf(t, source, "inheritedMethod", "$c->inheritedMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for inheritedMethod")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "childclass.inheritedmethod.php") {
		t.Errorf("expected manual link pointing at childclass, got:\n%s", val)
	}
	if strings.Contains(val, "parentclass.inheritedmethod.php") {
		t.Errorf("manual link should NOT point at parentclass, got:\n%s", val)
	}
}

// TestHoverDirectMethodURLUsesReceiverClass verifies that hovering a method
// declared directly on the receiver class also correctly names that class in
// the PHP Manual URL.
func TestHoverDirectMethodURLUsesReceiverClass(t *testing.T) {
	p := setupInheritedProvider(t)

	source := `<?php
$c = new ChildClass();
$c->ownMethod();
`
	pos := charPosOf(t, source, "ownMethod", "$c->ownMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for ownMethod")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "childclass.ownmethod.php") {
		t.Errorf("expected manual link pointing at childclass, got:\n%s", val)
	}
}

// TestHoverInheritedPropertyURLUsesReceiverClass verifies that hovering an
// inherited property produces a PHP Manual link pointing at the receiver class.
func TestHoverInheritedPropertyURLUsesReceiverClass(t *testing.T) {
	p := setupInheritedProvider(t)

	source := `<?php
$c = new ChildClass();
$c->inheritedProp;
`
	pos := charPosOf(t, source, "inheritedProp", "$c->inheritedProp")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for inheritedProp")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "class.childclass.php") {
		t.Errorf("expected property manual link containing class.childclass.php, got:\n%s", val)
	}
	if strings.Contains(val, "class.parentclass.php") {
		t.Errorf("property manual link should NOT point at parentclass, got:\n%s", val)
	}
}

// TestHoverStaticConstantURLUsesReceiverClass verifies that hovering a class
// constant inherited from a parent produces a PHP Manual link for the receiver.
func TestHoverStaticConstantURLUsesReceiverClass(t *testing.T) {
	p := setupInheritedProvider(t)

	source := `<?php
ChildClass::PARENT_CONST;
`
	pos := charPosOf(t, source, "PARENT_CONST", "ChildClass::PARENT_CONST")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover result for PARENT_CONST")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "class.childclass.php") {
		t.Errorf("expected constant manual link containing class.childclass.php, got:\n%s", val)
	}
	if strings.Contains(val, "class.parentclass.php") {
		t.Errorf("constant manual link should NOT point at parentclass, got:\n%s", val)
	}
}
