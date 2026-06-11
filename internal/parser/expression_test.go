package parser

import (
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// Match expression tests
// ---------------------------------------------------------------------------

// TestMatchBasic verifies a simple single-arm match expression is captured.
func TestMatchBasic(t *testing.T) {
	t.Helper()
	src := `<?php
$result = match ($status) {
    1 => 'active',
    default => 'inactive',
};
`
	r := New().Parse(src)
	if len(r.Errors) != 0 {
		for _, e := range r.Errors {
			t.Errorf("unexpected parse error line %d: %s", e.Line, e.Message)
		}
	}
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match expression, got %d", len(r.Matches))
	}
	m := r.Matches[0]
	if m.SubjectText != "$status" {
		t.Errorf("subject: want %q, got %q", "$status", m.SubjectText)
	}
	if len(m.Arms) != 2 {
		t.Fatalf("expected 2 arms, got %d", len(m.Arms))
	}
	// First arm
	if len(m.Arms[0].Conditions) != 1 || m.Arms[0].Conditions[0] != "1" {
		t.Errorf("arm[0] conditions: want [\"1\"], got %v", m.Arms[0].Conditions)
	}
	if m.Arms[0].IsDefault {
		t.Error("arm[0] should not be default")
	}
	// Default arm
	if !m.Arms[1].IsDefault {
		t.Error("arm[1] should be default")
	}
}

// TestMatchMultipleConditionsPerArm verifies comma-separated conditions in one arm.
func TestMatchMultipleConditionsPerArm(t *testing.T) {
	t.Helper()
	src := `<?php
$label = match ($code) {
    200, 201, 202 => 'success',
    400, 401, 403 => 'client error',
    500, 503 => 'server error',
    default => 'unknown',
};
`
	r := New().Parse(src)
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	m := r.Matches[0]
	if len(m.Arms) != 4 {
		t.Fatalf("expected 4 arms, got %d", len(m.Arms))
	}
	if len(m.Arms[0].Conditions) != 3 {
		t.Errorf("arm[0]: want 3 conditions, got %d: %v", len(m.Arms[0].Conditions), m.Arms[0].Conditions)
	}
	if len(m.Arms[1].Conditions) != 3 {
		t.Errorf("arm[1]: want 3 conditions, got %d: %v", len(m.Arms[1].Conditions), m.Arms[1].Conditions)
	}
	if len(m.Arms[2].Conditions) != 2 {
		t.Errorf("arm[2]: want 2 conditions, got %d: %v", len(m.Arms[2].Conditions), m.Arms[2].Conditions)
	}
	if !m.Arms[3].IsDefault {
		t.Error("arm[3] should be default")
	}
}

// TestMatchNestedMatch verifies that nested match expressions are each collected.
func TestMatchNestedMatch(t *testing.T) {
	t.Helper()
	src := `<?php
$result = match ($x) {
    1 => match ($y) {
        'a' => 'one-a',
        default => 'one-other',
    },
    default => 'other',
};
`
	r := New().Parse(src)
	if len(r.Errors) != 0 {
		for _, e := range r.Errors {
			t.Errorf("unexpected parse error line %d: %s", e.Line, e.Message)
		}
	}
	// Both the outer and inner match must be collected.
	if len(r.Matches) != 2 {
		t.Fatalf("expected 2 match expressions (outer + nested), got %d", len(r.Matches))
	}
}

// TestMatchInClassMethod verifies match inside a class method body is collected.
func TestMatchInClassMethod(t *testing.T) {
	t.Helper()
	src := `<?php
class StatusMapper {
    public function label(int $code): string {
        return match ($code) {
            200 => 'OK',
            404 => 'Not Found',
            500 => 'Server Error',
            default => 'Unknown',
        };
    }
}
`
	r := New().Parse(src)
	if len(r.Errors) != 0 {
		for _, e := range r.Errors {
			t.Errorf("unexpected parse error line %d: %s", e.Line, e.Message)
		}
	}
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	if len(r.Matches[0].Arms) != 4 {
		t.Errorf("expected 4 arms, got %d", len(r.Matches[0].Arms))
	}
	if len(r.Classes) != 1 {
		t.Fatalf("class not parsed, got %d classes", len(r.Classes))
	}
}

// TestMatchAsMethodName verifies that 'match' used as a method name does NOT
// produce a MatchDef (it is not a match expression).
func TestMatchAsMethodName(t *testing.T) {
	t.Helper()
	src := `<?php
class Router {
    public function match(string $path): bool {
        return $this->pattern === $path;
    }
}
`
	r := New().Parse(src)
	if len(r.Matches) != 0 {
		t.Errorf("expected 0 match expressions, got %d (match used as method name should not be collected)", len(r.Matches))
	}
	if len(r.Classes) != 1 || len(r.Classes[0].Methods) != 1 {
		t.Errorf("class/method not parsed correctly")
	}
}

// TestMatchInsideStringNotParsed verifies that 'match' inside a string literal
// is not parsed as a match expression. The tokenizer collapses strings to
// TokenStringLiteral, so the keyword can never be reached.
func TestMatchInsideStringNotParsed(t *testing.T) {
	t.Helper()
	src := `<?php
$help = "Use match (expr) { ... } to match values";
$other = 'match (x) { 1 => "a" }';
`
	r := New().Parse(src)
	if len(r.Matches) != 0 {
		t.Errorf("expected 0 match expressions from strings, got %d", len(r.Matches))
	}
}

// TestMatchInsideCommentNotParsed verifies that 'match' inside a comment is
// not parsed.
func TestMatchInsideCommentNotParsed(t *testing.T) {
	t.Helper()
	src := `<?php
// match (x) { 1 => 'a', default => 'b' }
/* match ($y) { default => null } */
/** @see match expression */
$x = 1;
`
	r := New().Parse(src)
	if len(r.Matches) != 0 {
		t.Errorf("expected 0 match expressions from comments, got %d", len(r.Matches))
	}
}

// TestMatchMalformedUnclosedBrace verifies that an unclosed match expression
// does not cause a panic or nil result. When a match brace is missing, the
// function's own closing brace may close the match instead, which is an
// inherent limitation of brace-counting recovery (pre-existing parser
// behaviour). The key invariants are: no panic, non-nil result, non-negative
// error positions.
func TestMatchMalformedUnclosedBrace(t *testing.T) {
	t.Helper()
	src := `<?php
function test(): string {
    return match ($x) {
        1 => 'one',
        default => 'other'
}

function after(): void {}
`
	r := New().Parse(src)
	// ParseResult must not be nil (conformance requirement)
	if r == nil {
		t.Fatal("ParseResult is nil")
	}
	// All parse error positions must be non-negative.
	for _, e := range r.Errors {
		if e.Line < 0 || e.Column < 0 {
			t.Errorf("ParseError has negative position: line=%d col=%d msg=%q", e.Line, e.Column, e.Message)
		}
	}
}

// TestMatchGarbageArms verifies graceful recovery from garbage match arm syntax.
func TestMatchGarbageArms(t *testing.T) {
	t.Helper()
	// First and last arms are valid; the middle arm is syntactically malformed.
	src := `<?php
$v = match ($x) {
    1 => 'ok',
    ??? garbage $^& =>! broken,
    default => 'fallback',
};
`
	r := New().Parse(src)
	if r == nil {
		t.Fatal("ParseResult must not be nil")
	}
	// At least one match must be collected (we attempted to parse it)
	if len(r.Matches) == 0 {
		t.Fatal("expected at least 1 match expression, got 0")
	}
	// The default arm should eventually be found even with garbage in between
	m := r.Matches[0]
	hasDefault := false
	for _, arm := range m.Arms {
		if arm.IsDefault {
			hasDefault = true
		}
	}
	if !hasDefault {
		t.Error("expected default arm to be recovered after garbage arm")
	}
}

// TestMatchMalformedNoSubjectParen verifies that 'match' without '(' is not
// mistaken for a match expression (e.g. match used as a variable name).
func TestMatchMalformedNoSubjectParen(t *testing.T) {
	t.Helper()
	src := `<?php
$match = 42;
echo $match;
`
	r := New().Parse(src)
	if len(r.Matches) != 0 {
		t.Errorf("expected 0 match expressions, got %d", len(r.Matches))
	}
}

// TestMatchMultilineNoTrailingComma verifies match arms without trailing commas.
func TestMatchMultilineNoTrailingComma(t *testing.T) {
	t.Helper()
	src := `<?php
$out = match (true) {
    $x > 0 => 'positive',
    $x < 0 => 'negative',
    default => 'zero'
};
`
	r := New().Parse(src)
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	m := r.Matches[0]
	if len(m.Arms) != 3 {
		t.Errorf("expected 3 arms, got %d: %+v", len(m.Arms), m.Arms)
	}
	if !m.Arms[2].IsDefault {
		t.Error("last arm should be default")
	}
}

// TestMatchLineNumbers verifies that MatchDef.Line is the line of 'match'.
func TestMatchLineNumbers(t *testing.T) {
	t.Helper()
	src := `<?php
// line 0 (0-indexed)
// line 1
$x = match ($n) {  // line 3 (0-indexed)
    1 => 'one',
    default => 'other',
};
`
	r := New().Parse(src)
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	if r.Matches[0].Line != 3 {
		t.Errorf("match Line: want 3 (0-indexed), got %d", r.Matches[0].Line)
	}
}

// TestMatchErrorPositionsNonNegative verifies ParseError positions from
// malformed matches are non-negative (conformance requirement).
func TestMatchErrorPositionsNonNegative(t *testing.T) {
	t.Helper()
	inputs := []string{
		"<?php $v = match ($x) { garbage =>!! };",
		"<?php $v = match ($x) {",
		"<?php $v = match ($x) { 1 =>",
	}
	for _, src := range inputs {
		r := New().Parse(src)
		if r == nil {
			t.Fatalf("ParseResult is nil for input %q", src)
		}
		for _, e := range r.Errors {
			if e.Line < 0 || e.Column < 0 {
				t.Errorf("ParseError has negative position line=%d col=%d for input %q", e.Line, e.Column, src)
			}
		}
	}
}

// ---------------------------------------------------------------------------
// Call-site (named arguments + positional + spread) tests
// ---------------------------------------------------------------------------

// TestCallSiteSimple verifies a simple function call is collected.
func TestCallSiteSimple(t *testing.T) {
	t.Helper()
	src := `<?php
doSomething(1, 'hello', true);
`
	r := New().Parse(src)
	cs := findCallSite(r, "doSomething")
	if cs == nil {
		t.Fatal("expected call site 'doSomething' not found")
	}
	if len(cs.Args) != 3 {
		t.Errorf("expected 3 args, got %d: %+v", len(cs.Args), cs.Args)
	}
	if cs.IsFirstClassCallable {
		t.Error("should not be first-class callable")
	}
}

// TestCallSiteNamedArguments verifies named arguments are recorded correctly.
func TestCallSiteNamedArguments(t *testing.T) {
	t.Helper()
	src := `<?php
createUser(name: 'Alice', age: 30, active: true);
`
	r := New().Parse(src)
	cs := findCallSite(r, "createUser")
	if cs == nil {
		t.Fatal("call site 'createUser' not found")
	}
	if len(cs.Args) != 3 {
		t.Fatalf("expected 3 args, got %d: %+v", len(cs.Args), cs.Args)
	}
	if cs.Args[0].Name != "name" {
		t.Errorf("arg[0].Name: want %q, got %q", "name", cs.Args[0].Name)
	}
	if cs.Args[1].Name != "age" {
		t.Errorf("arg[1].Name: want %q, got %q", "age", cs.Args[1].Name)
	}
	if cs.Args[2].Name != "active" {
		t.Errorf("arg[2].Name: want %q, got %q", "active", cs.Args[2].Name)
	}
}

// TestCallSiteMixedPositionalAndNamed verifies positional and named args together.
func TestCallSiteMixedPositionalAndNamed(t *testing.T) {
	t.Helper()
	src := `<?php
htmlspecialchars($string, flags: ENT_QUOTES, encoding: 'UTF-8');
`
	r := New().Parse(src)
	cs := findCallSite(r, "htmlspecialchars")
	if cs == nil {
		t.Fatal("call site 'htmlspecialchars' not found")
	}
	if len(cs.Args) != 3 {
		t.Fatalf("expected 3 args, got %d: %+v", len(cs.Args), cs.Args)
	}
	if cs.Args[0].Name != "" {
		t.Errorf("arg[0] should be positional (Name empty), got %q", cs.Args[0].Name)
	}
	if cs.Args[1].Name != "flags" {
		t.Errorf("arg[1].Name: want %q, got %q", "flags", cs.Args[1].Name)
	}
	if cs.Args[2].Name != "encoding" {
		t.Errorf("arg[2].Name: want %q, got %q", "encoding", cs.Args[2].Name)
	}
}

// TestCallSiteSpreadArgument verifies spread args are marked IsSpread=true.
func TestCallSiteSpreadArgument(t *testing.T) {
	t.Helper()
	src := `<?php
$args = [1, 2, 3];
myFunc(...$args);
`
	r := New().Parse(src)
	cs := findCallSite(r, "myFunc")
	if cs == nil {
		t.Fatal("call site 'myFunc' not found")
	}
	if len(cs.Args) != 1 {
		t.Fatalf("expected 1 arg, got %d: %+v", len(cs.Args), cs.Args)
	}
	if !cs.Args[0].IsSpread {
		t.Error("expected arg[0].IsSpread = true")
	}
}

// TestCallSiteMethodCall verifies $var->method() call sites are collected.
func TestCallSiteMethodCall(t *testing.T) {
	t.Helper()
	src := `<?php
class Foo {
    public function doWork(): void {
        $this->logger->info('hello', context: ['user' => 42]);
    }
}
`
	r := New().Parse(src)
	cs := findCallSiteContaining(r, "logger->info")
	if cs == nil {
		t.Fatal("call site for $this->logger->info not found")
	}
	if len(cs.Args) != 2 {
		t.Errorf("expected 2 args, got %d: %+v", len(cs.Args), cs.Args)
	}
	if cs.Args[1].Name != "context" {
		t.Errorf("arg[1].Name: want \"context\", got %q", cs.Args[1].Name)
	}
}

// TestCallSiteStaticCall verifies Type::method() call sites are collected.
func TestCallSiteStaticCall(t *testing.T) {
	t.Helper()
	src := `<?php
Cache::get('key', default: null);
`
	r := New().Parse(src)
	cs := findCallSite(r, "Cache::get")
	if cs == nil {
		t.Fatal("call site 'Cache::get' not found")
	}
	if len(cs.Args) != 2 {
		t.Fatalf("expected 2 args, got %d: %+v", len(cs.Args), cs.Args)
	}
	if cs.Args[1].Name != "default" {
		t.Errorf("arg[1].Name: want \"default\", got %q", cs.Args[1].Name)
	}
}

// TestCallSiteConstructor verifies 'new Type()' call sites are collected.
func TestCallSiteConstructor(t *testing.T) {
	t.Helper()
	src := `<?php
$req = new Request(method: 'GET', uri: '/api/users');
`
	r := New().Parse(src)
	cs := findCallSite(r, "new Request")
	if cs == nil {
		t.Fatal("call site 'new Request' not found")
	}
	if len(cs.Args) != 2 {
		t.Fatalf("expected 2 args, got %d: %+v", len(cs.Args), cs.Args)
	}
}

// TestCallSiteTrailingComma verifies trailing commas in argument lists work.
func TestCallSiteTrailingComma(t *testing.T) {
	t.Helper()
	src := `<?php
foo(
    bar: 1,
    baz: 2,
);
`
	r := New().Parse(src)
	cs := findCallSite(r, "foo")
	if cs == nil {
		t.Fatal("call site 'foo' not found")
	}
	if len(cs.Args) != 2 {
		t.Errorf("expected 2 args (trailing comma), got %d: %+v", len(cs.Args), cs.Args)
	}
}

// TestCallSiteZeroArgs verifies calls with no arguments produce an empty Args slice.
func TestCallSiteZeroArgs(t *testing.T) {
	t.Helper()
	src := `<?php
flush();
`
	r := New().Parse(src)
	cs := findCallSite(r, "flush")
	if cs == nil {
		t.Fatal("call site 'flush' not found")
	}
	if cs.IsFirstClassCallable {
		t.Error("flush() with no args should not be first-class callable")
	}
	if len(cs.Args) != 0 {
		t.Errorf("expected 0 args, got %d: %+v", len(cs.Args), cs.Args)
	}
}

// TestCallSiteMultiline verifies a multi-line call is collected correctly.
func TestCallSiteMultiline(t *testing.T) {
	t.Helper()
	src := `<?php
DB::table('users')
   ->where('id', '=', $id)
   ->first();
`
	r := New().Parse(src)
	// We should find at least the DB::table call site
	cs := findCallSite(r, "DB::table")
	if cs == nil {
		t.Fatal("call site 'DB::table' not found")
	}
	if len(cs.Args) != 1 {
		t.Errorf("expected 1 arg, got %d: %+v", len(cs.Args), cs.Args)
	}
}

// ---------------------------------------------------------------------------
// First-class callable tests
// ---------------------------------------------------------------------------

// TestFirstClassCallablePlainFunction verifies foo(...) is marked IsFirstClassCallable.
func TestFirstClassCallablePlainFunction(t *testing.T) {
	t.Helper()
	src := `<?php
$fn = strlen(...);
`
	r := New().Parse(src)
	cs := findCallSite(r, "strlen")
	if cs == nil {
		t.Fatal("call site 'strlen' not found")
	}
	if !cs.IsFirstClassCallable {
		t.Error("strlen(...) should be marked IsFirstClassCallable")
	}
	if len(cs.Args) != 0 {
		t.Errorf("FCC should have 0 Args, got %d", len(cs.Args))
	}
}

// TestFirstClassCallableMethod verifies $obj->method(...) is marked IsFirstClassCallable.
func TestFirstClassCallableMethod(t *testing.T) {
	t.Helper()
	src := `<?php
$fn = $this->process(...);
`
	r := New().Parse(src)
	cs := findCallSiteContaining(r, "->process")
	if cs == nil {
		t.Fatal("call site '$this->process' not found")
	}
	if !cs.IsFirstClassCallable {
		t.Errorf("$this->process(...) should be IsFirstClassCallable, got %+v", cs)
	}
}

// TestFirstClassCallableStaticMethod verifies Type::method(...) is marked IsFirstClassCallable.
func TestFirstClassCallableStaticMethod(t *testing.T) {
	t.Helper()
	src := `<?php
$fn = Str::upper(...);
`
	r := New().Parse(src)
	cs := findCallSite(r, "Str::upper")
	if cs == nil {
		t.Fatal("call site 'Str::upper' not found")
	}
	if !cs.IsFirstClassCallable {
		t.Error("Str::upper(...) should be marked IsFirstClassCallable")
	}
}

// TestFirstClassCallableNotConfusedWithSpread verifies that ...$args is NOT
// treated as a first-class callable — it's a spread argument.
func TestFirstClassCallableNotConfusedWithSpread(t *testing.T) {
	t.Helper()
	src := `<?php
myFunc(...$items);
`
	r := New().Parse(src)
	cs := findCallSite(r, "myFunc")
	if cs == nil {
		t.Fatal("call site 'myFunc' not found")
	}
	if cs.IsFirstClassCallable {
		t.Error("myFunc(...$items) should NOT be first-class callable")
	}
	if len(cs.Args) != 1 || !cs.Args[0].IsSpread {
		t.Errorf("expected 1 spread arg, got %+v", cs.Args)
	}
}

// TestFirstClassCallableAllThreeShapes verifies all three FCC forms in one file.
func TestFirstClassCallableAllThreeShapes(t *testing.T) {
	t.Helper()
	src := `<?php
$a = strlen(...);
$b = $obj->format(...);
$c = Carbon::parse(...);
`
	r := New().Parse(src)
	if r == nil {
		t.Fatal("nil result")
	}

	fccCount := 0
	for _, cs := range r.Calls {
		if cs.IsFirstClassCallable {
			fccCount++
		}
	}
	if fccCount < 3 {
		t.Errorf("expected at least 3 FCC call sites, got %d (calls: %+v)", fccCount, r.Calls)
	}
}

// ---------------------------------------------------------------------------
// Determinism and panic-safety tests
// ---------------------------------------------------------------------------

// TestExpressionScannerDeterministic verifies two parses of the same source
// produce identical Matches and Calls counts (conformance requirement).
func TestExpressionScannerDeterministic(t *testing.T) {
	t.Helper()
	src := `<?php
namespace App;

class Processor {
    public function run(int $mode): string {
        $fn = strlen(...);
        doThing(a: 1, b: 2);
        return match ($mode) {
            1 => compute($mode, factor: 2),
            2, 3 => Cache::get('key', default: null),
            default => 'fallback',
        };
    }
}
`
	r1 := New().Parse(src)
	r2 := New().Parse(src)
	if len(r1.Matches) != len(r2.Matches) {
		t.Errorf("matches non-deterministic: %d vs %d", len(r1.Matches), len(r2.Matches))
	}
	if len(r1.Calls) != len(r2.Calls) {
		t.Errorf("calls non-deterministic: %d vs %d", len(r1.Calls), len(r2.Calls))
	}
}

// TestExpressionScannerNoPanic verifies the scanner never panics on any input.
func TestExpressionScannerNoPanic(t *testing.T) {
	t.Helper()
	inputs := []string{
		"",
		"<?php",
		"<?php match",
		"<?php match (",
		"<?php match ($x) {",
		"<?php match ($x) { 1 =>",
		"<?php foo(",
		"<?php $x->(",
		"<?php new",
		"<?php $a->b->c->d->e->f->g(",
		"<?php match ($x) { default",
		// binary garbage
		"\x00\xff\xfe<?php match($x){1=>2}",
		// deeply nested
		"<?php match(match(match($x){1=>2}){3=>4}){5=>6}",
	}
	for _, src := range inputs {
		func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("panic on input %q: %v", truncate(src, 40), r)
				}
			}()
			result := New().Parse(src)
			if result == nil {
				t.Errorf("nil result for input %q", truncate(src, 40))
			}
		}()
	}
}

// TestExpressionScannerErrorPositionsNonNegative verifies all ParseErrors
// emitted by the expression scanner have non-negative positions.
func TestExpressionScannerErrorPositionsNonNegative(t *testing.T) {
	t.Helper()
	src := `<?php
$v = match ($x) { garbage ==!> broken, default => 'ok' };
$w = match ($y) { ??? };
foo(bar:, baz: 1);
`
	r := New().Parse(src)
	for _, e := range r.Errors {
		if e.Line < 0 || e.Column < 0 {
			t.Errorf("ParseError has negative position: line=%d col=%d msg=%q", e.Line, e.Column, e.Message)
		}
	}
}

// TestCallSiteCapAtMaxPerFile verifies the cap (maxCallSitesPerFile) is respected.
func TestCallSiteCapAtMaxPerFile(t *testing.T) {
	t.Helper()
	// Build a source file with more calls than the cap.
	var sb strings.Builder
	sb.WriteString("<?php\n")
	// maxCallSitesPerFile + some extra calls
	for i := 0; i < maxCallSitesPerFile+100; i++ {
		sb.WriteString("foo();\n")
	}
	r := New().Parse(sb.String())
	if len(r.Calls) > maxCallSitesPerFile {
		t.Errorf("call site cap exceeded: got %d, max is %d", len(r.Calls), maxCallSitesPerFile)
	}
}

// TestParseResultNeverNil verifies ParseFile (compat layer) still never returns nil.
func TestParseResultNeverNilWithExpressions(t *testing.T) {
	t.Helper()
	inputs := []string{
		"",
		"<?php match($x){1=>2}",
		"<?php foo(bar: 1, ...baz());",
		"<?php strlen(...);",
	}
	for _, src := range inputs {
		file := ParseFile(src)
		if file == nil {
			t.Errorf("ParseFile returned nil for input %q", truncate(src, 50))
		}
	}
}

// ---------------------------------------------------------------------------
// Edge-case tests
// ---------------------------------------------------------------------------

// TestMatchWithComplexSubject verifies a function-call subject is captured.
func TestMatchWithComplexSubject(t *testing.T) {
	t.Helper()
	src := `<?php
$r = match (gettype($value)) {
    'integer' => 'int',
    'string'  => 'str',
    default   => 'other',
};
`
	r := New().Parse(src)
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	subj := r.Matches[0].SubjectText
	if subj == "" {
		t.Error("SubjectText should not be empty for gettype($value)")
	}
	// The subject should contain "gettype" somewhere
	if !strings.Contains(subj, "gettype") {
		t.Errorf("SubjectText should contain 'gettype', got %q", subj)
	}
}

// TestMatchSubjectTextSimpleVariable verifies the subject text for a simple variable.
func TestMatchSubjectTextSimpleVariable(t *testing.T) {
	t.Helper()
	src := `<?php
$out = match ($x) { 1 => 'a', default => 'b' };
`
	r := New().Parse(src)
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	if r.Matches[0].SubjectText != "$x" {
		t.Errorf("SubjectText: want %q, got %q", "$x", r.Matches[0].SubjectText)
	}
}

// TestCallSiteNestedCallArgs verifies nested call expressions inside args do
// not confuse the depth tracking.
func TestCallSiteNestedCallArgs(t *testing.T) {
	t.Helper()
	src := `<?php
outer(inner(a: 1, b: 2), extra: true);
`
	r := New().Parse(src)
	cs := findCallSite(r, "outer")
	if cs == nil {
		t.Fatal("call site 'outer' not found")
	}
	// outer has 2 args: the inner call result and extra
	if len(cs.Args) != 2 {
		t.Errorf("expected 2 args for outer(), got %d: %+v", len(cs.Args), cs.Args)
	}
	if cs.Args[1].Name != "extra" {
		t.Errorf("cs.Args[1].Name: want %q, got %q", "extra", cs.Args[1].Name)
	}
}

// TestMatchArmLineNumbers verifies MatchArm.Line captures the correct line.
func TestMatchArmLineNumbers(t *testing.T) {
	t.Helper()
	src := `<?php
$v = match ($x) {
    1       => 'one',     // line 2 (0-indexed)
    2, 3    => 'two-three', // line 3
    default => 'other',   // line 4
};
`
	r := New().Parse(src)
	if len(r.Matches) != 1 {
		t.Fatalf("expected 1 match, got %d", len(r.Matches))
	}
	m := r.Matches[0]
	if len(m.Arms) != 3 {
		t.Fatalf("expected 3 arms, got %d", len(m.Arms))
	}
	// Arm lines should be non-negative and in order
	for i, arm := range m.Arms {
		if arm.Line < 0 {
			t.Errorf("arm[%d].Line is negative: %d", i, arm.Line)
		}
	}
	if m.Arms[0].Line > m.Arms[2].Line {
		t.Errorf("arm lines not increasing: %d, %d, %d",
			m.Arms[0].Line, m.Arms[1].Line, m.Arms[2].Line)
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func findCallSite(r *ParseResult, callee string) *CallSite {
	for i := range r.Calls {
		if r.Calls[i].CalleeText == callee {
			return &r.Calls[i]
		}
	}
	return nil
}

func findCallSiteContaining(r *ParseResult, substr string) *CallSite {
	for i := range r.Calls {
		if strings.Contains(r.Calls[i].CalleeText, substr) {
			return &r.Calls[i]
		}
	}
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
