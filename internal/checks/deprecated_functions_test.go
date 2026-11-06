package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

func TestDeprecatedFunctionRuleIgnoresMethodsAndNonCalls(t *testing.T) {
	src := `<?php
namespace App;
use Illuminate\Support\Collection;
class Seeder {
    public function each(callable $fn) {}
    public function run(): void {
        Collection::make([
            ['email' => 'a@b.c'],
        ])->each(function (array $data) {});
        $items->each(fn ($x) => $x);
        $items?->each(fn ($x) => $x);
        $items
            ->each(fn ($x) => $x);
        $items->
            each(fn ($x) => $x);
        Arr::each([]);
        static::each(fn () => null);
        foreach($items as $x) {}
        Foo\each($items);
        $s = 'each($x)'; // each($y)
        /* create_function('$a', '') */
    }
}
`
	got := (&DeprecatedFunctionRule{}).Check(parser.ParseFile(src), src, nil)
	if len(got) != 0 {
		t.Fatalf("expected no deprecation findings, got %+v", got)
	}
}

func TestDeprecatedFunctionRuleFlagsGlobalCalls(t *testing.T) {
	src := `<?php
$a = each($arr); $b = \each($arr);
$c = create_function('$a', 'return $a;');
$d = UTF8_ENCODE($s) . utf8_decode($s);
`
	got := (&DeprecatedFunctionRule{}).Check(parser.ParseFile(src), src, nil)
	if len(got) != 5 {
		t.Fatalf("expected 5 deprecation findings, got %d: %+v", len(got), got)
	}
	if got[0].StartLine != 1 || got[0].StartCol != 5 || got[0].EndCol != 9 {
		t.Errorf("unexpected range for each(): %+v", got[0])
	}
}

func TestTextScanRulesSkipMultilineStringsAndHeredocs(t *testing.T) {
	src := `<?php
function f($values) {
    $sql = <<<SQL
    insert into device_trace_logs (a) values
    select to_char($1, 'YY'), each(x)
    SQL;
    $raw = 'CREATE UNIQUE INDEX x
        ON download_batches (file_name)';
    $now = <<<'TXT'
    utf8_encode(x)
    TXT;
    return each($values);
}
function g() {
    $rows = '1, 2';
    return <<<SQL
insert into t values
{$rows};
SQL;
}
`
	file := parser.ParseFile(src)
	got := (&DeprecatedFunctionRule{}).Check(file, src, nil)
	if len(got) != 1 || got[0].StartLine != 11 {
		t.Fatalf("expected only the real each() call on line 11, got %+v", got)
	}
	for _, f := range (&UnknownFunctionRule{}).Check(file, src, nil) {
		if f.StartLine != 11 {
			t.Errorf("unknown-function inside a string: %+v", f)
		}
	}
	if got := (&UnreachableCodeRule{}).Check(file, src, nil); len(got) != 0 {
		t.Errorf("unexpected unreachable-code findings: %+v", got)
	}
	for _, f := range (&UnusedVariableRule{}).Check(file, src, nil) {
		if strings.Contains(f.Message, "$rows") {
			t.Errorf("variable read inside a heredoc reported unused: %+v", f)
		}
	}
}

func TestDeprecatedFunctionRuleFollowsPHPVersion(t *testing.T) {
	src := "<?php\n$a = utf8_encode($s);\n$b = each($arr);\n"
	file := parser.ParseFile(src)
	names := func(version string) []string {
		var out []string
		for _, f := range (&DeprecatedFunctionRule{PHPVersion: version}).Check(file, src, nil) {
			out = append(out, f.Message[:strings.Index(f.Message, "(")])
		}
		return out
	}
	// A project that still supports PHP 7.4 can't move off utf8_encode() yet,
	// but each() was already deprecated in 7.2.
	if got := names("7.4"); len(got) != 1 || got[0] != "each" {
		t.Errorf("PHP 7.4: want only each(), got %v", got)
	}
	if got := names("8.2"); len(got) != 2 {
		t.Errorf("PHP 8.2: want utf8_encode() and each(), got %v", got)
	}
	if got := names(""); len(got) != 2 {
		t.Errorf("unknown version: want every deprecation, got %v", got)
	}
}
