package scope

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestNonASCIIVariableIsOneBinding(t *testing.T) {
	source := "<?php\nfunction run($größe) {\n    $café = $größe;\n    return $café;\n}\n"
	doc := Collect(source)
	binding := doc.BindingAt(protocol.Position{Line: 1, Character: 14})
	if binding == nil || binding.Name != "$größe" || binding.Kind != BindingParameter {
		t.Fatalf("the parameter $größe is one binding, got %+v", binding)
	}
	if got := len(doc.Occurrences(binding)); got != 2 {
		t.Errorf("$größe occurs twice, got %d", got)
	}
	local := doc.BindingAt(protocol.Position{Line: 2, Character: 5})
	if local == nil || local.Name != "$café" || len(doc.Occurrences(local)) != 2 {
		t.Errorf("$café is one binding used twice, got %+v", local)
	}
}
