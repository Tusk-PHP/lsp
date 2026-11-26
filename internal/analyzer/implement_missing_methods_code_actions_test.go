package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestGetCodeActionsImplementMissingMethodsFromInterface(t *testing.T) {
	source := `<?php
namespace App;

class UserRepository implements RepositoryInterface {
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///repo.php": `<?php
namespace App;

interface RepositoryInterface {
    public function find(string $id = 'root'): ?User;
    public function save(User $user): void;
}
`,
		"file:///user.php": `<?php
namespace App;

class User {}
`,
	})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 3, Character: 52},
		},
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})

	var action *protocol.CodeAction
	for i := range actions {
		if actions[i].Title == "Implement missing methods" {
			action = &actions[i]
			break
		}
	}
	if action == nil || action.Edit == nil {
		t.Fatalf("expected implement missing methods action, got %#v", actions)
	}

	edits := action.Edit.Changes["file:///test.php"]
	if len(edits) != 1 {
		t.Fatalf("expected one edit, got %#v", edits)
	}
	text := edits[0].NewText
	if !strings.Contains(text, "public function find(string $id = 'root'): ?User") {
		t.Fatalf("missing copied interface signature:\n%s", text)
	}
	if !strings.Contains(text, "public function save(User $user): void") {
		t.Fatalf("missing second interface method:\n%s", text)
	}
	if !strings.Contains(text, "throw new \\BadMethodCallException(__METHOD__ . ' is not implemented.');") {
		t.Fatalf("missing placeholder body:\n%s", text)
	}
}

func TestGetCodeActionsImplementMissingMethodsFromAbstractParent(t *testing.T) {
	source := `<?php
namespace App;

class ConcreteHandler extends BaseHandler {
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///base.php": `<?php
namespace App;

abstract class BaseHandler {
    abstract protected function handle(array $payload): int;
}
`,
	})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 3, Character: 39},
		},
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})

	if len(actions) == 0 {
		t.Fatal("expected refactor action")
	}
	text := actions[0].Edit.Changes["file:///test.php"][0].NewText
	if strings.Contains(text, "abstract protected function") {
		t.Fatalf("generated method should not stay abstract:\n%s", text)
	}
	if !strings.Contains(text, "protected function handle(array $payload): int") {
		t.Fatalf("missing abstract parent signature:\n%s", text)
	}
}

func TestGetCodeActionsImplementMissingMethodsSkipsNonDeclarationLine(t *testing.T) {
	source := `<?php
namespace App;

class UserRepository implements RepositoryInterface {
    private string $name;
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///repo.php": `<?php
namespace App;

interface RepositoryInterface {
    public function save(string $id): void;
}
`,
	})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 4, Character: 4},
			End:   protocol.Position{Line: 4, Character: 24},
		},
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})

	for _, action := range actions {
		if action.Title == "Implement missing methods" {
			t.Fatalf("did not expect action away from declaration line: %#v", actions)
		}
	}
}

func TestGetCodeActionsImplementMissingMethodsFromExtendedInterfaces(t *testing.T) {
	source := `<?php
namespace App;

class FileStore implements Store {
}
`

	a, _ := setupRenameAnalyzer(map[string]string{
		"file:///test.php": source,
		"file:///store.php": `<?php
namespace App;

use App\Contracts\Readable;

interface Store extends Readable, Writable {
    public function flush(): void;
}

interface Writable {
    public function write(string $key): void;
}
`,
		"file:///readable.php": `<?php
namespace App\Contracts;

interface Readable {
    public function read(string $key): string;
}
`,
	})

	actions := a.GetCodeActions("file:///test.php", source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: "file:///test.php"},
		Range: protocol.Range{
			Start: protocol.Position{Line: 3, Character: 0},
			End:   protocol.Position{Line: 3, Character: 36},
		},
		Context: protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})

	var text string
	for _, action := range actions {
		if action.Title == "Implement missing methods" && action.Edit != nil {
			for _, edit := range action.Edit.Changes["file:///test.php"] {
				text += edit.NewText
			}
		}
	}
	for _, want := range []string{
		"public function flush(): void",
		"public function read(string $key): string",
		"public function write(string $key): void",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("the stub implements %q from the interface or one it extends:\n%s", want, text)
		}
	}
}

func implementMissingMethodsText(t *testing.T, sources map[string]string, uri string, line int) string {
	t.Helper()
	a, _ := setupRenameAnalyzer(sources)
	source := sources[uri]
	actions := a.GetCodeActions(uri, source, protocol.CodeActionParams{
		TextDocument: protocol.TextDocumentIdentifier{URI: uri},
		Range:        protocol.Range{Start: protocol.Position{Line: line, Character: 0}, End: protocol.Position{Line: line, Character: 1}},
		Context:      protocol.CodeActionContext{Only: []protocol.CodeActionKind{"refactor"}},
	})
	for _, action := range actions {
		if action.Title != "Implement missing methods" || action.Edit == nil {
			continue
		}
		edits := action.Edit.Changes[uri]
		if len(edits) != 1 {
			t.Fatalf("expected one edit, got %#v", edits)
		}
		return edits[0].NewText
	}
	t.Fatalf("expected implement missing methods action, got %#v", actions)
	return ""
}

func TestGetCodeActionsImplementMissingMethodsFromBuiltinInterfaces(t *testing.T) {
	source := `<?php
namespace App;

class Bag implements \Countable, \ArrayAccess, \JsonSerializable {
}
`
	text := implementMissingMethodsText(t, map[string]string{"file:///test.php": source}, "file:///test.php", 3)
	for _, want := range []string{
		"    public function count(): int\n    {\n        throw new \\BadMethodCallException(__METHOD__ . ' is not implemented.');\n    }",
		"public function jsonSerialize(): mixed",
		"public function offsetExists($offset)",
		"public function offsetGet($offset)",
		"public function offsetSet($offset, $value)",
		"public function offsetUnset($offset)",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
}

func TestGetCodeActionsImplementMissingMethodsRendersTheSignatureForTheTargetFile(t *testing.T) {
	source := `<?php
namespace App\Http;

use Lib\Contracts\Store as StoreContract;
use Lib\Models\Item;

class Shop implements StoreContract {
}
`
	text := implementMissingMethodsText(t, map[string]string{
		"file:///test.php": source,
		"file:///contracts.php": `<?php
namespace Lib\Contracts;

use Lib\Models\Item;
use Lib\Models\Cart;

interface Reads { public static function find(int|string $id, ?Cart $cart = null): ?Item; }
interface Store extends Reads {
    abstract public function put(Item &$item, string ...$tags): void;
    public function fill(\Traversable&\Countable $items, array $options = []): static;
}
`,
		"file:///models.php": "<?php\nnamespace Lib\\Models;\nclass Item {}\nclass Cart {}\n",
	}, "file:///test.php", 6)
	for _, want := range []string{
		"public static function find(int|string $id, ?\\Lib\\Models\\Cart $cart = null): ?Item",
		"public function put(Item &$item, string ...$tags): void",
		"public function fill(\\Traversable&\\Countable $items, array $options = []): static",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q in:\n%s", want, text)
		}
	}
	if strings.Contains(text, "abstract") {
		t.Errorf("an implementation isn't abstract:\n%s", text)
	}
}
