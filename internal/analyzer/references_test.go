package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestFindAllReferencesClass(t *testing.T) {
	sources := map[string]string{
		"file:///user.php": `<?php
namespace App\Models;
class User {
    public string $name;
}
`,
		"file:///controller.php": `<?php
namespace App\Http;
use App\Models\User;
class UserController {
    public function show(User $user): User {
        $x = new User();
        return $user;
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Find references to User from its declaration
	locs := a.FindAllReferences("file:///user.php", sources["file:///user.php"],
		protocol.Position{Line: 2, Character: 8}, reader)

	if len(locs) < 4 {
		t.Errorf("expected at least 4 references to User (decl + use + 2 type hints + new), got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		}
	}

	// Verify references span both files
	fileCount := make(map[string]int)
	for _, loc := range locs {
		fileCount[loc.URI]++
	}
	if fileCount["file:///controller.php"] == 0 {
		t.Error("expected references in controller.php")
	}
	if fileCount["file:///user.php"] == 0 {
		t.Error("expected references in user.php (definition)")
	}
}

func TestFindAllReferencesMethod(t *testing.T) {
	sources := map[string]string{
		"file:///service.php": `<?php
namespace App;
class Service {
    public function process(): void {}
}
`,
		"file:///caller.php": `<?php
namespace App;
use App\Service;
class Caller {
    public function run(Service $s): void {
        $s->process();
        $s->process();
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Find references to process from its declaration
	locs := a.FindAllReferences("file:///service.php", sources["file:///service.php"],
		protocol.Position{Line: 3, Character: 22}, reader)

	// Definition + 2 call sites
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to process(), got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character)
		}
	}
}

func TestFindAllReferencesVariable(t *testing.T) {
	source := `<?php
namespace App;
class Foo {
    public function bar() {
        $count = 0;
        echo $count;
        $count++;
    }
    public function other() {
        $count = 99;
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	locs := a.FindAllReferences("file:///test.php", source,
		protocol.Position{Line: 4, Character: 9}, nil)

	// Should find $count only in bar(), not in other()
	for _, loc := range locs {
		if loc.Range.Start.Line == 9 {
			t.Error("should NOT include $count from other() method")
		}
	}
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to $count in bar(), got %d", len(locs))
	}
}

func TestFindAllReferencesParameter(t *testing.T) {
	source := `<?php
function run($name) {
    echo $name;
    return $name;
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	locs := a.FindAllReferences("file:///test.php", source,
		protocol.Position{Line: 1, Character: 14}, nil)

	if len(locs) != 3 {
		t.Fatalf("expected 3 references to $name (decl + 2 uses), got %d", len(locs))
	}
}

func TestFindAllReferencesProperty(t *testing.T) {
	sources := map[string]string{
		"file:///model.php": `<?php
namespace App;
class User {
    public string $name;
    public function getName(): string {
        return $this->name;
    }
}
`,
		"file:///use.php": `<?php
namespace App;
use App\User;
class Test {
    public function run(User $u): void {
        echo $u->name;
    }
}
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	// Find references to $name property
	locs := a.FindAllReferences("file:///model.php", sources["file:///model.php"],
		protocol.Position{Line: 3, Character: 20}, reader)

	// Declaration ($name) + $this->name + $u->name
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to name property, got %d", len(locs))
		for _, loc := range locs {
			t.Logf("  %s:%d:%d-%d", loc.URI, loc.Range.Start.Line, loc.Range.Start.Character, loc.Range.End.Character)
		}
	}

	// Verify cross-file
	fileCount := make(map[string]int)
	for _, loc := range locs {
		fileCount[loc.URI]++
	}
	if fileCount["file:///use.php"] == 0 {
		t.Error("expected property reference in use.php")
	}
}

func TestFindAllReferencesFunction(t *testing.T) {
	sources := map[string]string{
		"file:///helpers.php": `<?php
function formatName(string $name): string {
    return ucfirst($name);
}
`,
		"file:///caller.php": `<?php
$result = formatName("john");
echo formatName("jane");
`,
	}
	a, reader := setupRenameAnalyzer(sources)

	locs := a.FindAllReferences("file:///helpers.php", sources["file:///helpers.php"],
		protocol.Position{Line: 1, Character: 10}, reader)

	// Definition + 2 call sites
	if len(locs) < 3 {
		t.Errorf("expected at least 3 references to formatName, got %d", len(locs))
	}
}

func TestFindAllReferencesDeduplicates(t *testing.T) {
	source := `<?php
namespace App;
class Foo {}
`
	a, reader := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	locs := a.FindAllReferences("file:///test.php", source,
		protocol.Position{Line: 2, Character: 8}, reader)

	// Definition should appear only once
	count := 0
	for _, loc := range locs {
		if loc.URI == "file:///test.php" && loc.Range.Start.Line == 2 {
			count++
		}
	}
	if count > 1 {
		t.Errorf("expected definition to appear once, got %d times", count)
	}
}

func TestFindReferencesOnBuiltinReturnsEmpty(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()

	source := "<?php\n$x = array_reverse([]);\n"
	idx.IndexFileWithSource("file:///project/test.php", source, symbols.SourceProject)

	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))

	// Position the cursor on "array_reverse"
	lines := strings.Split(source, "\n")
	col := strings.Index(lines[1], "array_reverse")
	if col < 0 {
		t.Fatalf("could not find array_reverse in source")
	}
	pos := protocol.Position{Line: 1, Character: col}

	locs := a.FindReferences("file:///project/test.php", source, pos)
	if len(locs) != 0 {
		t.Fatalf("expected empty references for builtin array_reverse, got %d locations", len(locs))
	}
}

func TestFindReferencesSkipsBuiltinURIs(t *testing.T) {
	// Index a project file that defines formatName, then also register builtins.
	// Confirm that no returned reference location has a URI starting with "builtin://".
	sources := map[string]string{
		"file:///project/helpers.php": `<?php
function formatName(string $name): string {
    return strtoupper($name);
}
`,
		"file:///project/caller.php": `<?php
$result = formatName("john");
echo formatName("jane");
`,
	}

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for uri, src := range sources {
		idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	}
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))
	reader := func(uri string) string { return sources[uri] }

	// Position on formatName in helpers.php (line 1)
	helperSrc := sources["file:///project/helpers.php"]
	lines := strings.Split(helperSrc, "\n")
	col := strings.Index(lines[1], "formatName")
	if col < 0 {
		t.Fatalf("could not find formatName in helpers.php")
	}
	pos := protocol.Position{Line: 1, Character: col}

	locs := a.FindAllReferences("file:///project/helpers.php", helperSrc, pos, reader)

	for _, loc := range locs {
		if strings.HasPrefix(loc.URI, "builtin://") {
			t.Errorf("found reference with builtin:// URI: %s", loc.URI)
		}
	}

	// Sanity check: we do get references from the project files
	if len(locs) == 0 {
		t.Fatalf("expected at least one reference to formatName in project files, got none")
	}
}
