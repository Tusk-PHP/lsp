package diagnostics

import (
	"sort"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// A parameter given a new value has the new value's type from then on, as in
// Laravel's Sleep::duration(), which replaces a DateInterval with a
// CarbonInterval and reads totalMicroseconds from it.
func TestReassignedParameterTakesItsNewType(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Interval.php", `<?php
namespace App;
class Interval extends \DateInterval {
    public $totalMicroseconds = 0;
    public static function instance(\DateInterval $i): static { return new static('P1D'); }
}
`)
	src := `<?php
namespace App;

class Cases
{
    /** @param \DateInterval|int $duration */
    public function docOnly($duration)
    {
        if (! $duration instanceof \DateInterval) {
            return;
        } else {
            $duration = Interval::instance($duration);
            if ($duration->totalMicroseconds < 0) {}
        }
    }

    public function native(\DateInterval $d)
    {
        $d = Interval::instance($d);
        echo $d->totalMicroseconds;
    }

    public function beforeReassignment(\DateInterval $d)
    {
        echo $d->totalMicroseconds;
        $d = Interval::instance($d);
    }

    // After the if, $d may be either: neither is reported.
    public function inABranch(\DateInterval $d, bool $c)
    {
        if ($c) {
            $d = Interval::instance($d);
            echo $d->totalMicroseconds;
        }
        echo $d->totalMicroseconds;
        echo $d->days;
    }

    public function braceless(\DateInterval $d, bool $c)
    {
        if ($c) $d = Interval::instance($d);
        echo $d->totalMicroseconds;
    }

    // Symfony's Preloader reuses a parameter as the foreach variable.
    /** @param Interval[] $list */
    public function asForeachVariable(\DateInterval $d, array $list)
    {
        foreach ($list as $d) {
            echo $d->totalMicroseconds;
        }
    }

    // ??= may keep the value it has, so the declared type still applies.
    public function coalesce(?\DateInterval $d)
    {
        $d ??= Interval::instance(new \DateInterval('P1D'));
        echo $d->missing;
    }
}
`
	idx.IndexFile("file:///Cases.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "none", "/tmp", nil, config.DefaultConfig())
	comp := completion.NewProvider(idx, nil, "none")
	p.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		return comp.ResolveExpressionType(expr, source, protocol.Position{Line: line}, file)
	}
	var got []string
	for _, d := range p.Analyze("file:///Cases.php", src) {
		if strings.HasPrefix(d.Message, "Unknown") {
			got = append(got, d.Message)
		}
	}
	sort.Strings(got)
	// Before the new value, the declared type still applies; after ??=, it
	// is one of the types.
	want := []string{
		"Unknown member 'missing' on 'DateInterval|Interval'",
		"Unknown member 'totalMicroseconds' on 'DateInterval'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
