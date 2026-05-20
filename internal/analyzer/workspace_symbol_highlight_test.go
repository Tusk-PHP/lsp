package analyzer

import (
	"testing"

	"github.com/open-southeners/tusk-php/internal/protocol"
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
