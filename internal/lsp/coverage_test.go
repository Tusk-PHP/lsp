package lsp

import (
	"strings"
	"testing"
	"time"
)

// initHarness creates an initialized harness ready for handler tests.
func initHarness(t *testing.T) *lspHarness {
	t.Helper()
	h := newHarness(t)
	root := testdataPath()
	initID := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
		"processId":    nil,
	})
	h.readResponse(initID)
	h.notify("initialized", map[string]interface{}{})
	time.Sleep(500 * time.Millisecond) // let async indexing start
	return h
}

func TestHandleDidOpenAndChange(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_coverage.php"
	source := `<?php
namespace App;
class Foo {
    public function bar(): string { return "hello"; }
}
`

	// didOpen
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	// didChange
	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri, "version": 2},
		"contentChanges": []map[string]interface{}{
			{"text": source + "\n// changed"},
		},
	})
	time.Sleep(100 * time.Millisecond)

	// didClose
	h.notify("textDocument/didClose", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
}

func TestPublishDiagnosticsIncludesParserErrors(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_parser_diags.php"
	source := `<?php
$name = "unterminated;
`

	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})

	msg := h.readNotification("textDocument/publishDiagnostics", 5*time.Second)
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected publishDiagnostics params, got %T", msg["params"])
	}
	if gotURI, _ := params["uri"].(string); gotURI != uri {
		t.Fatalf("expected diagnostics for %q, got %q", uri, gotURI)
	}
	diagnostics, ok := params["diagnostics"].([]interface{})
	if !ok {
		t.Fatalf("expected diagnostics array, got %T", params["diagnostics"])
	}

	found := false
	for _, raw := range diagnostics {
		diag, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if diag["code"] != "syntax-error" || diag["message"] != "unterminated string" {
			continue
		}
		found = true
		if diag["source"] != "tusk-php" {
			t.Fatalf("expected source tusk-php, got %v", diag["source"])
		}
		if severity, _ := diag["severity"].(float64); int(severity) != 1 {
			t.Fatalf("expected error severity 1, got %v", diag["severity"])
		}
		rng, ok := diag["range"].(map[string]interface{})
		if !ok {
			t.Fatalf("expected range map, got %T", diag["range"])
		}
		start, _ := rng["start"].(map[string]interface{})
		end, _ := rng["end"].(map[string]interface{})
		if int(start["line"].(float64)) != 1 || int(start["character"].(float64)) != 8 {
			t.Fatalf("expected start 1:8, got %v", start)
		}
		if int(end["line"].(float64)) != 1 || int(end["character"].(float64)) != 9 {
			t.Fatalf("expected end 1:9, got %v", end)
		}
	}
	if !found {
		t.Fatalf("expected syntax-error publishDiagnostics entry, got %#v", diagnostics)
	}
}

func TestPublishDiagnosticsIncludesStructuralParserErrorsOnChange(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_parser_change_diags.php"
	initial := "<?php\nclass Foo {}\n"
	changed := `<?php
class Foo {
    public function bar($name {
        return $name;
    }
}
`

	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": initial,
		},
	})
	_ = h.readNotification("textDocument/publishDiagnostics", 5*time.Second)

	h.notify("textDocument/didChange", map[string]interface{}{
		"textDocument":   map[string]interface{}{"uri": uri, "version": 2},
		"contentChanges": []map[string]interface{}{{"text": changed}},
	})

	msg := h.readNotification("textDocument/publishDiagnostics", 5*time.Second)
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected publishDiagnostics params, got %T", msg["params"])
	}
	diagnostics, ok := params["diagnostics"].([]interface{})
	if !ok {
		t.Fatalf("expected diagnostics array, got %T", params["diagnostics"])
	}

	found := false
	for _, raw := range diagnostics {
		diag, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if diag["code"] == "syntax-error" && diag["message"] == "expected ')'" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected structural syntax-error publishDiagnostics entry, got %#v", diagnostics)
	}
}

func TestPublishDiagnosticsIncludesSprint3BaselineDiagnostics(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_sprint3_baseline.php"
	source := `<?php
namespace App;

class Known {
    public function run(): void {}
}

function helper(): void {}

new MissingClass();
missing_function();
Known::missingMethod();

class Broken {
    public function bad($value {
        return $value;
    }
}
`

	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})

	msg := h.readNotification("textDocument/publishDiagnostics", 5*time.Second)
	params, ok := msg["params"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected publishDiagnostics params, got %T", msg["params"])
	}
	diagnostics, ok := params["diagnostics"].([]interface{})
	if !ok {
		t.Fatalf("expected diagnostics array, got %T", params["diagnostics"])
	}

	want := map[string]bool{
		"syntax-error":     false,
		"unknown-class":    false,
		"unknown-function": false,
		"unknown-member":   false,
	}
	for _, raw := range diagnostics {
		diag, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		code, _ := diag["code"].(string)
		if _, ok := want[code]; ok {
			want[code] = true
		}
	}
	for code, found := range want {
		if !found {
			t.Fatalf("expected publishDiagnostics entry for %s, got %#v", code, diagnostics)
		}
	}
}

func TestHandleCompletion(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_completion.php"
	source := `<?php
use Monolog\Logger;
$log = new Logger('test');
$log->
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/completion", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 3, "character": 6},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("completion returned error: %v", resp["error"])
	}
}

func TestHandleDefinition(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_def.php"
	source := `<?php
use Monolog\Logger;
$log = new Logger('test');
$log->info("hello");
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/definition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 3, "character": 7},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("definition returned error: %v", resp["error"])
	}
}

func TestHandleTypeDefinition(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_type_def.php"
	source := `<?php
use Monolog\Handler\StreamHandler;
$handler = new StreamHandler();
$handler->handle(["message" => "hello"]);
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/typeDefinition", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 3, "character": 1},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("typeDefinition returned error: %v", resp["error"])
	}
	results, ok := resp["result"].([]interface{})
	if !ok || len(results) == 0 {
		t.Fatalf("expected typeDefinition results, got %#v", resp["result"])
	}
	first, _ := results[0].(map[string]interface{})
	if uri, _ := first["uri"].(string); !strings.Contains(uri, "StreamHandler.php") {
		t.Fatalf("expected StreamHandler type definition, got %v", first["uri"])
	}
}

func TestHandleImplementation(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_impl.php"
	source := `<?php
interface CacheStore {
    public function get(string $key): string;
}

abstract class BaseStore implements CacheStore {}

class RedisStore extends BaseStore {
    public function get(string $key): string { return $key; }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/implementation", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 1, "character": 11},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("implementation returned error: %v", resp["error"])
	}
	results, ok := resp["result"].([]interface{})
	if !ok || len(results) == 0 {
		t.Fatalf("expected implementation results, got %#v", resp["result"])
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 implementation result, got %d", len(results))
	}
	first, _ := results[0].(map[string]interface{})
	if gotURI, _ := first["uri"].(string); gotURI != uri {
		t.Fatalf("expected implementation result in opened document, got %v", first["uri"])
	}
}

func TestHandleImplementationIncludesEnumImplementors(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_enum_impl.php"
	source := `<?php
interface StatusLabel {
    public function label(): string;
}

enum Status: string implements StatusLabel {
    case Draft = 'draft';
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/implementation", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 1, "character": 11},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("implementation returned error: %v", resp["error"])
	}
	results, ok := resp["result"].([]interface{})
	if !ok || len(results) != 1 {
		t.Fatalf("expected 1 enum implementation result, got %#v", resp["result"])
	}
	first, _ := results[0].(map[string]interface{})
	if gotURI, _ := first["uri"].(string); gotURI != uri {
		t.Fatalf("expected enum implementation result in opened document, got %v", first["uri"])
	}
}

func TestHandleReferences(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_refs.php"
	source := `<?php
class Foo {
    public function bar(): void {}
    public function baz(): void {
        $this->bar();
    }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/references", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 2, "character": 22},
		"context":      map[string]interface{}{"includeDeclaration": true},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("references returned error: %v", resp["error"])
	}
}

func TestHandleDocumentHighlight(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_highlight.php"
	source := `<?php
function run() {
    $count = 1;
    echo $count;
    $count++;
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/documentHighlight", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 2, "character": 7},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("documentHighlight returned error: %v", resp["error"])
	}
	result, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected highlight array result, got %T", resp["result"])
	}
	if len(result) != 3 {
		t.Fatalf("expected 3 highlights, got %d", len(result))
	}
}

func TestHandleWorkspaceSymbol(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	id := h.send("workspace/symbol", map[string]interface{}{"query": "User"})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("workspace/symbol returned error: %v", resp["error"])
	}
	result, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected workspace symbol array result, got %T", resp["result"])
	}
	if len(result) == 0 {
		t.Fatal("expected at least one workspace symbol result")
	}
	first, ok := result[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected first workspace symbol result object, got %T", result[0])
	}
	if first["name"] == "" {
		t.Fatal("expected workspace symbol name")
	}
}

func TestHandleDocumentSymbol(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_syms.php"
	source := `<?php
class Foo {
    public string $name;
    public function bar(): void {}
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/documentSymbol", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("documentSymbol returned error: %v", resp["error"])
	}
	if resp["result"] == nil {
		t.Error("expected non-nil result for document symbols")
	}
}

func TestHandleFoldingRange(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_folding.php"
	source := `<?php
namespace App;

class Foo {
    public function bar(): array {
        $items = [
            'a' => 1,
            'b' => 2,
        ];
        return $items;
    }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/foldingRange", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("foldingRange returned error: %v", resp["error"])
	}

	ranges, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected folding ranges array, got %T", resp["result"])
	}
	assertHasFoldRange(t, ranges, 1, 11, "")
	assertHasFoldRange(t, ranges, 3, 10, "")
	assertHasFoldRange(t, ranges, 4, 9, "")
	assertHasFoldRange(t, ranges, 5, 7, "region")
}

func TestHandleSignatureHelp(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_sig.php"
	source := `<?php
function greet(string $name, int $age): string {
    return "$name is $age";
}
greet(
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/signatureHelp", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 4, "character": 6},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("signatureHelp returned error: %v", resp["error"])
	}
}

func assertHasFoldRange(t *testing.T, ranges []interface{}, startLine, endLine int, kind string) {
	t.Helper()
	for _, raw := range ranges {
		rng, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		gotStart, startOK := rng["startLine"].(float64)
		gotEnd, endOK := rng["endLine"].(float64)
		if !startOK || !endOK {
			continue
		}
		gotKind, _ := rng["kind"].(string)
		if int(gotStart) == startLine && int(gotEnd) == endLine && gotKind == kind {
			return
		}
	}
	t.Fatalf("expected folding range %d-%d kind=%q, got %#v", startLine, endLine, kind, ranges)
}

func TestHandleCodeAction(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_action.php"
	source := `<?php
namespace App\Models;
class User {}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 2, "character": 0},
			"end":   map[string]interface{}{"line": 2, "character": 12},
		},
		"context": map[string]interface{}{"diagnostics": []interface{}{}},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("codeAction returned error: %v", resp["error"])
	}
}

func TestHandleCodeActionGenerateConstructor(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_constructor_action.php"
	source := `<?php
namespace App\Models;

class User {
    private string $name;
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 3, "character": 0},
			"end":   map[string]interface{}{"line": 3, "character": 10},
		},
		"context": map[string]interface{}{
			"diagnostics": []interface{}{},
			"only":        []string{"refactor"},
		},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("codeAction returned error: %v", resp["error"])
	}

	actions, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected code actions array, got %T", resp["result"])
	}

	var constructor map[string]interface{}
	for _, raw := range actions {
		action, ok := raw.(map[string]interface{})
		if !ok {
			continue
		}
		if action["title"] == "Generate Constructor" {
			constructor = action
			break
		}
	}
	if constructor == nil {
		t.Fatalf("expected Generate Constructor action, got %#v", actions)
	}

	edit, ok := constructor["edit"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected workspace edit, got %#v", constructor)
	}
	changes, ok := edit["changes"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected edit changes map, got %#v", edit)
	}
	fileChanges, ok := changes[uri].([]interface{})
	if !ok || len(fileChanges) != 1 {
		t.Fatalf("expected one file edit, got %#v", changes[uri])
	}
	textEdit, ok := fileChanges[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected text edit object, got %#v", fileChanges[0])
	}
	newText, _ := textEdit["newText"].(string)
	if !strings.Contains(newText, "public function __construct(string $name)") {
		t.Fatalf("expected constructor edit text, got %q", newText)
	}
}

func TestHandleCodeActionFiltersByOnlyKinds(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_action_filters.php"
	source := `<?php
namespace App\Models;
class User {}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	diagnostics := []interface{}{
		map[string]interface{}{
			"range": map[string]interface{}{
				"start": map[string]interface{}{"line": 2, "character": 0},
				"end":   map[string]interface{}{"line": 2, "character": 5},
			},
			"message":  "unused import",
			"severity": 2,
			"source":   "tusk-php",
			"code":     "unused-import",
		},
	}

	sourceOnlyID := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 2, "character": 0},
			"end":   map[string]interface{}{"line": 2, "character": 12},
		},
		"context": map[string]interface{}{
			"diagnostics": diagnostics,
			"only":        []string{"source"},
		},
	})
	sourceOnlyResp := h.readResponse(sourceOnlyID)
	if sourceOnlyResp["error"] != nil {
		t.Fatalf("source-only codeAction returned error: %v", sourceOnlyResp["error"])
	}
	sourceActions, ok := sourceOnlyResp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected source-only result array, got %T", sourceOnlyResp["result"])
	}
	if len(sourceActions) == 0 {
		t.Fatal("expected at least one source action")
	}
	for _, raw := range sourceActions {
		action, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected code action object, got %T", raw)
		}
		kind, _ := action["kind"].(string)
		if kind != "source" && !strings.HasPrefix(kind, "source.") {
			t.Fatalf("expected only source actions, got kind %q in %#v", kind, action)
		}
	}

	refactorOnlyID := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 2, "character": 0},
			"end":   map[string]interface{}{"line": 2, "character": 12},
		},
		"context": map[string]interface{}{
			"diagnostics": diagnostics,
			"only":        []string{"refactor"},
		},
	})
	refactorOnlyResp := h.readResponse(refactorOnlyID)
	if refactorOnlyResp["error"] != nil {
		t.Fatalf("refactor-only codeAction returned error: %v", refactorOnlyResp["error"])
	}
	refactorActions, ok := refactorOnlyResp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected refactor-only result array, got %T", refactorOnlyResp["result"])
	}
	if len(refactorActions) == 0 {
		t.Fatal("expected at least one refactor action")
	}
	for _, raw := range refactorActions {
		action, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected code action object, got %T", raw)
		}
		kind, _ := action["kind"].(string)
		if kind != "refactor" && !strings.HasPrefix(kind, "refactor.") {
			t.Fatalf("expected only refactor actions, got kind %q in %#v", kind, action)
		}
	}

	quickfixOnlyID := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 2, "character": 0},
			"end":   map[string]interface{}{"line": 2, "character": 12},
		},
		"context": map[string]interface{}{
			"diagnostics": diagnostics,
			"only":        []string{"quickfix"},
		},
	})
	quickfixOnlyResp := h.readResponse(quickfixOnlyID)
	if quickfixOnlyResp["error"] != nil {
		t.Fatalf("quickfix-only codeAction returned error: %v", quickfixOnlyResp["error"])
	}
	quickfixActions, ok := quickfixOnlyResp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected quickfix-only result array, got %T", quickfixOnlyResp["result"])
	}
	if len(quickfixActions) != 0 {
		t.Fatalf("expected no quickfix actions before analyzer implementation, got %#v", quickfixActions)
	}
}

func TestHandleCodeActionExtractVariable(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_extract.php"
	source := `<?php
function run($user) {
    return strtoupper($user);
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/codeAction", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 2, "character": 11},
			"end":   map[string]interface{}{"line": 2, "character": 28},
		},
		"context": map[string]interface{}{
			"diagnostics": []interface{}{},
			"only":        []string{"refactor.extract"},
		},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("codeAction returned error: %v", resp["error"])
	}

	actions, ok := resp["result"].([]interface{})
	if !ok {
		t.Fatalf("expected result array, got %T", resp["result"])
	}
	for _, raw := range actions {
		action, ok := raw.(map[string]interface{})
		if !ok {
			t.Fatalf("expected code action object, got %T", raw)
		}
		if kind, _ := action["kind"].(string); kind == "refactor.extract" {
			return
		}
	}

	t.Fatal("expected refactor.extract code action")
}

func TestHandlePrepareRename(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_rename.php"
	source := `<?php
class Foo {
    public function bar(): void {}
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/prepareRename", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 2, "character": 22},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("prepareRename returned error: %v", resp["error"])
	}
}

func TestHandleRename(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_rename2.php"
	source := `<?php
class Foo {
    public function bar(): void {}
    public function baz(): void {
        $this->bar();
    }
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("textDocument/rename", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
		"position":     map[string]interface{}{"line": 2, "character": 22},
		"newName":      "newBar",
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("rename returned error: %v", resp["error"])
	}
}

func TestHandleExecuteCommand(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_cmd.php"
	source := `<?php
namespace App\Models;
class User {}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(200 * time.Millisecond)

	id := h.send("workspace/executeCommand", map[string]interface{}{
		"command":   "tuskPhpLsp.copyNamespace",
		"arguments": []interface{}{uri},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Errorf("executeCommand returned error: %v", resp["error"])
	}
}

func TestHandleDidSave(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_save.php"
	source := `<?php
class Foo {
    public function bar(): void {}
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri": uri, "languageId": "php", "version": 1, "text": source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	// didSave with no text (IncludeText: false)
	h.notify("textDocument/didSave", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	time.Sleep(200 * time.Millisecond)
}
