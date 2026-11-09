package completion

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// `$this->` inside an anonymous class completes its own members and its
// parent's, not those of the class around it. The anonymous class itself is
// never offered as a class name.
func TestCompleteThisInsideAnonymousClass(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///Base.php", `<?php
namespace App;
abstract class Base { public function fromBase() {} }
`)
	src := `<?php
namespace App;

class Outer
{
    public function outerOnly() {}

    public function make()
    {
        return new class extends Base
        {
            public function own() {}

            public function run()
            {
                $this->
            }
        };
    }
}
`
	idx.IndexFile("file:///Outer.php", src)
	p := NewProvider(idx, nil, "")
	labels := map[string]bool{}
	for _, item := range p.GetCompletions("file:///Outer.php", src, protocol.Position{Line: 15, Character: 23}) {
		labels[item.Label] = true
	}
	for _, want := range []string{"own", "fromBase"} {
		if !labels[want] {
			t.Errorf("missing %q in %v", want, labels)
		}
	}
	if labels["outerOnly"] {
		t.Errorf("offered the outer class's outerOnly")
	}

	for _, item := range p.GetCompletions("file:///Outer.php", strings.Replace(src, "$this->", "clas", 1), protocol.Position{Line: 15, Character: 20}) {
		if strings.Contains(item.Label, "anonymous") {
			t.Errorf("offered %q", item.Label)
		}
	}
}
