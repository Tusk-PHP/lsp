package analyzer

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestGetWorkspaceSymbols(t *testing.T) {
	sources := map[string]string{
		"file:///workspace/User.php": `<?php
namespace App\Models;
class User {}
`,
		"file:///workspace/UserController.php": `<?php
namespace App\Http;
use App\Models\User;
class UserController {
    public function show(User $user): User {
        return $user;
    }
}
`,
	}
	a, _ := setupRenameAnalyzer(sources)

	results := a.GetWorkspaceSymbols("User")
	if len(results) == 0 {
		t.Fatal("expected workspace symbol results")
	}
	if results[0].Name != "User" {
		t.Fatalf("expected exact class match first, got %q", results[0].Name)
	}
	if results[0].ContainerName != "App\\Models" {
		t.Fatalf("expected App\\Models container, got %q", results[0].ContainerName)
	}

	fqnResults := a.GetWorkspaceSymbols("App\\Models\\U")
	if len(fqnResults) == 0 {
		t.Fatal("expected FQN-prefix workspace symbol results")
	}
	if got := fqnResults[0].Location.URI; got != "file:///workspace/User.php" {
		t.Fatalf("expected User.php result first, got %q", got)
	}
}

func manyClassSources(names []string) map[string]string {
	sources := make(map[string]string, len(names))
	for _, name := range names {
		sources["file:///workspace/"+name+".php"] = "<?php\nnamespace App;\nclass " + name + " {}\n"
	}
	return sources
}

func TestGetWorkspaceSymbolsKeepsTheBestMatchesUpToTheLimit(t *testing.T) {
	names := []string{"Item"}
	for i := range maxWorkspaceSymbols + 100 {
		names = append(names, fmt.Sprintf("ITEM%03d", i))
	}
	a, _ := setupRenameAnalyzer(manyClassSources(names))

	results := a.GetWorkspaceSymbols("item")
	if len(results) != maxWorkspaceSymbols {
		t.Fatalf("got %d results, want the limit, %d", len(results), maxWorkspaceSymbols)
	}
	if results[0].Name != "Item" {
		t.Errorf("the exact match sorts after every ITEM… name by name but must come first, got %q", results[0].Name)
	}
}

func TestGetWorkspaceSymbolsEmptyQueryReturnsNothing(t *testing.T) {
	a, _ := setupRenameAnalyzer(manyClassSources([]string{"User", "Order"}))
	for _, query := range []string{"", "  "} {
		if results := a.GetWorkspaceSymbols(query); results == nil || len(results) != 0 {
			t.Errorf("query %q: got %v, want an empty list", query, results)
		}
	}
}

func BenchmarkGetWorkspaceSymbols(b *testing.B) {
	names := make([]string, 0, 20000)
	for i := range 20000 {
		names = append(names, fmt.Sprintf("%c%sService%05d", 'A'+i%26, strings.Repeat("x", i%5), i))
	}
	a, _ := setupRenameAnalyzer(manyClassSources(names))
	for _, query := range []string{"", "A", "Axx"} {
		b.Run(fmt.Sprintf("query=%q", query), func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				a.GetWorkspaceSymbols(query)
			}
		})
	}
}

func TestGetDocumentHighlightsVariable(t *testing.T) {
	source := `<?php
function run() {
    $count = 1;
    echo $count;
    $count++;
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	highlights := a.GetDocumentHighlights("file:///test.php", source, protocol.Position{Line: 2, Character: 7})
	if len(highlights) != 3 {
		t.Fatalf("expected 3 variable highlights, got %d", len(highlights))
	}
	if highlights[0].Kind != protocol.DocumentHighlightKindWrite {
		t.Fatalf("expected declaration highlight to be write, got %d", highlights[0].Kind)
	}
}

func TestGetDocumentHighlightsProperty(t *testing.T) {
	source := `<?php
class User {
    public string $name;

    public function label(User $other): string {
        return $this->name . $other->name;
    }
}
`
	a, _ := setupRenameAnalyzer(map[string]string{"file:///test.php": source})

	highlights := a.GetDocumentHighlights("file:///test.php", source, protocol.Position{Line: 2, Character: 19})
	if len(highlights) != 3 {
		t.Fatalf("expected 3 property highlights, got %d", len(highlights))
	}
}
