package diagnostics

import (
	"io"
	"log"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func newTestProvider() *Provider {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	logger := log.New(io.Discard, "", 0)
	cfg := config.DefaultConfig()
	// Disable external tools for unit tests
	f := false
	cfg.PHPStanEnabled = &f
	cfg.PintEnabled = &f
	return NewProvider(idx, "none", "/tmp", logger, cfg)
}

func TestStaticDeprecations(t *testing.T) {
	p := newTestProvider()
	source := `<?php
namespace App;
class Foo {
    public function bar(): void {
        $data = each($arr);
        $fn = create_function('$a', 'return $a;');
        $enc = utf8_encode($str);
    }
}
`
	diags := p.Analyze("file:///test.php", source)
	deprecations := filterBySource(diags, "tusk-php")
	deprecations = filterByCode(deprecations, "deprecated")
	if len(deprecations) != 3 {
		t.Fatalf("expected 3 deprecation diagnostics, got %d", len(deprecations))
	}
	for _, d := range deprecations {
		if d.Severity != protocol.DiagnosticSeverityWarning {
			t.Errorf("expected warning severity, got %d", d.Severity)
		}
		if d.Range.Start.Character == 0 && d.Range.End.Character == 0 {
			t.Error("expected non-zero column for deprecation")
		}
	}
}

func TestStaticUnusedImports(t *testing.T) {
	p := newTestProvider()
	source := `<?php
namespace App;
use Some\UnusedClass;
use Some\UsedClass;
class Foo {
    public function bar(UsedClass $x): void {}
}
`
	diags := p.Analyze("file:///test.php", source)
	unused := filterByCode(diags, "unused-import")
	if len(unused) != 1 {
		t.Fatalf("expected 1 unused import diagnostic, got %d", len(unused))
	}
	if unused[0].Severity != protocol.DiagnosticSeverityHint {
		t.Errorf("expected hint severity, got %d", unused[0].Severity)
	}
}

func TestStaticAbstractInConcrete(t *testing.T) {
	p := newTestProvider()
	source := `<?php
class Foo {
    abstract public function bar(): void;
}
`
	diags := p.Analyze("file:///test.php", source)
	abstracts := filterByCode(diags, "abstract-in-concrete")
	if len(abstracts) != 1 {
		t.Fatalf("expected 1 abstract-in-concrete diagnostic, got %d", len(abstracts))
	}
	if abstracts[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected error severity, got %d", abstracts[0].Severity)
	}
}

func TestSprint3BaselineUnknownDiagnostics(t *testing.T) {
	p := newTestProvider()
	p.index.MarkReady()
	p.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return ""
	}

	source := `<?php
namespace App;

class Known {
    public function run(): void {}
}

function helper(): void {}

class Demo {
    public function go(): void {
        new Known();
        new MissingClass();
        helper();
        missing_function();
        Known::run();
        Known::missingMethod();
    }
}
`

	diags := p.Analyze("file:///test.php", source)
	if got := filterByCode(diags, "unknown-class"); len(got) == 0 {
		t.Fatal("expected unknown-class diagnostic")
	}
	if got := filterByCode(diags, "unknown-function"); len(got) == 0 {
		t.Fatal("expected unknown-function diagnostic")
	}
	if got := filterByCode(diags, "unknown-member"); len(got) == 0 {
		t.Fatal("expected unknown-member diagnostic")
	}

	for _, code := range []string{"unknown-class", "unknown-function", "unknown-member"} {
		if len(filterByCode(diags, code)) == 0 {
			t.Fatalf("expected at least one %s diagnostic", code)
		}
	}
}

func TestUnknownFunctionAllowsBuiltinAndLaravelHelperCalls(t *testing.T) {
	p := newTestProvider()
	source := `<?php
class LambdaFunction {}

function build(LambdaFunction $lambdaFunction): string {
    return class_basename(get_class($lambdaFunction));
}
`
	diags := p.Analyze("file:///test.php", source)
	unknownFunctions := filterByCode(diags, "unknown-function")
	if len(unknownFunctions) != 0 {
		t.Fatalf("expected no unknown-function diagnostics, got %#v", unknownFunctions)
	}
}

func TestUnknownDiagnosticsAllowGeneratedPHPBuiltinsAndAbsoluteBuiltinClasses(t *testing.T) {
	p := newTestProvider()
	p.index.IndexFile("file:///vendor/faker/src/Faker/Provider/Base.php", `<?php
namespace Faker\Provider;

class Base {}
`)

	source := `<?php
namespace Faker;

class Generator {}
class UniqueGenerator {}

class Documentor {
    public function getFormatters(): array {
        $providers = array_reverse([]);
        $providers[] = new Provider\Base();
        $refl = new \ReflectionObject($this);
        foreach ($refl->getMethods(\ReflectionMethod::IS_PUBLIC) as $method) {}
        try {
            return $providers;
        } catch (\InvalidArgumentException $e) {
            return [];
        }
    }
}
`

	diags := p.Analyze("file:///vendor/faker/src/Faker/Documentor.php", source)
	if unknownClasses := filterByCode(diags, "unknown-class"); len(unknownClasses) != 0 {
		t.Fatalf("expected no unknown-class diagnostics, got %#v", unknownClasses)
	}
	if unknownFunctions := filterByCode(diags, "unknown-function"); len(unknownFunctions) != 0 {
		t.Fatalf("expected no unknown-function diagnostics, got %#v", unknownFunctions)
	}
}

func TestUnknownFunctionRespectsPHPBuiltinProfile(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})
	idx.MarkReady()
	logger := log.New(io.Discard, "", 0)
	cfg := config.DefaultConfig()
	f := false
	cfg.PHPStanEnabled = &f
	cfg.PintEnabled = &f
	p := NewProvider(idx, "none", "/tmp", logger, cfg)

	source := `<?php
array_reverse([]);
str_contains('haystack', 'needle');
json_validate('{}');
`

	diags := p.Analyze("file:///test.php", source)
	unknownFunctions := filterByCode(diags, "unknown-function")
	if len(unknownFunctions) != 2 {
		t.Fatalf("expected 2 unknown-function diagnostics, got %#v", unknownFunctions)
	}
	if unknownFunctions[0].Message != "Unknown function 'str_contains'" {
		t.Fatalf("expected str_contains diagnostic first, got %q", unknownFunctions[0].Message)
	}
	if unknownFunctions[1].Message != "Unknown function 'json_validate'" {
		t.Fatalf("expected json_validate diagnostic second, got %q", unknownFunctions[1].Message)
	}
}

func TestStaticSyntaxDiagnosticsTokenizerError(t *testing.T) {
	p := newTestProvider()
	source := `<?php
$name = "unterminated;
`
	diags := p.Analyze("file:///test.php", source)
	syntax := filterByCode(diags, "syntax-error")
	if len(syntax) != 1 {
		t.Fatalf("expected 1 syntax diagnostic, got %d: %#v", len(syntax), syntax)
	}
	if syntax[0].Severity != protocol.DiagnosticSeverityError {
		t.Fatalf("expected error severity, got %d", syntax[0].Severity)
	}
	if syntax[0].Message != "unterminated string" {
		t.Fatalf("expected unterminated string message, got %q", syntax[0].Message)
	}
	if syntax[0].Range.Start.Line != 1 || syntax[0].Range.Start.Character != 8 {
		t.Fatalf("expected diagnostic start 1:8, got %d:%d", syntax[0].Range.Start.Line, syntax[0].Range.Start.Character)
	}
	if syntax[0].Range.End.Line != 1 || syntax[0].Range.End.Character != 9 {
		t.Fatalf("expected diagnostic end 1:9, got %d:%d", syntax[0].Range.End.Line, syntax[0].Range.End.Character)
	}
}

func TestStaticSyntaxDiagnosticsUnterminatedComment(t *testing.T) {
	p := newTestProvider()
	source := `<?php
/* never closed
class Foo {}
`
	diags := p.Analyze("file:///test.php", source)
	syntax := filterByCode(diags, "syntax-error")
	if len(syntax) != 1 {
		t.Fatalf("expected 1 syntax diagnostic, got %d: %#v", len(syntax), syntax)
	}
	if syntax[0].Message != "unterminated comment" {
		t.Fatalf("expected unterminated comment message, got %q", syntax[0].Message)
	}
	if syntax[0].Range.Start.Line != 1 || syntax[0].Range.Start.Character != 0 {
		t.Fatalf("expected diagnostic start 1:0, got %d:%d", syntax[0].Range.Start.Line, syntax[0].Range.Start.Character)
	}
	if syntax[0].Range.End.Line != 1 || syntax[0].Range.End.Character != 2 {
		t.Fatalf("expected diagnostic end 1:2, got %d:%d", syntax[0].Range.End.Line, syntax[0].Range.End.Character)
	}
}

func TestStaticSyntaxDiagnosticsStructuralError(t *testing.T) {
	p := newTestProvider()
	source := `<?php
class Foo {
    public function bar($name {
        return $name;
    }
}
`
	diags := p.Analyze("file:///test.php", source)
	syntax := filterByCode(diags, "syntax-error")
	if len(syntax) == 0 {
		t.Fatal("expected syntax diagnostic for missing paren")
	}
	found := false
	for _, diag := range syntax {
		if diag.Message == "expected ')'" {
			found = true
			if diag.Severity != protocol.DiagnosticSeverityError {
				t.Fatalf("expected error severity, got %d", diag.Severity)
			}
			if diag.Range.Start.Line != 2 || diag.Range.Start.Character != 30 {
				t.Fatalf("expected diagnostic start 2:30, got %d:%d", diag.Range.Start.Line, diag.Range.Start.Character)
			}
			if diag.Range.End.Line != 2 || diag.Range.End.Character != 31 {
				t.Fatalf("expected diagnostic end 2:31, got %d:%d", diag.Range.End.Line, diag.Range.End.Character)
			}
		}
	}
	if !found {
		t.Fatalf("expected missing paren diagnostic, got %#v", syntax)
	}
}

func TestAnalyzeIncludesSprint3BaselineDiagnostics(t *testing.T) {
	p := newTestProvider()
	p.index.MarkReady()
	uri := "file:///test.php"
	source := `<?php
$name = "unterminated;
`

	p.mu.Lock()
	p.saveResults[uri] = []protocol.Diagnostic{
		{
			Range:    protocol.Range{Start: protocol.Position{Line: 2, Character: 4}, End: protocol.Position{Line: 2, Character: 15}},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "tusk-php",
			Message:  "Class MissingClass not found",
			Code:     "unknown-class",
		},
		{
			Range:    protocol.Range{Start: protocol.Position{Line: 3, Character: 4}, End: protocol.Position{Line: 3, Character: 12}},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "tusk-php",
			Message:  "Function clients not found",
			Code:     "unknown-function",
		},
		{
			Range:    protocol.Range{Start: protocol.Position{Line: 4, Character: 4}, End: protocol.Position{Line: 4, Character: 15}},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "tusk-php",
			Message:  "Access to property $name on MissingClass",
			Code:     "unknown-member",
		},
	}
	p.mu.Unlock()

	diags := p.Analyze(uri, source)
	if len(diags) != 4 {
		t.Fatalf("expected 4 diagnostics, got %d: %#v", len(diags), diags)
	}

	syntax := filterByCode(diags, "syntax-error")
	if len(syntax) != 1 {
		t.Fatalf("expected 1 syntax diagnostic, got %d: %#v", len(syntax), syntax)
	}
	if syntax[0].Message != "unterminated string" {
		t.Fatalf("expected parser error message, got %q", syntax[0].Message)
	}

	for _, code := range []string{"unknown-class", "unknown-function", "unknown-member"} {
		matches := filterByCode(diags, code)
		if len(matches) != 1 {
			t.Fatalf("expected 1 %s diagnostic, got %d: %#v", code, len(matches), matches)
		}
		if matches[0].Source != "tusk-php" {
			t.Fatalf("expected %s source tusk-php, got %q", code, matches[0].Source)
		}
		if matches[0].Severity != protocol.DiagnosticSeverityError {
			t.Fatalf("expected %s severity error, got %d", code, matches[0].Severity)
		}
	}
}

func TestToolResultsCaching(t *testing.T) {
	p := newTestProvider()
	uri := "file:///test.php"

	// Simulate cached tool results
	p.mu.Lock()
	p.toolResults[uri] = []protocol.Diagnostic{
		{
			Range:    protocol.Range{Start: protocol.Position{Line: 5, Character: 0}},
			Severity: protocol.DiagnosticSeverityError,
			Source:   "phpstan",
			Message:  "Test error",
			Code:     "test.error",
		},
	}
	p.mu.Unlock()

	source := `<?php
class Foo {
    public function bar(): void {}
}
`
	diags := p.Analyze(uri, source)
	phpstanDiags := filterBySource(diags, "phpstan")
	if len(phpstanDiags) != 1 {
		t.Fatalf("expected 1 cached phpstan diagnostic, got %d", len(phpstanDiags))
	}
	if phpstanDiags[0].Message != "Test error" {
		t.Errorf("expected 'Test error', got %q", phpstanDiags[0].Message)
	}

	// Clear cache and verify it's gone
	p.ClearCache(uri)
	diags = p.Analyze(uri, source)
	phpstanDiags = filterBySource(diags, "phpstan")
	if len(phpstanDiags) != 0 {
		t.Errorf("expected 0 phpstan diagnostics after clear, got %d", len(phpstanDiags))
	}
}

func TestPHPStanOutputParsing(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"totals": {"errors": 0, "file_errors": 2},
		"files": {
			"/project/src/Service.php": {
				"errors": 2,
				"messages": [
					{
						"message": "Parameter #1 $message of method info() expects string, int given.",
						"line": 15,
						"ignorable": true,
						"identifier": "argument.type"
					},
					{
						"message": "Call to undefined method Foo::bar().",
						"line": 22,
						"ignorable": true,
						"identifier": "method.notFound"
					}
				]
			}
		}
	}`)

	diags := r.parseOutput(output, nil)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	// First diagnostic
	if diags[0].Range.Start.Line != 14 { // 15 → 14 (0-based)
		t.Errorf("expected line 14, got %d", diags[0].Range.Start.Line)
	}
	if diags[0].Source != "phpstan" {
		t.Errorf("expected source 'phpstan', got %q", diags[0].Source)
	}
	if diags[0].Code != "argument.type" {
		t.Errorf("expected code 'argument.type', got %q", diags[0].Code)
	}
	if diags[0].Severity != protocol.DiagnosticSeverityError {
		t.Errorf("expected error severity, got %d", diags[0].Severity)
	}

	// Second diagnostic
	if diags[1].Range.Start.Line != 21 {
		t.Errorf("expected line 21, got %d", diags[1].Range.Start.Line)
	}
	if diags[1].Code != "method.notFound" {
		t.Errorf("expected code 'method.notFound', got %q", diags[1].Code)
	}
}

func TestPHPStanOutputWithPreamble(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`Note: Using configuration file phpstan.neon.
{"totals":{"errors":0,"file_errors":1},"files":{"/test.php":{"errors":1,"messages":[{"message":"Undefined variable: $x","line":3,"ignorable":true,"identifier":"variable.undefined"}]}}}`)

	diags := r.parseOutput(output, nil)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	if diags[0].Range.Start.Line != 2 {
		t.Errorf("expected line 2, got %d", diags[0].Range.Start.Line)
	}
}

func TestPHPStanDiagnosticRange(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{"totals":{"errors":0,"file_errors":2},"files":{"/test.php":{"errors":2,"messages":[
		{"message":"Function clients not found.","line":5,"ignorable":true,"identifier":"function.notFound"},
		{"message":"Call to undefined method App\\Service::missing().","line":8,"ignorable":true,"identifier":"method.notFound"}
	]}}}`)

	lines := []string{
		"<?php",                             // 0
		"",                                  // 1
		"namespace App\\Http\\Controllers;", // 2
		"",                                  // 3
		"        clients();",                // 4 (line 5 in PHPStan = 0-based 4)
		"",                                  // 5
		"        $svc = new Service();",     // 6
		"        $svc->missing();",          // 7 (line 8 in PHPStan = 0-based 7)
	}

	diags := r.parseOutput(output, lines)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics, got %d", len(diags))
	}

	// "Function clients not found" → should highlight "clients" on line 4
	d0 := diags[0]
	if d0.Range.Start.Line != 4 {
		t.Errorf("d0: expected line 4, got %d", d0.Range.Start.Line)
	}
	// "clients" starts at col 8 in "        clients();"
	if d0.Range.Start.Character != 8 {
		t.Errorf("d0: expected start col 8, got %d", d0.Range.Start.Character)
	}
	if d0.Range.End.Character != 8+len("clients") {
		t.Errorf("d0: expected end col %d, got %d", 8+len("clients"), d0.Range.End.Character)
	}

	// "Call to undefined method App\Service::missing()" → should highlight "missing"
	d1 := diags[1]
	if d1.Range.Start.Line != 7 {
		t.Errorf("d1: expected line 7, got %d", d1.Range.Start.Line)
	}
	// "missing" appears after "$svc->" at col 14 in "        $svc->missing();"
	if d1.Range.Start.Character != 14 {
		t.Errorf("d1: expected start col 14 for 'missing', got %d", d1.Range.Start.Character)
	}
	if d1.Range.End.Character != 14+len("missing") {
		t.Errorf("d1: expected end col %d, got %d", 14+len("missing"), d1.Range.End.Character)
	}
}

func TestPHPStanDiagnosticRangeFallback(t *testing.T) {
	r := &phpstanRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{"totals":{"errors":0,"file_errors":1},"files":{"/test.php":{"errors":1,"messages":[
		{"message":"Some obscure error.","line":2,"ignorable":true,"identifier":"phpstan"}
	]}}}`)

	lines := []string{
		"<?php",                       // 0
		"    $x = something_weird();", // 1 (line 2 in PHPStan = 0-based 1)
	}

	diags := r.parseOutput(output, lines)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}

	// No identifier found in message → should highlight trimmed line content
	d := diags[0]
	if d.Range.Start.Character != 4 { // skip 4 leading spaces
		t.Errorf("expected start col 4, got %d", d.Range.Start.Character)
	}
	if d.Range.End.Character != len("    $x = something_weird();") {
		t.Errorf("expected end col %d, got %d", len("    $x = something_weird();"), d.Range.End.Character)
	}
}

func TestPintDiffParsing(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"files": [
			{
				"name": "src/Service.php",
				"diff": "--- Original\n+++ New\n@@ -3,7 +3,6 @@\n namespace App;\n \n-use Illuminate\\Contracts\\Auth\\MustVerifyEmail;\n use Illuminate\\Database\\Eloquent\\Factories\\HasFactory;\n use Illuminate\\Foundation\\Auth\\User as Authenticatable;\n",
				"appliedFixers": ["no_unused_imports"]
			}
		]
	}`)

	diags := r.parseOutput(output)
	if len(diags) != 1 {
		t.Fatalf("expected 1 diagnostic, got %d", len(diags))
	}
	// The removed line is at original line 5 (hunk starts at 3, context +2)
	// @@ -3,7 +3,6 @@ → starts at original line 3
	// " namespace App;" → context line 3 → origLine becomes 4
	// "" → context line 4 → origLine becomes 5
	// "-use Illuminate..." → removed at origLine 5 → reports line 4 (0-based)
	if diags[0].Range.Start.Line != 4 {
		t.Errorf("expected line 4 (0-based), got %d", diags[0].Range.Start.Line)
	}
	if diags[0].Source != "pint" {
		t.Errorf("expected source 'pint', got %q", diags[0].Source)
	}
	if diags[0].Severity != protocol.DiagnosticSeverityWarning {
		t.Errorf("expected warning severity, got %d", diags[0].Severity)
	}
}

func TestPintMultipleHunks(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"files": [
			{
				"name": "src/Foo.php",
				"diff": "--- Original\n+++ New\n@@ -5,3 +5,3 @@\n class Foo\n-{\n+{\n@@ -10,3 +10,3 @@\n     public function bar()\n-    {\n+    {",
				"appliedFixers": ["braces"]
			}
		]
	}`)

	diags := r.parseOutput(output)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics (one per hunk), got %d", len(diags))
	}
	// First hunk: @@ -5 → origLine 5, context "class Foo" at 5→6, removed at 6 → line 5 (0-based)
	if diags[0].Range.Start.Line != 5 {
		t.Errorf("first diagnostic: expected line 5, got %d", diags[0].Range.Start.Line)
	}
	// Second hunk: @@ -10 → origLine 10, context at 10→11, removed at 11 → line 10 (0-based)
	if diags[1].Range.Start.Line != 10 {
		t.Errorf("second diagnostic: expected line 10, got %d", diags[1].Range.Start.Line)
	}
}

func TestPintNoFixers(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{"files": []}`)
	diags := r.parseOutput(output)
	if len(diags) != 0 {
		t.Errorf("expected 0 diagnostics for clean output, got %d", len(diags))
	}
}

func TestPintWithoutDiff(t *testing.T) {
	r := &pintRunner{logger: log.New(io.Discard, "", 0)}

	output := []byte(`{
		"files": [
			{
				"name": "src/Foo.php",
				"appliedFixers": ["no_unused_imports", "braces"]
			}
		]
	}`)

	diags := r.parseOutput(output)
	if len(diags) != 2 {
		t.Fatalf("expected 2 diagnostics (one per fixer), got %d", len(diags))
	}
	// Without diff, all diagnostics should be at line 0
	for _, d := range diags {
		if d.Range.Start.Line != 0 {
			t.Errorf("expected line 0 for no-diff diagnostic, got %d", d.Range.Start.Line)
		}
	}
}

func TestRunnerAvailability(t *testing.T) {
	logger := log.New(io.Discard, "", 0)

	t.Run("disabled by config", func(t *testing.T) {
		cfg := config.DefaultConfig()
		f := false
		cfg.PHPStanEnabled = &f
		cfg.PintEnabled = &f
		r1 := newPHPStanRunner("/nonexistent", cfg, logger)
		r2 := newPintRunner("/nonexistent", cfg, logger)
		if r1.available() {
			t.Error("phpstan should not be available when disabled")
		}
		if r2.available() {
			t.Error("pint should not be available when disabled")
		}
	})

	t.Run("unavailable binary", func(t *testing.T) {
		cfg := config.DefaultConfig()
		r1 := newPHPStanRunner("/nonexistent", cfg, logger)
		r2 := newPintRunner("/nonexistent", cfg, logger)
		if r1.available() {
			t.Error("phpstan should not be available with missing binary")
		}
		if r2.available() {
			t.Error("pint should not be available with missing binary")
		}
	})
}

func TestUnknownDiagnosticsSuppressedWhenIndexNotReady(t *testing.T) {
	p := newTestProvider()
	// Do NOT call MarkReady — the index is in warm-up state.

	source := `<?php
class Caller {
    public function run(): void {
        $x = new SomeMysteryClass();
        otherMystery();
    }
}
`
	diags := p.Analyze("file:///test.php", source)

	// Soft-moded: unknown-* findings must be absent during warm-up.
	if found := filterByCode(diags, "unknown-class"); len(found) != 0 {
		t.Fatalf("expected unknown-class to be soft-moded, got %#v", found)
	}
	if found := filterByCode(diags, "unknown-function"); len(found) != 0 {
		t.Fatalf("expected unknown-function to be soft-moded, got %#v", found)
	}

	// Now mark ready and confirm the diagnostics return on a fresh analyze.
	p.index.MarkReady()
	diags2 := p.Analyze("file:///test.php", source)
	if found := filterByCode(diags2, "unknown-class"); len(found) == 0 {
		t.Fatalf("expected unknown-class once Ready, got none")
	}
	if found := filterByCode(diags2, "unknown-function"); len(found) == 0 {
		t.Fatalf("expected unknown-function once Ready, got none")
	}
}

func TestBuiltinUnavailableRuleEmitsEvenWhenIndexNotReady(t *testing.T) {
	// Use a PHP 7.4 profile so str_contains is NOT registered in the index.
	// The BuiltinUnavailableRule fires precisely when a symbol is absent from the
	// index (excluded by profile) but IS a known builtin — this exercises that path.
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})
	logger := log.New(io.Discard, "", 0)
	cfg := config.DefaultConfig()
	f := false
	cfg.PHPStanEnabled = &f
	cfg.PintEnabled = &f
	p := NewProvider(idx, "none", "/tmp", logger, cfg)
	// DO NOT call MarkReady — we're proving the rule is not soft-moded.
	p.BuiltinUnavailableRule = &checks.BuiltinUnavailableRule{
		PHPVersion: "7.4",
		Extensions: nil,
	}

	source := `<?php
$x = str_contains("hay", "needle");
`
	diags := p.Analyze("file:///test.php", source)

	found := filterByCode(diags, "builtin-unavailable")
	if len(found) == 0 {
		t.Fatalf("expected builtin-unavailable finding, got %#v", diags)
	}
	if !strings.Contains(found[0].Message, "PHP >= 8.0") {
		t.Fatalf("expected version message, got %q", found[0].Message)
	}
}

func filterBySource(diags []protocol.Diagnostic, source string) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, d := range diags {
		if d.Source == source {
			out = append(out, d)
		}
	}
	return out
}

func filterByCode(diags []protocol.Diagnostic, code string) []protocol.Diagnostic {
	var out []protocol.Diagnostic
	for _, d := range diags {
		if d.Code == code {
			out = append(out, d)
		}
	}
	return out
}
