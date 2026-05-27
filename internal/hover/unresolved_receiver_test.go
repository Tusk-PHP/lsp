package hover

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/container"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// TestHoverUnresolvedReceiverNoFalsePositive verifies that hovering over a member
// access where the receiver type is unknown returns nil instead of a hover card
// for an unrelated class that happens to have a member with the same name.
//
// Scenario: $reflmethod comes from an array returned by getMethods(), so its type
// is unknown. An unrelated class has a method called "handle" with the same name
// as the member being accessed. Without the fix, the broad LookupByName fallback
// would return the unrelated class's method.
func TestHoverUnresolvedReceiverNoFalsePositive(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	// This class has a "handle" method — it should NOT appear when hovering over
	// $reflmethod->handle where $reflmethod has an unknown type.
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
	p := NewProvider(idx, ca, "none")

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
	// Position hover on "handle" in "$reflmethod->handle()"
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	hover := p.GetHover("file:///app/Service.php", source, pos)
	if hover != nil {
		t.Errorf("expected nil hover for member access with unresolved receiver, got:\n%s", hover.Contents.Value)
	}
}

// TestHoverResolvedReceiverStillWorks verifies that hover on a member access with
// a known receiver type continues to return the correct hover card.
func TestHoverResolvedReceiverStillWorks(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	idx.IndexFile("file:///app/Thing.php", `<?php
namespace App;

class Thing {
    public function handle(): void {}
}
`)

	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	source := `<?php
namespace App;

use App\Thing;

class Consumer {
    public function run(): void {
        $t = new Thing();
        $t->handle();
    }
}
`
	pos := charPosOf(t, source, "handle", "$t->handle")
	hover := p.GetHover("file:///app/Consumer.php", source, pos)
	if hover == nil {
		t.Fatal("expected hover on member access with known receiver type")
	}
	val := hover.Contents.Value
	if !strings.Contains(val, "handle") {
		t.Errorf("expected 'handle' in hover, got:\n%s", val)
	}
	if !strings.Contains(val, "App\\Thing") {
		t.Errorf("expected parent class FQN App\\Thing in hover, got:\n%s", val)
	}
}
