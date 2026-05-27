package resolve

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/protocol"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// boxSource is the shared fixture for class-string<T> binding tests.
// Box<T> accepts a class-string<T> constructor arg and returns T from make().
// make() carries `: object` as a PHP type hint (realistic stub shape); the
// rawMemberReturnType fix ensures @return T wins over the placeholder hint.
const boxSource = `<?php
namespace App;

class MyModel {
    public function customMethod(): string {}
}

/** @template T of object */
class Box {
    /** @param class-string<T>|T $cls */
    public function __construct(string|object $cls) {}

    /** @return T */
    public function make(): object {}
}

class Service {
    public function run(): void {
        $b1 = new Box('App\\MyModel');

        $b2 = new Box(MyModel::class);

        $cls = MyModel::class;
        $b3 = new Box($cls);

        $reassigned = MyModel::class;
        $reassigned = 'something_else';
        $b5 = new Box($reassigned);

        $b6 = new Box(strtolower('App\\MyModel'));
    }
}
`

// paramSource tests that a variable sourced from a function parameter is not bound.
const paramSource = `<?php
namespace App;

class MyModel {}

/** @template T of object */
class Box {
    /** @param class-string<T>|T $cls */
    public function __construct(string|object $cls) {}

    /** @return T */
    public function make(): object {}
}

class Service {
    public function run(string $param): void {
        $b7 = new Box($param);
    }
}
`

func setupBoxResolver(t *testing.T, src string) (*Resolver, *parser.FileNode, []string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///box.php", src)
	file := parser.ParseFile(src)
	if file == nil {
		t.Fatal("expected parsed file")
	}
	return NewResolver(idx), file, SplitLines(src)
}

func TestClassStringBindingLiteralString(t *testing.T) {
	r, file, lines := setupBoxResolver(t, boxSource)
	pos := protocol.Position{Line: lineContaining(t, lines, "$b1 =")}
	rt := r.ResolveVariableTypeTyped("$b1", lines, pos, file)
	if rt.FQN != "App\\Box" {
		t.Fatalf("FQN = %q, want App\\Box", rt.FQN)
	}
	if len(rt.Params) != 1 || rt.Params[0].FQN != "App\\MyModel" {
		t.Fatalf("Params = %v, want [{App\\MyModel}]", rt.Params)
	}
}

func TestClassStringBindingClassConstant(t *testing.T) {
	r, file, lines := setupBoxResolver(t, boxSource)
	pos := protocol.Position{Line: lineContaining(t, lines, "$b2 =")}
	rt := r.ResolveVariableTypeTyped("$b2", lines, pos, file)
	if rt.FQN != "App\\Box" {
		t.Fatalf("FQN = %q, want App\\Box", rt.FQN)
	}
	if len(rt.Params) != 1 || rt.Params[0].FQN != "App\\MyModel" {
		t.Fatalf("Params = %v, want [{App\\MyModel}]", rt.Params)
	}
}

func TestClassStringBindingVariableFromClassConstant(t *testing.T) {
	r, file, lines := setupBoxResolver(t, boxSource)
	pos := protocol.Position{Line: lineContaining(t, lines, "$b3 =")}
	rt := r.ResolveVariableTypeTyped("$b3", lines, pos, file)
	if rt.FQN != "App\\Box" {
		t.Fatalf("FQN = %q, want App\\Box", rt.FQN)
	}
	if len(rt.Params) != 1 || rt.Params[0].FQN != "App\\MyModel" {
		t.Fatalf("Params = %v, want [{App\\MyModel}]", rt.Params)
	}
}

func TestClassStringBindingReassignedVariableNotBound(t *testing.T) {
	r, file, lines := setupBoxResolver(t, boxSource)
	pos := protocol.Position{Line: lineContaining(t, lines, "$b5 =")}
	rt := r.ResolveVariableTypeTyped("$b5", lines, pos, file)
	if rt.FQN != "App\\Box" {
		t.Fatalf("FQN = %q, want App\\Box", rt.FQN)
	}
	// reassigned variable must not produce any generic binding
	if len(rt.Params) != 0 {
		t.Fatalf("expected no Params for reassigned variable, got %v", rt.Params)
	}
}

func TestClassStringBindingArbitraryExpressionNotBound(t *testing.T) {
	r, file, lines := setupBoxResolver(t, boxSource)
	pos := protocol.Position{Line: lineContaining(t, lines, "$b6 =")}
	rt := r.ResolveVariableTypeTyped("$b6", lines, pos, file)
	if rt.FQN != "App\\Box" {
		t.Fatalf("FQN = %q, want App\\Box", rt.FQN)
	}
	if len(rt.Params) != 0 {
		t.Fatalf("expected no Params for function-call arg, got %v", rt.Params)
	}
}

func TestClassStringBindingParameterVariableNotBound(t *testing.T) {
	r, file, lines := setupBoxResolver(t, paramSource)
	pos := protocol.Position{Line: lineContaining(t, lines, "$b7 =")}
	rt := r.ResolveVariableTypeTyped("$b7", lines, pos, file)
	if rt.FQN != "App\\Box" {
		t.Fatalf("FQN = %q, want App\\Box", rt.FQN)
	}
	if len(rt.Params) != 0 {
		t.Fatalf("expected no Params for parameter variable, got %v", rt.Params)
	}
}

func TestClassStringBindingMakeMethodReturn(t *testing.T) {
	r, file, lines := setupBoxResolver(t, boxSource)

	// Resolve $b2 type to get Box<App\MyModel>
	b2Line := protocol.Position{Line: lineContaining(t, lines, "$b2 =")}
	b2Type := r.ResolveVariableTypeTyped("$b2", lines, b2Line, file)
	if b2Type.FQN != "App\\Box" || len(b2Type.Params) != 1 {
		t.Fatalf("prerequisite failed: $b2 type = %v", b2Type)
	}

	// $b2->make() should resolve to App\MyModel via template substitution
	makeSym := r.FindMember("App\\Box", "make")
	if makeSym == nil {
		t.Fatal("Box::make not found in index")
	}
	ret := r.MemberTypeResolved(makeSym, file, "", b2Type.Params)
	if ret.FQN != "App\\MyModel" {
		t.Fatalf("make() return type = %q, want App\\MyModel", ret.FQN)
	}
}
