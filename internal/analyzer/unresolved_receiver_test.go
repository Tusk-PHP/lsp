package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const unresolvedReceiverSource = `<?php
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

func setupUnresolvedReceiverAnalyzer() (*Analyzer, string, string) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///app/UnrelatedHandler.php", `<?php
namespace App;

class UnrelatedHandler {
    public function handle(): void {}
}
`)
	idx.IndexFile("file:///app/Service.php", unresolvedReceiverSource)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)
	return a, "file:///app/Service.php", unresolvedReceiverSource
}

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

// TestFindReferencesUnresolvedReceiverNoFalsePositive verifies that FindReferences
// for a member access with an unknown receiver type returns nil/empty rather than
// collecting unrelated symbols that share the same name.
func TestFindReferencesUnresolvedReceiverNoFalsePositive(t *testing.T) {
	a, uri, source := setupUnresolvedReceiverAnalyzer()
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	reader := func(u string) string {
		if u == uri {
			return source
		}
		return ""
	}
	locs := a.FindAllReferences(uri, source, pos, reader)
	if len(locs) != 0 {
		t.Errorf("expected no references for member access with unresolved receiver, got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		}
	}
}

// TestPrepareRenameUnresolvedReceiverNoFalsePositive verifies that PrepareRename
// returns nil for a member access whose receiver type cannot be resolved.
func TestPrepareRenameUnresolvedReceiverNoFalsePositive(t *testing.T) {
	a, uri, source := setupUnresolvedReceiverAnalyzer()
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	result := a.PrepareRename(uri, source, pos)
	if result != nil {
		t.Errorf("expected nil PrepareRename for member access with unresolved receiver, got placeholder=%q", result.Placeholder)
	}
}

// TestRenameUnresolvedReceiverNoFalsePositive verifies that Rename returns nil
// (or an empty edit) for a member access whose receiver type cannot be resolved.
func TestRenameUnresolvedReceiverNoFalsePositive(t *testing.T) {
	a, uri, source := setupUnresolvedReceiverAnalyzer()
	pos := charPosOf(t, source, "handle", "$reflmethod->handle")
	reader := func(u string) string {
		if u == uri {
			return source
		}
		return ""
	}
	edit := a.Rename(uri, source, pos, "newHandle", reader)
	if edit != nil && len(edit.Changes) > 0 {
		t.Errorf("expected nil/empty WorkspaceEdit for member access with unresolved receiver, got %d change sets", len(edit.Changes))
		for fileURI, edits := range edit.Changes {
			t.Logf("  %s: %d edits", fileURI, len(edits))
		}
	}
}
