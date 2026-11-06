package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// WordPress targets PHP 7.4 and adds PHP 8 attributes for when it runs on 8.x.
const forwardAttributesSource = `<?php
#[AllowDynamicProperties]
class WP_Thing {
    #[ReturnTypeWillChange]
    public function offsetGet( $offset ) {}
    public function login( #[\SensitiveParameter] $password ) {}
    #[Override]
    public function run() {}
    #[NotAnAttributeAnywhere]
    public function other() {}
}
`

func checkAttributes(t *testing.T, version string) (unknown, unavailable []Finding) {
	t.Helper()
	file := parser.ParseFile(forwardAttributesSource)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: version})
	unknown = (&UnknownClassRule{}).Check(file, forwardAttributesSource, idx)
	unavailable = (&BuiltinUnavailableRule{PHPVersion: version}).Check(file, forwardAttributesSource, idx)
	return unknown, unavailable
}

func TestForwardCompatibleAttributesOnPHP7(t *testing.T) {
	unknown, unavailable := checkAttributes(t, "7.4")
	if len(unknown) != 1 || !strings.Contains(unknown[0].Message, "NotAnAttributeAnywhere") {
		t.Errorf("only the attribute PHP doesn't define should be unknown, got %+v", unknown)
	}
	if len(unavailable) != 0 {
		t.Errorf("PHP 7 reads attributes as comments; got %+v", unavailable)
	}
}

func TestForwardCompatibleAttributesOnPHP8(t *testing.T) {
	unknown, unavailable := checkAttributes(t, "8.0")
	if len(unknown) != 1 || !strings.Contains(unknown[0].Message, "NotAnAttributeAnywhere") {
		t.Errorf("only the attribute PHP doesn't define should be unknown, got %+v", unknown)
	}
	// On 8.0, AllowDynamicProperties, ReturnTypeWillChange (8.1),
	// SensitiveParameter (8.2) and Override (8.3) do nothing yet: hints only.
	if len(unavailable) != 4 {
		t.Fatalf("expected 4 hints, got %+v", unavailable)
	}
	for _, f := range unavailable {
		if f.Severity != SeverityHint || !strings.Contains(f.Message, "has no effect before PHP >=") {
			t.Errorf("expected a hint that the attribute does nothing yet, got %+v", f)
		}
	}
}

func TestNamespacedAttributeWithoutImportIsStillUnknown(t *testing.T) {
	// Inside a namespace, #[AllowDynamicProperties] without a leading \ or a
	// `use` means App\AllowDynamicProperties, which PHP does not treat as its own.
	src := "<?php\nnamespace App;\n#[AllowDynamicProperties]\nclass A {}\n#[\\AllowDynamicProperties]\nclass B {}\n"
	got := (&UnknownClassRule{}).Check(parser.ParseFile(src), src, symbols.NewIndex())
	if len(got) != 1 || got[0].StartLine != 2 {
		t.Errorf("expected only the unqualified attribute in the namespace to be unknown, got %+v", got)
	}
}
