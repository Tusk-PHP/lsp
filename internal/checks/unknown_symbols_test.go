package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestAttributeWithArgsNoFalsePositives reproduces the user-reported bug where
// attribute classes used with constructor arguments were flagged as unknown
// functions, and named arguments inside them (e.g. `nullable: true`) were
// flagged as unknown attributes.
func TestAttributeWithArgsNoFalsePositives(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3"})
	idx.IndexFile("file:///attrs.php", `<?php
namespace Toramanlis\ImplicitMigrations\Attributes;
#[\Attribute]
class Char {}
#[\Attribute]
class Unique {}
`)

	source := `<?php
namespace App\Models;

use Toramanlis\ImplicitMigrations\Attributes\Char;
use Toramanlis\ImplicitMigrations\Attributes\Unique;

#[Unique(["ext_id", "email"])]
#[Char("picture", nullable: true)]
#[Char("name", nullable: false)]
#[Char("ext_id", nullable: true)]
class User
{
}
`
	file := parser.ParseFile(source)

	fnFindings := (&UnknownFunctionRule{}).Check(file, source, idx)
	for _, f := range fnFindings {
		t.Errorf("unexpected unknown-function finding: %q at line %d col %d", f.Message, f.StartLine, f.StartCol)
	}

	clsFindings := (&UnknownClassRule{}).Check(file, source, idx)
	for _, f := range clsFindings {
		t.Errorf("unexpected unknown-class/attribute finding: %q at line %d col %d", f.Message, f.StartLine, f.StartCol)
	}
}

// TestUnknownClassUnknownAttribute verifies that an attribute with an unknown
// class-like name is flagged by UnknownClassRule.
func TestUnknownClassUnknownAttribute(t *testing.T) {
	source := `<?php
class Foo {
    #[NeverHeardOfThisAttribute]
    public function bar(): void {}
}
`
	file := parser.ParseFile(source)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.3"})

	rule := &UnknownClassRule{}
	findings := rule.Check(file, source, idx)

	var relevant []Finding
	for _, f := range findings {
		if f.Code == "unknown-class" {
			relevant = append(relevant, f)
		}
	}
	if len(relevant) == 0 {
		t.Fatal("expected at least one unknown-class finding for NeverHeardOfThisAttribute, got none")
	}
	found := false
	for _, f := range relevant {
		if strings.Contains(f.Message, "NeverHeardOfThisAttribute") && strings.Contains(strings.ToLower(f.Message), "attribute") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected a finding mentioning 'NeverHeardOfThisAttribute' and 'attribute', got: %v", relevant)
	}
}
