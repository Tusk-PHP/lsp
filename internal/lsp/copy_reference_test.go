package lsp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// posInSource returns the (line, character) of the first occurrence of needle
// in src. The returned position points at the first character of needle.
// Panics if needle is not found (test helper).
func posInSource(src, needle string) (line, character int) {
	lines := strings.Split(src, "\n")
	for l, ln := range lines {
		if i := strings.Index(ln, needle); i >= 0 {
			return l, i
		}
	}
	panic("posInSource: needle not found: " + needle)
}

// --- Direct analyzer tests (no LSP harness) ---

func TestCopyReferenceClassSymbol(t *testing.T) {
	idx := symbols.NewIndex()
	uri := "file:///tmp/copy_ref_class.php"
	src := `<?php
namespace App\Models;

class User {
}
`
	idx.IndexFile(uri, src)
	a := analyzer.NewAnalyzer(idx, nil)

	// Position the cursor on "User" in the class declaration.
	line, char := posInSource(src, "User")
	pos := protocol.Position{Line: line, Character: char + 1}

	variants := a.CopyReference(uri, src, pos)
	if len(variants) != 2 {
		t.Fatalf("expected 2 variants for a class, got %d: %v", len(variants), variants)
	}

	want := []struct{ label, value string }{
		{"FQN", `App\Models\User`},
		{"Short name", "User"},
	}
	for i, w := range want {
		if variants[i].Label != w.label {
			t.Errorf("variant[%d].Label: want %q, got %q", i, w.label, variants[i].Label)
		}
		if variants[i].Value != w.value {
			t.Errorf("variant[%d].Value: want %q, got %q", i, w.value, variants[i].Value)
		}
	}
}

func TestCopyReferenceMethodSymbol(t *testing.T) {
	idx := symbols.NewIndex()
	uri := "file:///tmp/copy_ref_method.php"
	src := `<?php
namespace App\Models;

class User {
    public function save(): bool {}
}
`
	idx.IndexFile(uri, src)
	a := analyzer.NewAnalyzer(idx, nil)

	// Position on "save" in the method declaration.
	line, char := posInSource(src, "save")
	pos := protocol.Position{Line: line, Character: char + 1}

	variants := a.CopyReference(uri, src, pos)
	if len(variants) != 6 {
		t.Fatalf("expected 6 variants for a method, got %d: %v", len(variants), variants)
	}

	want := []struct{ label, value string }{
		{"FQN::member()", `App\Models\User::save()`},
		{"FQN::member(signature)", `App\Models\User::save(): bool`},
		{"Short::member()", "User::save()"},
		{"Short::member(signature)", "User::save(): bool"},
		{"FQN", `App\Models\User`},
		{"Short name", "User"},
	}
	for i, w := range want {
		if variants[i].Label != w.label {
			t.Errorf("variant[%d].Label: want %q, got %q", i, w.label, variants[i].Label)
		}
		if variants[i].Value != w.value {
			t.Errorf("variant[%d].Value: want %q, got %q", i, w.value, variants[i].Value)
		}
	}
}

func TestCopyReferenceMethodNoReturnType(t *testing.T) {
	idx := symbols.NewIndex()
	uri := "file:///tmp/copy_ref_no_ret.php"
	src := `<?php
namespace App\Services;

class Mailer {
    public function send() {}
}
`
	idx.IndexFile(uri, src)
	a := analyzer.NewAnalyzer(idx, nil)

	line, char := posInSource(src, "send")
	pos := protocol.Position{Line: line, Character: char + 1}

	variants := a.CopyReference(uri, src, pos)
	if len(variants) != 6 {
		t.Fatalf("expected 6 variants for a method, got %d: %v", len(variants), variants)
	}

	// Without a return type the signature variants must NOT have ": " appended.
	for _, v := range variants {
		if strings.Contains(v.Label, "signature") {
			if strings.Contains(v.Value, ": ") {
				t.Errorf("signature variant should have no return suffix when ReturnType is empty, got %q", v.Value)
			}
			if !strings.HasSuffix(v.Value, "send()") {
				t.Errorf("signature variant should end with send(), got %q", v.Value)
			}
		}
	}
}

func TestCopyReferenceMethodMultiParam(t *testing.T) {
	idx := symbols.NewIndex()
	uri := "file:///tmp/copy_ref_multi_param.php"
	src := `<?php
namespace App\Http;

class Request {
    public function validate(string $rules, array $messages): array {}
}
`
	idx.IndexFile(uri, src)
	a := analyzer.NewAnalyzer(idx, nil)

	line, char := posInSource(src, "validate")
	pos := protocol.Position{Line: line, Character: char + 1}

	variants := a.CopyReference(uri, src, pos)
	if len(variants) != 6 {
		t.Fatalf("expected 6 variants, got %d: %v", len(variants), variants)
	}

	// The signature variant must include both params separated by ", " and the
	// return type.
	wantSig := `App\Http\Request::validate(string $rules, array $messages): array`
	if variants[1].Value != wantSig {
		t.Errorf("FQN::member(signature) mismatch\n  want: %q\n  got:  %q", wantSig, variants[1].Value)
	}
	wantShortSig := `Request::validate(string $rules, array $messages): array`
	if variants[3].Value != wantShortSig {
		t.Errorf("Short::member(signature) mismatch\n  want: %q\n  got:  %q", wantShortSig, variants[3].Value)
	}
}

// --- LSP harness test (full round-trip through workspace/executeCommand) ---

func TestExecuteCommandCopyReference(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	root := testdataPath()
	initID := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
		"processId":    nil,
	})
	h.readResponse(initID)

	h.notify("initialized", map[string]interface{}{})
	time.Sleep(200 * time.Millisecond)

	uri := "file:///tmp/test_copy_ref.php"
	source := `<?php
namespace App\Models;

class Order {
    public function confirm(string $note): bool {}
}
`
	h.notify("textDocument/didOpen", map[string]interface{}{
		"textDocument": map[string]interface{}{
			"uri":        uri,
			"languageId": "php",
			"version":    1,
			"text":       source,
		},
	})
	time.Sleep(100 * time.Millisecond)

	// Cursor on "Order" (line 3, char 7 — inside the class name).
	classLine, classChar := posInSource(source, "Order")
	classID := h.send("workspace/executeCommand", map[string]interface{}{
		"command": "tuskPhpLsp.copyReference",
		"arguments": []interface{}{
			uri,
			map[string]interface{}{"line": classLine, "character": classChar + 1},
		},
	})
	classResp := h.readResponse(classID)
	if classResp["error"] != nil {
		t.Fatalf("copyReference (class) returned error: %v", classResp["error"])
	}
	classVariants := parseVariants(t, classResp["result"])
	if len(classVariants) != 2 {
		t.Errorf("expected 2 variants for class cursor, got %d: %v", len(classVariants), classVariants)
	} else {
		if classVariants[0]["value"] != `App\Models\Order` {
			t.Errorf("class FQN: want %q, got %q", `App\Models\Order`, classVariants[0]["value"])
		}
		if classVariants[1]["value"] != "Order" {
			t.Errorf("class short name: want %q, got %q", "Order", classVariants[1]["value"])
		}
	}

	// Cursor on "confirm" method.
	methodLine, methodChar := posInSource(source, "confirm")
	methodID := h.send("workspace/executeCommand", map[string]interface{}{
		"command": "tuskPhpLsp.copyReference",
		"arguments": []interface{}{
			uri,
			map[string]interface{}{"line": methodLine, "character": methodChar + 1},
		},
	})
	methodResp := h.readResponse(methodID)
	if methodResp["error"] != nil {
		t.Fatalf("copyReference (method) returned error: %v", methodResp["error"])
	}
	methodVariants := parseVariants(t, methodResp["result"])
	if len(methodVariants) != 6 {
		t.Errorf("expected 6 variants for method cursor, got %d: %v", len(methodVariants), methodVariants)
	} else {
		if methodVariants[0]["value"] != `App\Models\Order::confirm()` {
			t.Errorf("FQN::member(): want %q, got %q", `App\Models\Order::confirm()`, methodVariants[0]["value"])
		}
		wantSig := `App\Models\Order::confirm(string $note): bool`
		if methodVariants[1]["value"] != wantSig {
			t.Errorf("FQN::member(signature): want %q, got %q", wantSig, methodVariants[1]["value"])
		}
	}
}

// parseVariants decodes the JSON array of {"label":"...","value":"..."} objects
// returned by tuskPhpLsp.copyReference.
func parseVariants(t *testing.T, raw interface{}) []map[string]string {
	t.Helper()
	b, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("parseVariants: marshal: %v", err)
	}
	var out []map[string]string
	if err := json.Unmarshal(b, &out); err != nil {
		t.Fatalf("parseVariants: unmarshal: %v", err)
	}
	return out
}
