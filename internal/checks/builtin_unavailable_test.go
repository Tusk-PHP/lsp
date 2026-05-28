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

// TestBuiltinUnavailableAttribute verifies that a builtin attribute introduced
// in PHP 8.3 is flagged when the project profile targets PHP 8.2.
func TestBuiltinUnavailableAttribute(t *testing.T) {
	source := "<?php\nclass Foo {\n    #[Override]\n    public function run(): void {}\n}\n"
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	// Register builtins for PHP 8.2 — Override is NOT included (PHP 8.3+).
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.2"})

	rule := &BuiltinUnavailableRule{PHPVersion: "8.2"}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one builtin-unavailable finding for Override attribute, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "Override") && strings.Contains(f.Message, "PHP >= 8.3") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding mentioning 'Override' and 'PHP >= 8.3', got: %v", relevant)
	}
}

// TestBuiltinUnavailableConstant verifies that a constant introduced in PHP 7.3
// is flagged when the project profile targets PHP 7.1.
func TestBuiltinUnavailableConstant(t *testing.T) {
	source := `<?php
$flags = JSON_THROW_ON_ERROR;
json_encode($data, $flags);
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.1"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.1"}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one builtin-unavailable finding for JSON_THROW_ON_ERROR, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "JSON_THROW_ON_ERROR") && strings.Contains(f.Message, "PHP >= 7.3") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding mentioning 'JSON_THROW_ON_ERROR' and 'PHP >= 7.3', got: %v", relevant)
	}
}

// TestBuiltinUnavailableConstantBitwiseOr verifies that when two constants are
// combined with '|', only the unavailable one is flagged.
func TestBuiltinUnavailableConstantBitwiseOr(t *testing.T) {
	// JSON_THROW_ON_ERROR requires PHP 7.3; FILTER_VALIDATE_BOOL requires 8.0.
	// On PHP 7.3, only FILTER_VALIDATE_BOOL should be flagged.
	source := `<?php
$opts = JSON_THROW_ON_ERROR | FILTER_VALIDATE_BOOL;
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.3"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.3"}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "builtin-unavailable" {
			relevant = append(relevant, f)
		}
	}

	foundThrowOnError := false
	foundValidateBool := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "JSON_THROW_ON_ERROR") {
			foundThrowOnError = true
		}
		if strings.Contains(f.Message, "FILTER_VALIDATE_BOOL") {
			foundValidateBool = true
		}
	}
	if foundThrowOnError {
		t.Errorf("JSON_THROW_ON_ERROR should NOT be flagged on PHP 7.3 (it requires PHP >= 7.3), findings: %v", relevant)
	}
	if !foundValidateBool {
		t.Errorf("FILTER_VALIDATE_BOOL should be flagged on PHP 7.3 (it requires PHP >= 8.0), findings: %v", relevant)
	}
}

// TestBuiltinUnavailableConstantClassConstantNotConfused verifies that a class
// constant access like Foo::BAR does not emit a builtin-unavailable finding even
// if BAR happens to match an entry in the availability table.
func TestBuiltinUnavailableConstantClassConstantNotConfused(t *testing.T) {
	// JSON_THROW_ON_ERROR is in the availability table. If it appeared after '::',
	// it must NOT be flagged — it is a class constant access, not a global constant.
	source := `<?php
class Foo {
    const JSON_THROW_ON_ERROR = 128;
}
$v = Foo::JSON_THROW_ON_ERROR;
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.1"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.1"}
	findings := rule.Check(file, source, idx)

	for _, f := range findings {
		if f.Code == "builtin-unavailable" && strings.Contains(f.Message, "JSON_THROW_ON_ERROR") {
			t.Errorf("unexpected builtin-unavailable finding for class constant access Foo::JSON_THROW_ON_ERROR: %v", f)
		}
	}
}

// TestBuiltinUnavailableConstantOnSufficientVersion verifies that no finding is
// emitted when the profile's PHP version satisfies the constant's requirement.
func TestBuiltinUnavailableConstantOnSufficientVersion(t *testing.T) {
	source := `<?php
$flags = JSON_THROW_ON_ERROR;
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})

	rule := &BuiltinUnavailableRule{PHPVersion: "7.4"}
	findings := rule.Check(file, source, idx)

	for _, f := range findings {
		if f.Code == "builtin-unavailable" && strings.Contains(f.Message, "JSON_THROW_ON_ERROR") {
			t.Errorf("unexpected builtin-unavailable finding for JSON_THROW_ON_ERROR on PHP 7.4: %v", f)
		}
	}
}
