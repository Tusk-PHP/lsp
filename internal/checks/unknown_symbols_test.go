package checks

import (
	"strings"
	"testing"

	"github.com/open-southeners/tusk-php/internal/parser"
	"github.com/open-southeners/tusk-php/internal/symbols"
)

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
