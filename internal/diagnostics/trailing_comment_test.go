package diagnostics

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A comment after an assignment isn't part of its value: `$n = $y->fooOnly();
// // see $y` is an int, not the Foo the `$y` in the comment would give.
func TestCommentAfterAnAssignmentIsNotItsValue(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	src := `<?php
namespace App;

class Foo { public function fooOnly(): int { return 1; } }
class Bar { public function barOnly(): int { return 1; } }

class Cases
{
    public function run(): void
    {
        $y = new Foo();
        $n = $y->fooOnly(); // see $y
        $n->anything();
        $k = new Bar(); # was $y
        $k->fooOnly();
        $y = $y->fooOnly(); /* uses $y */
        $y->barOnly();
    }
}
`
	idx.IndexFile("file:///Cases.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "none", "/tmp", nil, config.DefaultConfig())
	comp := completion.NewProvider(idx, nil, "none")
	p.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return comp.ResolveDiagnosticType(expr, source, protocol.Position{Line: line}, file)
	}
	var got []string
	for _, d := range p.Analyze("file:///Cases.php", src) {
		if strings.HasPrefix(d.Message, "Unknown") {
			got = append(got, d.Message)
		}
	}
	want := "Unknown member 'fooOnly' on 'Bar'"
	if strings.Join(got, "\n") != want {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), want)
	}
}
