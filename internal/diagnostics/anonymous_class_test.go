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

// Inside `new class { ... }`, $this, self and static are the anonymous class,
// as in Laravel's AsCollection cast, whose anonymous class reads its promoted
// $arguments property.
func TestThisInsideAnonymousClass(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Base.php", `<?php
namespace App;
abstract class Base { protected function fromBase() {} }
`)
	src := `<?php
namespace App;

class AsCollection
{
    private $own;

    public static function castUsing(array $arguments)
    {
        return new class($arguments) extends Base
        {
            const KIND = 'x';

            public function __construct(protected array $arguments) {}

            public function get()
            {
                $this->fromBase();
                echo self::KIND, static::KIND;
                $this->helper();
                return $this->arguments[0] ?? $this->missing;
            }

            private function helper() {}
        };
    }

    public function after()
    {
        echo $this->own;
        $this->arguments;
    }

    // On the lines that open and close the anonymous class, $this is still
    // the outer class, as in Laravel's MakesHttpRequests.
    public function around()
    {
        $this->register(new class
        {
            public function handle() {}
        }, $this->own);
    }

    private function register($x, $y) {}
}
`
	idx.IndexFile("file:///AsCollection.php", src)
	idx.MarkReady()
	p := NewProvider(idx, "none", "/tmp", nil, config.DefaultConfig())
	comp := completion.NewProvider(idx, nil, "none")
	p.TypeResolver = func(expr, source string, line, col int, file *parser.FileNode) string {
		return comp.ResolveExpressionType(expr, source, protocol.Position{Line: line}, file)
	}
	var got []string
	for _, d := range p.Analyze("file:///AsCollection.php", src) {
		if strings.HasPrefix(d.Message, "Unknown") {
			got = append(got, d.Message)
		}
	}
	sort.Strings(got)
	// A real mistake inside the anonymous class is still reported, and after it
	// ends, $this is the outer class again.
	want := []string{
		"Unknown member 'arguments' on 'AsCollection'",
		"Unknown member 'missing' on 'class@anonymous'",
	}
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n  %s\nwant\n  %s", strings.Join(got, "\n  "), strings.Join(want, "\n  "))
	}
}
