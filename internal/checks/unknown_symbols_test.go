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

func TestUnknownFunctionRuleSkipsKeywords(t *testing.T) {
	source := `<?php
class B {
    public function a($p, $x)
    {
        $f = function ($t) use ($x) { return $t; };
        $g = fn ($v) => $v;
        $h = (1 and (2)) or (3) xor (4);
        list($c) = $p;
        switch ($c) { case (1): break; }
        $y = $c instanceof ($p);
        $p($c);
        static($c);
        yield ($c);
        declare(strict_types=1);
        throw ($c);
        return (1);
    }
    public function b()
    {
        return function ($joinTable) {
            return $joinTable;
        };
    }
}
`
	if findings := (&UnknownFunctionRule{}).Check(parser.ParseFile(source), source, symbols.NewIndex()); len(findings) != 0 {
		t.Errorf("unexpected findings: %+v", findings)
	}

	source = "<?php\nmissing_function(1);\n"
	findings := (&UnknownFunctionRule{}).Check(parser.ParseFile(source), source, symbols.NewIndex())
	if len(findings) != 1 {
		t.Errorf("want missing_function reported, got %+v", findings)
	}
}
