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

// DateInterval's public properties are members like any other.
func TestDateIntervalPropertiesAreKnown(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	src := `<?php
namespace App;

class Cases
{
    public function read(\DateInterval $d): void
    {
        echo $d->y, $d->m, $d->d, $d->h, $d->i, $d->s, $d->f, $d->invert, $d->days;
        echo $d->nope;
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
	want := "Unknown member 'nope' on 'DateInterval'"
	if strings.Join(got, "\n") != want {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), want)
	}
}
