package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const attributedVendor = `<?php
namespace Lib;

#[\Deprecated('use fresh() instead')]
function stale(): int { return 1; }

#[\NoDiscard]
function compute(): int { return 1; }

function plain(): int { return 1; }

class Svc {
    #[\Deprecated(message: 'gone soon', since: '2.0')]
    public const OLD = 1;

    #[\Deprecated]
    public static function legacy(): void {}

    #[\NoDiscard('the copy is the result')]
    public function with(): static { return $this; }

    public function self(): static { return $this; }
}
`

// typeOf types the receivers the test source uses, as the server's
// resolver would.
func typeOf(expr, _ string, _, _ int, _ *parser.FileNode) string {
	switch strings.TrimSpace(expr) {
	case "$svc", "$svc->self()", "(new Svc())", "new Svc()":
		return "Lib\\Svc"
	}
	return ""
}

func attributeFindings(t *testing.T, src string) []string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.IndexFile("file:///vendor/lib.php", attributedVendor)
	file := parser.ParseFile(src, token.Latest)
	uses := &MemberUseCache{}
	var got []string
	for _, f := range (&DeprecatedUsageRule{TypeResolver: typeOf, Uses: uses}).Check(file, src, idx) {
		got = append(got, string(f.Code)+": "+f.Message)
	}
	for _, f := range (&UnusedResultRule{TypeResolver: typeOf, Uses: uses}).Check(file, src, idx) {
		got = append(got, string(f.Code)+": "+f.Message)
	}
	return got
}

func TestDeprecatedAndUnusedResultRules(t *testing.T) {
	src := `<?php
namespace App;

use Lib\Svc;
use function Lib\stale;
use function Lib\compute;

function run(Svc $svc): int {
    stale();
    $n = \Lib\stale();
    Svc::legacy();
    echo Svc::OLD;
    compute();
    $kept = compute();
    (void) compute();
    $svc->with();
    $svc?->with();
    $svc->self()->with();
    $copy = $svc->with();
    return compute() + \Lib\plain();
}
`
	want := []string{
		"deprecated: 'stale' is deprecated: use fresh() instead",
		"deprecated: 'stale' is deprecated: use fresh() instead",
		"deprecated: 'legacy' is deprecated",
		"deprecated: 'OLD' is deprecated: gone soon",
		"unused-result: The result of 'compute' should be used",
		"unused-result: The result of 'with' should be used: the copy is the result",
		"unused-result: The result of 'with' should be used: the copy is the result",
		"unused-result: The result of 'with' should be used: the copy is the result",
	}
	if got := attributeFindings(t, src); strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

// Declarations, `new` and code with no attributed symbols are left alone.
func TestAttributeRulesLeaveOtherCodeAlone(t *testing.T) {
	src := `<?php
namespace Lib;
function stale(): int { return 2; }
$x = new Svc();
plain();
`
	// The declaration of stale() here isn't a use, and nothing else is attributed.
	if got := attributeFindings(t, src); len(got) != 0 {
		t.Errorf("got %q", got)
	}
	// An index with no attributed symbols finds nothing without parsing.
	idx := symbols.NewIndex()
	idx.IndexFile("file:///plain.php", "<?php\nfunction plain() {}\n")
	file := parser.ParseFile("<?php\nplain();\n", token.Latest)
	if got := (&DeprecatedUsageRule{}).Check(file, "<?php\nplain();\n", idx); len(got) != 0 {
		t.Errorf("got %v", got)
	}
}

func TestAttributeMessage(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{[]string{`'first'`}, "first"},
		{[]string{`message: "named"`}, "named"},
		{[]string{`since: '2.0'`, `message: 'later'`}, "later"},
		{[]string{`since: '2.0'`}, ""},
		{nil, ""},
	} {
		if got := attributeMessage(&symbols.Attribute{Args: c.args}); got != c.want {
			t.Errorf("%v: got %q, want %q", c.args, got, c.want)
		}
	}
}

func TestDeprecatedFunctionCalledInAnotherCase(t *testing.T) {
	src := `<?php
namespace App;
use function Lib\stale;

STALE();
\LIB\Stale();
`
	want := "deprecated: 'stale' is deprecated: use fresh() instead"
	if got := attributeFindings(t, src); len(got) != 2 || got[0] != want || got[1] != want {
		t.Errorf("PHP calls a function whatever the case it is written in, got %q", got)
	}
}
