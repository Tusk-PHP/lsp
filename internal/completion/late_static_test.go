package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Methods returning static, $this or self give back the class they were
// called on. Diagnostics use the plain-string resolver and hover the typed
// one; both must agree, or fluent and factory chains report false
// unknown-member warnings ("Unknown member 'childOnly' on 'Base'").
func TestLateStaticReturnTypes(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Base.php", `<?php
namespace App;
abstract class Base {
    public static function create(): static { return new static(); }
    public function withName(string $n): static { return $this; }
    /** @return $this */
    public function touch() { return $this; }
    public function copy(): self { return clone $this; }
    public function base(): ?static { return null; }
}
`)
	src := `<?php
namespace App;
final class Child extends Base {
    public function childOnly(): int { return 1; }
    public function run(): void {
        $x = 1;
    }
}
`
	idx.IndexFile("file:///Child.php", src)
	p := NewProvider(idx, nil, "none")
	file := parser.ParseFile(src)
	pos := protocol.Position{Line: 5, Character: 8}
	for expr, want := range map[string]string{
		"static::create()":                  "App\\Child",
		"self::create()":                    "App\\Child",
		"parent::create()":                  "App\\Child",
		"PARENT::create()":                  "App\\Child",
		"Base::create()":                    "App\\Base",
		"Child::create()":                   "App\\Child",
		"$this->withName('a')":              "App\\Child",
		"$this->withName('a')->touch()":     "App\\Child",
		"$this->copy()":                     "App\\Child",
		"$this->base()":                     "App\\Child",
		"static::create()->withName('a')":   "App\\Child",
		"$this->withName('a')->childOnly()": "int",
	} {
		if got := p.ResolveExpressionType(expr, src, pos, file); got != want {
			t.Errorf("ResolveExpressionType(%q) = %q, want %q", expr, got, want)
		}
		if want == "int" {
			continue
		}
		if got := p.ResolveExpressionTypeTyped(expr, src, pos, file).FQN; got != want {
			t.Errorf("ResolveExpressionTypeTyped(%q) = %q, want %q", expr, got, want)
		}
	}
}

func TestForwardsCallingClass(t *testing.T) {
	for before, want := range map[string]bool{
		"parent::": true, "  self :: ": true, "return STATIC::": true,
		"Base::": false, "$this->": false, "$parent::": false, "": false,
	} {
		if got := forwardsCallingClass(before); got != want {
			t.Errorf("forwardsCallingClass(%q) = %v, want %v", before, got, want)
		}
	}
}
