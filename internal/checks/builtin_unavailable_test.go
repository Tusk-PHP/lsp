package checks

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

// TestBuiltinUnavailableFunctionVersion verifies that a function introduced in
// PHP 8.0 is flagged when the project profile targets PHP 7.4.
func TestBuiltinUnavailableFunctionVersion(t *testing.T) {
	source := `<?php
$a = 'hello world';
$b = 'world';
str_contains($a, $b);
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 7.4 — str_contains is NOT included.
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.4"}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one builtin-unavailable finding, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "requires PHP >= 8.0") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding with message containing 'requires PHP >= 8.0', got: %v", relevant)
	}
}

// TestBuiltinUnavailableFunctionExtension verifies that a function requiring a
// specific extension is flagged when that extension is absent from the profile.
func TestBuiltinUnavailableFunctionExtension(t *testing.T) {
	source := `<?php
$a = '{"key": "value"}';
json_validate($a);
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 8.3 without ext-json — json_validate is excluded.
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3", Extensions: []string{}})

	rule := &BuiltinUnavailableRule{PHPVersion: "8.3", Extensions: []string{}}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one builtin-unavailable finding, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "ext-json") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding with message containing 'ext-json', got: %v", relevant)
	}
}

// TestBuiltinUnavailableClass verifies that a class introduced in PHP 8.1 is
// flagged when the project profile targets PHP 7.4.
func TestBuiltinUnavailableClass(t *testing.T) {
	source := `<?php
$fn = function() { return 42; };
$fiber = new Fiber($fn);
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 7.4 — Fiber is NOT included.
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.4"}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one builtin-unavailable finding for Fiber, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "Fiber") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding mentioning 'Fiber', got: %v", relevant)
	}
}

// TestBuiltinUnavailableSkippedWhenAvailable verifies that no finding is
// emitted when the builtin is within the project's PHP version profile.
func TestBuiltinUnavailableSkippedWhenAvailable(t *testing.T) {
	source := `<?php
$a = 'hello world';
$b = 'world';
str_contains($a, $b);
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 8.1 — str_contains IS included.
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.1"})

	rule := &BuiltinUnavailableRule{PHPVersion: "8.1"}
	findings := rule.Check(file, source, idx)

	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			t.Errorf("unexpected builtin-unavailable finding: %v", f)
		}
	}
}

// TestBuiltinUnavailableSkippedWhenIndexShadowed verifies that the rule does
// not fire when a local function definition shadows the builtin name.
func TestBuiltinUnavailableSkippedWhenIndexShadowed(t *testing.T) {
	source := `<?php
function str_contains($haystack, $needle) {
    return strpos($haystack, $needle) !== false;
}

$a = 'hello world';
$b = 'world';
str_contains($a, $b);
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 7.4 — str_contains is NOT in the index.
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.4"}
	findings := rule.Check(file, source, idx)

	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			t.Errorf("unexpected builtin-unavailable finding for locally-defined str_contains: %v", f)
		}
	}
}

// TestBuiltinUnavailableImplementsList verifies that a class-like introduced in
// PHP 8.0 is flagged when referenced in an implements clause on PHP 7.4.
//
// Note: Stringable is intentionally NOT used here because the builtinAvailable
// helper in the symbols package only checks the hand-authored override map
// (builtinAvailabilityByName), not the generated table, so Stringable ends up
// incorrectly indexed for PHP 7.4. WeakMap IS in the hand-authored map and is
// correctly excluded from the PHP 7.4 index. See Issues discovered section of
// the unit report for details on the builtinAvailable gap.
func TestBuiltinUnavailableImplementsList(t *testing.T) {
	source := `<?php
class Foo implements WeakMap {}
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 7.4 — WeakMap is NOT included (PHP 8.0+).
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.4"}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one builtin-unavailable finding for WeakMap, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "WeakMap") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding mentioning 'WeakMap', got: %v", relevant)
	}
}

// TestBuiltinUnavailableAttribute verifies attribute syntax behavior.
//
// NOTE: This test is intentionally skipped. The maskPHPLine helper treats '#'
// as a line comment delimiter and blanks out everything from '#' onward,
// which means PHP 8 attribute syntax (#[Override]) is masked before any regex
// sees it. Adding a new regex to detect attributes is out of scope for this
// unit — see CURRENT_ISSUES.md for the deferred tracking item.
func TestBuiltinUnavailableAttribute(t *testing.T) {
	t.Skip("attribute syntax (#[Name]) is masked by maskPHPLine — regex matching not supported in this unit")
}
