package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

func setupReflectionProvider(t *testing.T) (*Provider, string) {
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
	p := NewProvider(idx, ca, "none")
	return p, src
}

// TestHoverReflectionClassStringBindingClassConstant verifies that hovering on
// the $r variable after new \ReflectionClass(MyModel::class) shows the bound
// generic type ReflectionClass<App\MyModel>.
func TestHoverReflectionClassStringBindingClassConstant(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$r = new \ReflectionClass(MyModel::class);
$r;
`
	pos := charPosOf(t, source, "$r", "$r;")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $r variable")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "ReflectionClass") {
		t.Errorf("expected ReflectionClass in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "MyModel") {
		t.Errorf("expected MyModel generic param in hover, got:\n%s", val)
	}
}

// TestHoverReflectionClassStringBindingLiteralString verifies that hovering on
// the $r variable after new \ReflectionClass('App\\MyModel') shows the bound
// generic type ReflectionClass<App\MyModel>.
func TestHoverReflectionClassStringBindingLiteralString(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$r = new \ReflectionClass('App\\MyModel');
$r;
`
	pos := charPosOf(t, source, "$r", "$r;")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $r variable")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "ReflectionClass") {
		t.Errorf("expected ReflectionClass in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "MyModel") {
		t.Errorf("expected MyModel generic param in hover, got:\n%s", val)
	}
}

// TestHoverReflectionMethodGetName verifies that hovering on $reflmethod->getName()
// resolves to ReflectionMethod::getName when the variable type is known from a
// direct assignment. This confirms the hover resolves correctly to ReflectionMethod,
// not an unrelated class.
func TestHoverReflectionMethodGetName(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {}

$reflmethod = new \ReflectionMethod(MyModel::class, 'customMethod');
$reflmethod->getName();
`
	pos := charPosOf(t, source, "getName", "$reflmethod->getName")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $reflmethod->getName()")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "ReflectionMethod") {
		t.Errorf("expected ReflectionMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "getName") {
		t.Errorf("expected getName in hover, got:\n%s", val)
	}
}

// TestHoverReflectionGetMethodsNoFalsePositive verifies that hovering on a
// foreach variable sourced from getMethods() does not produce a false-positive
// hover card for an unrelated class with the same member name. This is the
// original user-reported bug: $reflmethod->handle should not resolve to some
// unrelated class's "handle" method.
func TestHoverReflectionGetMethodsNoFalsePositive(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///app/MyModel.php", `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}
`)
	// This class has a "handle" method — it must NOT appear when hovering over
	// $reflmethod->handle where $reflmethod has an unknown type from foreach.
	idx.IndexFile("file:///app/UnrelatedHandler.php", `<?php
namespace App;

class UnrelatedHandler {
    public function handle(): void {}
}
`)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
namespace App;

$refl = new \ReflectionClass(MyModel::class);
foreach ($refl->getMethods() as $reflmethod) {
    $reflmethod->handle();
}
`
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover != nil {
		t.Errorf("expected nil hover for foreach variable with unresolved receiver, got:\n%s", hover.Contents.Value)
	}
}

// TestHoverReflectionObjectInheritance verifies that ReflectionObject inherits
// getMethod from ReflectionClass so that hover on $refl->getMethod resolves
// correctly to the method defined on the parent class.
func TestHoverReflectionObjectInheritance(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {}

$obj = new MyModel();
$refl = new \ReflectionObject($obj);
$refl->getMethod('customMethod');
`
	pos := charPosOf(t, source, "getMethod", "$refl->getMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $refl->getMethod (inherited from ReflectionClass)")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "getMethod") {
		t.Errorf("expected getMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "ReflectionClass") {
		t.Errorf("expected ReflectionClass as parent in hover, got:\n%s", val)
	}
}

// TestHoverChainReflectionNewInstanceMethod verifies that hovering on customMethod
// after $r->newInstance() resolves the bound generic T (MyModel) so that
// members of App\MyModel are found.
func TestHoverChainReflectionNewInstanceMethod(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$r = new \ReflectionClass(MyModel::class);
$inst = $r->newInstance();
$inst->customMethod();
`
	pos := charPosOf(t, source, "customMethod", "$inst->customMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $inst->customMethod(), got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "customMethod") {
		t.Errorf("expected customMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "MyModel") {
		t.Errorf("expected MyModel in hover, got:\n%s", val)
	}
}

// TestHoverChainReflectionDirectMethod verifies that hovering on customMethod in
// a direct chain $r->newInstance()->customMethod() propagates the bound T.
func TestHoverChainReflectionDirectMethod(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$r = new \ReflectionClass(MyModel::class);
$r->newInstance()->customMethod();
`
	pos := charPosOf(t, source, "customMethod", "->newInstance()->customMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on direct chain ->newInstance()->customMethod(), got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "customMethod") {
		t.Errorf("expected customMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "MyModel") {
		t.Errorf("expected MyModel in hover, got:\n%s", val)
	}
}

// TestHoverChainReflectionLiteralStringArg verifies that propagation works when
// the ReflectionClass constructor receives a string literal instead of ::class.
func TestHoverChainReflectionLiteralStringArg(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$r = new \ReflectionClass('App\\MyModel');
$r->newInstance()->customMethod();
`
	pos := charPosOf(t, source, "customMethod", "->newInstance()->customMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on literal-string chain ->newInstance()->customMethod(), got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "customMethod") {
		t.Errorf("expected customMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "MyModel") {
		t.Errorf("expected MyModel in hover, got:\n%s", val)
	}
}

// TestHoverReflectionMethodNameProperty verifies that hovering on the $name
// property of a ReflectionMethod instance shows a hover card for the builtin
// property ReflectionMethod::$name. This requires the property to be declared
// in the stub and PickBestStandalone to not poison standalone-name searches.
func TestHoverReflectionMethodNameProperty(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {}

$reflmethod = new \ReflectionMethod(MyModel::class, 'customMethod');
$reflmethod->name;
`
	pos := charPosOf(t, source, "name", "$reflmethod->name")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $reflmethod->name property, got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "ReflectionMethod") {
		t.Errorf("expected ReflectionMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "name") {
		t.Errorf("expected 'name' in hover, got:\n%s", val)
	}
}

// TestHoverReflectionMethodClassProperty verifies that hovering on the $class
// property of a ReflectionMethod instance shows the builtin property.
func TestHoverReflectionMethodClassProperty(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {}

$reflmethod = new \ReflectionMethod(MyModel::class, 'customMethod');
$reflmethod->class;
`
	pos := charPosOf(t, source, "class", "$reflmethod->class")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $reflmethod->class property, got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "ReflectionMethod") {
		t.Errorf("expected ReflectionMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "class") {
		t.Errorf("expected 'class' in hover, got:\n%s", val)
	}
}

// TestHoverForeachReflectionMethodArrayProperty verifies the end-to-end Faker use
// case: $refl->getMethods() returns ReflectionMethod[], so iterating it with foreach
// should bind $reflmethod as ReflectionMethod, making ->name resolve correctly.
func TestHoverForeachReflectionMethodArrayProperty(t *testing.T) {
	p, _ := setupReflectionProvider(t)

	source := `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

$refl = new \ReflectionClass(MyModel::class);
foreach ($refl->getMethods() as $reflmethod) {
    $reflmethod->name;
}
`
	pos := charPosOf(t, source, "name", "$reflmethod->name")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $reflmethod->name inside foreach loop, got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "ReflectionMethod") {
		t.Errorf("expected ReflectionMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "name") {
		t.Errorf("expected 'name' property in hover, got:\n%s", val)
	}
}

// TestHoverForeachArrayLiteralElementType verifies that foreach over a function
// returning T[] binds the loop variable to T so member access resolves.
func TestHoverForeachArrayLiteralElementType(t *testing.T) {
	idx := symbols.NewIndex()
	src := `<?php
namespace App;

class MyClass {
    public function customMethod(): string {}
}
`
	idx.IndexFile("file:///app/MyClass.php", src)
	idx.IndexFile("file:///app/helpers.php", `<?php
namespace App;

/** @return MyClass[] */
function getThings(): array {}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
namespace App;

/** @return MyClass[] */
function getThings(): array {}

foreach (getThings() as $thing) {
    $thing->customMethod();
}
`
	pos := charPosOf(t, source, "customMethod", "$thing->customMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $thing->customMethod() inside foreach, got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "customMethod") {
		t.Errorf("expected customMethod in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "MyClass") {
		t.Errorf("expected MyClass in hover, got:\n%s", val)
	}
}

// TestHoverForeachKeyValueValueTyped verifies that foreach with key => value binds
// the value variable ($thing) to the array's value element type.
func TestHoverForeachKeyValueValueTyped(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/MyClass.php", `<?php
namespace App;

class MyClass {
    public function customMethod(): string {}
}
`)
	idx.IndexFile("file:///app/helpers.php", `<?php
namespace App;

/** @return array<string, MyClass> */
function getMap(): array {}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
namespace App;

/** @return array<string, MyClass> */
function getMap(): array {}

foreach (getMap() as $key => $thing) {
    $thing->customMethod();
}
`
	pos := charPosOf(t, source, "customMethod", "$thing->customMethod")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on $thing->customMethod() with key=>value foreach, got nil")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "customMethod") {
		t.Errorf("expected customMethod in hover, got:\n%s", val)
	}
}

// TestHoverForeachUnknownArrayNoFalsePositive verifies that when the iterable has
// no @return annotation, foreach does not produce a false-positive hover.
func TestHoverForeachUnknownArrayNoFalsePositive(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///app/UnrelatedClass.php", `<?php
namespace App;

class UnrelatedClass {
    public function whatever(): void {}
}
`)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
namespace App;

function getStuff(): array {}

foreach (getStuff() as $thing) {
    $thing->whatever;
}
`
	pos := charPosOf(t, source, "whatever", "$thing->whatever")
	hover := p.GetHover("file:///test.php", source, pos)
	if hover != nil {
		t.Errorf("expected nil hover for untyped foreach variable, got:\n%s", hover.Contents.Value)
	}
}
