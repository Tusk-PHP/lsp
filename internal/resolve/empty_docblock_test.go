package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestDocTypesReadEmptyDocBlocks(t *testing.T) {
	src := `<?php
class Logger {}
class Person {}
/** @return \Generator<int, Person, Logger, void> */
function people(): \Generator {
    $sent = yield new Person(); // sent
}
/**
 */
function plain() {
    $none = yield 1; // plain
}
class Service {
    public function run(
        /** @var Logger */ $typed,
        /**
         */ $untyped,
        /** The person to greet. */ $who,
    ) {
        echo $typed, $untyped, $who; // params
    }
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src)
	lines := SplitLines(src)
	at := func(marker, v string) textpos.Position {
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+marker) {
				return textpos.Position{Line: i, Character: strings.Index(l, v) + 2}
			}
		}
		t.Fatalf("no line marked %q", marker)
		return textpos.Position{}
	}
	for _, c := range []struct{ v, marker, want string }{
		{"$sent", "sent", "Logger"},
		{"$none", "plain", ""},
		{"$typed", "params", "Logger"},
		{"$untyped", "params", ""},
	} {
		if got := r.ResolveVariableTypeTyped(c.v, lines, at(c.marker, c.v), file).FullString(); got != c.want {
			t.Errorf("%s: type %q, want %q", c.v, got, c.want)
		}
	}
	for v, want := range map[string]string{"$who": "The person to greet.", "$untyped": "", "$typed": ""} {
		if got := ParamDocSummary(file, at("params", v), v); got != want {
			t.Errorf("ParamDocSummary(%s) = %q, want %q", v, got, want)
		}
	}
}
