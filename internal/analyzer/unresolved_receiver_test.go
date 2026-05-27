package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// TestDefinitionUnresolvedReceiverNoFalsePositive verifies that go-to-definition
// for a member access with an unknown receiver type returns nil instead of
// jumping to an unrelated class that happens to have a member with the same name.
func TestDefinitionUnresolvedReceiverNoFalsePositive(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///app/UnrelatedHandler.php", `<?php
namespace App;

class UnrelatedHandler {
    public function handle(): void {}
}
`)
	idx.IndexFile("file:///app/Service.php", `<?php
namespace App;

class Service {
    public function run(): void {
        $reflMethods = (new \ReflectionObject($this))->getMethods();
        foreach ($reflMethods as $reflmethod) {
            $reflmethod->handle();
        }
    }
}
`)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	source := `<?php
namespace App;

class Service {
    public function run(): void {
        $reflMethods = (new \ReflectionObject($this))->getMethods();
        foreach ($reflMethods as $reflmethod) {
            $reflmethod->handle();
        }
    }
}
`
	// Position on "handle" in "$reflmethod->handle()"
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	loc := a.FindDefinition("file:///app/Service.php", source, pos)
	if loc != nil {
		t.Errorf("expected nil definition for member access with unresolved receiver, got: %s", loc.URI)
	}
}

// TestTypeDefinitionUnresolvedReceiverNoFalsePositive verifies that type-definition
// for a member access with an unknown receiver type returns nil.
func TestTypeDefinitionUnresolvedReceiverNoFalsePositive(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///app/UnrelatedHandler.php", `<?php
namespace App;

class UnrelatedHandler {
    public function handle(): void {}
}
`)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	source := `<?php
namespace App;

class Service {
    public function run(): void {
        $reflMethods = (new \ReflectionObject($this))->getMethods();
        foreach ($reflMethods as $reflmethod) {
            $reflmethod->handle();
        }
    }
}
`
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	locs := a.FindTypeDefinition("file:///app/Service.php", source, pos)
	if len(locs) != 0 {
		t.Errorf("expected no type definitions for member access with unresolved receiver, got %d", len(locs))
	}
}
