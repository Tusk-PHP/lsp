package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestCompletionAfterParentSelfStaticIncludesInstanceMethods(t *testing.T) {
	src := `<?php
namespace App;
abstract class Base {
    public const BASE = 1;
    public function __construct(string $n) {}
    protected function helper(): int { return 1; }
    private function secret(): void {}
    public static function create(): static { return new static('x'); }
}
final class Child extends Base {
    public function childOnly(): int { return 2; }
    public function __construct() {
        parent::
        self::
    }
    public static function make(): void {
        parent::
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Child.php", src)
	p := NewProvider(idx, nil, "none")
	labels := func(marker string, nth int) map[string]bool {
		t.Helper()
		seen := 0
		for i, line := range strings.Split(src, "\n") {
			if col := strings.Index(line, marker); col >= 0 {
				if seen++; seen < nth {
					continue
				}
				got := map[string]bool{}
				for _, item := range p.GetCompletions("file:///Child.php", src, protocol.Position{Line: i, Character: col + len(marker)}) {
					got[item.Label] = true
				}
				return got
			}
		}
		t.Fatalf("marker %q #%d not found", marker, nth)
		return nil
	}
	inInstance := labels("parent::", 1)
	for _, want := range []string{"__construct", "helper", "create", "BASE"} {
		if !inInstance[want] {
			t.Errorf("parent:: in an instance method should offer %s; got %v", want, inInstance)
		}
	}
	if inInstance["secret"] {
		t.Errorf("parent:: must not offer the parent's private method")
	}
	if self := labels("self::", 1); !self["childOnly"] || !self["helper"] {
		t.Errorf("self:: in an instance method should offer childOnly and helper; got %v", self)
	}
	inStatic := labels("parent::", 2)
	if inStatic["helper"] || inStatic["__construct"] {
		t.Errorf("parent:: in a static method should offer only static members; got %v", inStatic)
	}
	if !inStatic["create"] || !inStatic["BASE"] {
		t.Errorf("parent:: in a static method should still offer create and BASE; got %v", inStatic)
	}
}
