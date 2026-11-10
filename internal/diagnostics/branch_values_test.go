package diagnostics

import (
	"fmt"
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A variable assigned in several branches may hold any of their values after
// them, so a member any of them has isn't reported there, and one none of
// them has still is.
func TestVariableAssignedInSeveralBranchesHasEveryType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	src := `<?php
namespace App;

class Foo { public function fooOnly(): int { return 1; } }
class Bar { public function barOnly(): int { return 1; } }

class Cases
{
    public function ifElse(Foo|Bar $u)
    {
        if ($u instanceof Bar) {
            $x = new Foo();
        } else {
            $x = new Bar();
        }
        $x->fooOnly();
        $x->barOnly();
        $x->neither();
    }

    public function switched(bool $flag)
    {
        switch ($flag) {
            case true:
                $y = new Foo();
                break;
            default:
                $y = new Bar();
        }
        $y->fooOnly();
        $y->neither();
    }

    public function ifOnly(bool $flag)
    {
        $z = new Foo();
        if ($flag) {
            $z = new Bar();
            $z->barOnly();
            $z->fooOnly();
        }
        $z->fooOnly();
        $z->neither();
    }

    public function looped(bool $flag)
    {
        $w = new Foo();
        while ($flag) {
            $w = new Bar();
        }
        $w->neither();
        return $w->fooOnly();
    }

    public function everyBranch(bool $flag)
    {
        $v = new Bar();
        if ($flag) {
            $v = new Foo();
        } else {
            $v = new Foo();
        }
        $v->barOnly();
    }
}
`
	idx.IndexFile("file:///Cases.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "none", "/tmp", nil, config.DefaultConfig())
	comp := completion.NewProvider(idx, nil, "none")
	p.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		return comp.ResolveDiagnosticType(expr, source, protocol.Position{Line: line, Character: col}, file)
	}
	var got []string
	for _, d := range p.Analyze("file:///Cases.php", src) {
		if strings.HasPrefix(d.Message, "Unknown") {
			got = append(got, fmt.Sprintf("%d %s", d.Range.Start.Line+1, d.Message))
		}
	}
	sort.Strings(got)
	want := []string{
		"18 Unknown member 'neither' on 'Foo|Bar'",
		"31 Unknown member 'neither' on 'Foo|Bar'",
		"40 Unknown member 'fooOnly' on 'Bar'",
		"43 Unknown member 'neither' on 'Foo|Bar'",
		"52 Unknown member 'neither' on 'Foo|Bar'",
		"64 Unknown member 'barOnly' on 'Foo'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
