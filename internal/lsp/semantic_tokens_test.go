package lsp

import (
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/document"
)

func TestHandleSemanticTokensFull(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_semantic_tokens.php"
	source := `<?php
namespace App;

// This is a comment
class UserService {
    private string $name;

    public function getName(): string {
        return $this->name; // inline comment
    }

    public static function create(string $name): static {
        $instance = new static();
        return $instance;
    }
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

	id := h.send("textDocument/semanticTokens/full", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("semanticTokens/full returned error: %v", resp["error"])
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T: %v", resp["result"], resp["result"])
	}

	data, ok := result["data"].([]interface{})
	if !ok {
		t.Fatalf("expected data array in result, got %T", result["data"])
	}

	if len(data) == 0 {
		t.Fatal("expected non-empty semantic token data array")
	}

	if len(data)%5 != 0 {
		t.Errorf("expected data length to be divisible by 5 (each token is 5 uints), got %d", len(data))
	}
}

func TestHandleSemanticTokensFullEmptyDocument(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_semantic_empty.php"
	source := `<?php
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

	id := h.send("textDocument/semanticTokens/full", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("semanticTokens/full on empty document returned error: %v", resp["error"])
	}

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp["result"])
	}
	// data may be nil or empty for an empty document — both are acceptable
	if result["data"] != nil {
		data, _ := result["data"].([]interface{})
		if len(data)%5 != 0 {
			t.Errorf("expected data length divisible by 5, got %d", len(data))
		}
	}
}

func TestHandleSemanticTokensAttributeDecorator(t *testing.T) {
	h := initHarness(t)
	defer h.close()

	uri := "file:///tmp/test_semantic_attributes.php"
	source := `<?php
namespace App;

#[Route]
class UserController {
    public function index(): void {
        @file_get_contents('/tmp/x');
    }
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

	id := h.send("textDocument/semanticTokens/full", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": uri},
	})
	resp := h.readResponse(id)
	if resp["error"] != nil {
		t.Fatalf("semanticTokens/full returned error: %v", resp["error"])
	}
	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected result map, got %T", resp["result"])
	}
	data, ok := result["data"].([]interface{})
	if !ok || len(data)%5 != 0 {
		t.Fatalf("expected data array divisible by 5, got %T len %d", result["data"], len(data))
	}

	// Decode relative encoding into absolute (line, char, type) triples.
	type decoded struct{ line, char, length, typ int }
	var toks []decoded
	line, char := 0, 0
	for i := 0; i+4 < len(data); i += 5 {
		dl := int(data[i].(float64))
		dc := int(data[i+1].(float64))
		if dl > 0 {
			line += dl
			char = dc
		} else {
			char += dc
		}
		toks = append(toks, decoded{line, char, int(data[i+2].(float64)), int(data[i+3].(float64))})
	}

	var decorators []decoded
	for _, tk := range toks {
		if tk.typ == semTokDecorator {
			decorators = append(decorators, tk)
		}
	}
	// Exactly one decorator: `Route` on line 3 (0-based), after `#[`.
	if len(decorators) != 1 {
		t.Fatalf("expected exactly 1 decorator token for #[Route], got %d: %v", len(decorators), decorators)
	}
	if decorators[0].line != 3 || decorators[0].length != len("Route") {
		t.Errorf("expected decorator at line 3 with length 5 (Route), got %+v", decorators[0])
	}
	// The @ error-suppression on file_get_contents must NOT produce a decorator
	// (guarded by the exactly-one assertion above; line 6 would be a second hit).
}

func TestHandleSemanticTokensFullAdvertisedInCapabilities(t *testing.T) {
	h := newHarness(t)
	defer h.close()

	root := testdataPath()
	initID := h.send("initialize", map[string]interface{}{
		"rootUri":      "file://" + root,
		"capabilities": map[string]interface{}{},
		"processId":    nil,
	})
	resp := h.readResponse(initID)

	result, ok := resp["result"].(map[string]interface{})
	if !ok {
		t.Fatalf("initialize: expected result map, got %T", resp["result"])
	}
	caps, ok := result["capabilities"].(map[string]interface{})
	if !ok {
		t.Fatal("initialize: expected capabilities map")
	}

	semTokenProv, ok := caps["semanticTokensProvider"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected semanticTokensProvider in capabilities, got %T", caps["semanticTokensProvider"])
	}

	if semTokenProv["full"] != true {
		t.Errorf("expected semanticTokensProvider.full to be true, got %v", semTokenProv["full"])
	}

	legend, ok := semTokenProv["legend"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected legend in semanticTokensProvider, got %T", semTokenProv["legend"])
	}

	tokenTypes, ok := legend["tokenTypes"].([]interface{})
	if !ok || len(tokenTypes) == 0 {
		t.Fatalf("expected non-empty tokenTypes in legend, got %T: %v", legend["tokenTypes"], legend["tokenTypes"])
	}

	// Verify key token types are present
	typeSet := make(map[string]bool)
	for _, tt := range tokenTypes {
		if s, ok := tt.(string); ok {
			typeSet[s] = true
		}
	}
	for _, expected := range []string{"class", "method", "function", "variable", "keyword", "comment", "string"} {
		if !typeSet[expected] {
			t.Errorf("expected token type %q in legend, got %v", expected, tokenTypes)
		}
	}
}

func TestBuildSemanticTokensReadsTheLexer(t *testing.T) {
	source := "<?php\nnamespace App\\Http;\n\n#[ORM\\Entity]\nclass A {\n    public private(set) int $n = 0x1F;\n    function f() { $o->class = `ls`; return fn() => 1.5e3; }\n}\n"
	got := decodedSemanticTokens(source)
	for key, want := range map[string]uint32{
		"1:Http":         semTokNamespace,
		"3:ORM\\Entity":  semTokDecorator,
		"4:class":        semTokKeyword,
		"5:private(set)": semTokKeyword,
		"5:0x1F":         semTokNumber,
		"6:`ls`":         semTokString,
		"6:fn":           semTokKeyword,
		"6:1.5e3":        semTokNumber,
		"6:$o":           semTokVariable,
	} {
		if tok, ok := got[key]; !ok || tok.tokenType != want {
			t.Errorf("%q: type %d (found %v), want %d", key, tok.tokenType, ok, want)
		}
	}
	for _, key := range []string{"6:class", "1:App\\Ht"} {
		if _, ok := got[key]; ok {
			t.Errorf("%q: unexpected token", key)
		}
	}
}

func decodedSemanticTokens(source string) map[string]semToken {
	lines := strings.Split(source, "\n")
	got := map[string]semToken{}
	data := buildSemanticTokens(source, document.NewColumns(source, document.UTF8))
	line, char := 0, 0
	for i := 0; i+4 < len(data); i += 5 {
		if data[i] > 0 {
			line += int(data[i])
			char = 0
		}
		char += int(data[i+1])
		got[strconv.Itoa(line)+":"+lines[line][char:char+int(data[i+2])]] = semToken{line: line, startChar: char, length: int(data[i+2]), tokenType: data[i+3], modifiers: data[i+4]}
	}
	return got
}

func TestBuildSemanticTokensDeclarationsFromTheSyntaxTree(t *testing.T) {
	source := `<?php
final class
    Spaced { public function __construct(private int $id, int $plain) {} }
interface Shape { public static function make(); }
trait Greets { public string $greeting = 'hi'; public static function hi() {} }
enum Suit { case Hearts; public static function wild(): self { return self::Hearts; } }
function helper(
    int $first,
    string ...$rest,
) {}
$o = new class { public function anonymous() {} };
function () { function nested() {} };
`
	declared := uint32(semModDeclaration)
	static := declared | semModStatic
	got := decodedSemanticTokens(source)
	for key, want := range map[string][2]uint32{
		"2:Spaced":     {semTokClass, declared},
		"2:$id":        {semTokProperty, declared},
		"3:Shape":      {semTokInterface, declared},
		"3:make":       {semTokMethod, static},
		"4:Greets":     {semTokClass, declared},
		"4:$greeting":  {semTokProperty, declared},
		"4:hi":         {semTokMethod, static},
		"5:Suit":       {semTokEnum, declared},
		"5:Hearts":     {semTokEnumMember, declared},
		"5:wild":       {semTokMethod, static},
		"6:helper":     {semTokFunction, declared},
		"7:$first":     {semTokParameter, declared},
		"8:$rest":      {semTokParameter, declared},
		"10:anonymous": {semTokMethod, declared},
		"11:nested":    {semTokFunction, declared},
	} {
		if tok, ok := got[key]; !ok || tok.tokenType != want[0] || tok.modifiers != want[1] {
			t.Errorf("%q: %+v (found %v), want type %d modifiers %d", key, tok, ok, want[0], want[1])
		}
	}
	if tok := got["2:$plain"]; tok.tokenType != semTokVariable {
		t.Errorf("a parameter that is not promoted is not a property: %+v", tok)
	}
	for key := range got {
		if strings.HasPrefix(key, "1:") && key != "1:final" && key != "1:class" {
			t.Errorf("%q: a declaration is reported on the keyword's line", key)
		}
	}
}

func TestBuildSemanticTokensSkipsDeclarationsWithoutAName(t *testing.T) {
	source := "<?php class A { public function () {} function __construct(private int) {} }\nfunction f(int"
	got := decodedSemanticTokens(source)
	for key, want := range map[string]uint32{"0:A": semTokClass, "0:__construct": semTokMethod, "1:f": semTokFunction} {
		if tok, ok := got[key]; !ok || tok.tokenType != want {
			t.Errorf("%q: %+v (found %v), want type %d", key, tok, ok, want)
		}
	}
	for key, tok := range got {
		if tok.tokenType == semTokProperty || tok.tokenType == semTokParameter {
			t.Errorf("%q: a declaration without a name: %+v", key, tok)
		}
	}
}
