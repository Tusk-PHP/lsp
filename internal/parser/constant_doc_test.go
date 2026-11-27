package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestConstantsKeepTheirDocComments(t *testing.T) {
	file := ParseFile("<?php\n/** Global. */\nconst G = 1;\nclass Room {\n    /** Both. */\n    const A = 1, B = 2;\n    const BARE = 3;\n}\n", token.Latest)
	if len(file.Constants) != 1 || file.Constants[0].DocComment != "/** Global. */" {
		t.Errorf("the global constant keeps its doc comment: %+v", file.Constants)
	}
	want := map[string]string{"A": "/** Both. */", "B": "/** Both. */", "BARE": ""}
	for _, c := range file.Classes[0].Constants {
		if doc, ok := want[c.Name]; !ok || c.DocComment != doc {
			t.Errorf("%s: doc comment %q, want %q", c.Name, c.DocComment, doc)
		}
		delete(want, c.Name)
	}
	if len(want) != 0 {
		t.Errorf("constants not parsed: %v", want)
	}
}
