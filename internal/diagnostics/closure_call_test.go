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

func analyzeUnknowns(t *testing.T, src string) []string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Cases.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "none", "/tmp", nil, config.DefaultConfig())
	comp := completion.NewProvider(idx, nil, "none")
	p.TypeResolver = func(expr, source string, line int, file *parser.FileNode) string {
		return comp.ResolveDiagnosticType(expr, source, protocol.Position{Line: line}, file)
	}
	var got []string
	for _, d := range p.Analyze("file:///Cases.php", src) {
		if strings.HasPrefix(d.Message, "Unknown") || strings.HasPrefix(d.Message, "Undefined") {
			got = append(got, d.Message)
		}
	}
	return got
}

// What calling a Closure or callable gives is checked, not the Closure: one
// without a signature gives nothing known, so nothing is reported.
func TestCallingAClosureChecksWhatItReturns(t *testing.T) {
	got := analyzeUnknowns(t, `<?php
namespace App;

class Foo { public function fooOnly(): int { return 1; } }

class Cases
{
    public function run(\Closure $make, callable $c): void
    {
        $make('x')->boot();
        $c('x')->boot();
        $f = \Closure::bind(static function ($k) { return new Foo(); }, null, Foo::class);
        $f(1)->fooOnly();
        $h = $make;
        $h()->anything();
    }
}
`)
	if len(got) != 0 {
		t.Errorf("got\n  %s\nwant nothing", strings.Join(got, "\n  "))
	}
}

// The parameters of a method that returns by reference are defined.
func TestReturnByReferenceParametersAreDefined(t *testing.T) {
	got := analyzeUnknowns(t, `<?php
namespace App;

class Cases
{
    private array $data = [];

    public function &__get($name): mixed
    {
        $v = $this->data[$name];
        return $v;
    }
}
`)
	if len(got) != 0 {
		t.Errorf("got\n  %s\nwant nothing", strings.Join(got, "\n  "))
	}
}
