package analyzer

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestSignatureHelpDocMethodWithCommasInParams(t *testing.T) {
	a := setupSigHelpAnalyzer(t, `<?php
namespace App;

/**
 * @method void configure(array{a: int, b: int} $opts, callable(int, string): bool $cb, array $x = [1, 2], string $s = 'a,b')
 */
class Builder {}
`)
	prefix := "    $b->configure([], fn() => true, "
	src := "<?php\nnamespace App;\n\nfunction run(Builder $b): void {\n" + prefix
	help := a.GetSignatureHelp("file:///call.php", src, protocol.Position{Line: 4, Character: len(prefix)})
	if help == nil || len(help.Signatures) != 1 {
		t.Fatalf("expected one signature, got %+v", help)
	}
	var labels []string
	for _, p := range help.Signatures[0].Parameters {
		labels = append(labels, p.Label)
	}
	want := []string{"array{a: int, b: int} $opts", "callable(int, string): bool $cb", "array $x", "string $s"}
	if !slices.Equal(labels, want) {
		t.Errorf("parameters = %q, want %q", labels, want)
	}
	if help.ActiveParameter != 2 {
		t.Errorf("active parameter = %d, want 2", help.ActiveParameter)
	}
}

func TestSignatureHelpDocMethodWithCallableReturnType(t *testing.T) {
	a := setupSigHelpAnalyzer(t, `<?php
namespace App;

/**
 * @method Closure(int): void make(string $name, int $size)
 */
class Factory {}
`)
	prefix := "    $f->make('a', "
	src := "<?php\nnamespace App;\n\nfunction run(Factory $f): void {\n" + prefix
	help := a.GetSignatureHelp("file:///call.php", src, protocol.Position{Line: 4, Character: len(prefix)})
	if help == nil || len(help.Signatures) != 1 {
		t.Fatalf("expected one signature, got %+v", help)
	}
	var labels []string
	for _, p := range help.Signatures[0].Parameters {
		labels = append(labels, p.Label)
	}
	if want := []string{"string $name", "int $size"}; !slices.Equal(labels, want) {
		t.Errorf("parameters = %q, want %q", labels, want)
	}
}
