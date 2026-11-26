package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func unavailableConstants(t *testing.T, src string) []string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})
	var got []string
	for _, f := range (&BuiltinUnavailableRule{PHPVersion: "7.4"}).Check(parser.ParseFile(src), src, idx) {
		if strings.HasPrefix(f.Message, "Constant") {
			got = append(got, f.Message)
		}
	}
	return got
}

func TestBuiltinUnavailableReportsOnlyGlobalConstantUses(t *testing.T) {
	quiet := []string{
		"<?php\n$x = 'FILTER_VALIDATE_BOOL';\n",
		"<?php\n$x = \"uses FILTER_VALIDATE_BOOL here\";\n",
		"<?php\n// FILTER_VALIDATE_BOOL\n/* FILTER_VALIDATE_BOOL */\n",
		"<?php\n$x = Filters::FILTER_VALIDATE_BOOL;\n",
		"<?php\n$x = $filters->FILTER_VALIDATE_BOOL;\n",
		"<?php\n$x = Lib\\FILTER_VALIDATE_BOOL;\n",
		"<?php\n$x = new FILTER_VALIDATE_BOOL();\n",
		"<?php\n$x = FILTER_VALIDATE_BOOL::class;\n",
		"<?php\n$x = FILTER_VALIDATE_BOOL(1);\n",
		"<?php\nfunction FILTER_VALIDATE_BOOL() {}\n",
		"<?php\nclass FILTER_VALIDATE_BOOL {}\n",
		"<?php\nclass C { const FILTER_VALIDATE_BOOL = 1; }\n",
		"<?php\nfunction f(FILTER_VALIDATE_BOOL $x) {}\n",
		"<?php\n$ok = $x instanceof FILTER_VALIDATE_BOOL;\n",
		"<?php\nf(FILTER_VALIDATE_BOOL: 1);\n",
		"<?php\nenum E { case FILTER_VALIDATE_BOOL; }\n",
		"<?php\nuse Lib\\FILTER_VALIDATE_BOOL;\n",
		"<p>FILTER_VALIDATE_BOOL</p>\n",
		"<?php\n$x = \"$a[FILTER_VALIDATE_BOOL]\";\n",
		"<?php\n$x = <<<TXT\nFILTER_VALIDATE_BOOL $a[FILTER_VALIDATE_BOOL]\nTXT;\n",
		"<?php\nfunction f(): FILTER_VALIDATE_BOOL { return 1; }\n",
		"<?php\n#[FILTER_VALIDATE_BOOL]\nfunction f() {}\n",
		"<?php\nFILTER_VALIDATE_BOOL:\ngoto FILTER_VALIDATE_BOOL;\n",
	}
	for _, src := range quiet {
		if got := unavailableConstants(t, src); len(got) != 0 {
			t.Errorf("%q: got %q", src, got)
		}
	}
	reported := []string{
		"<?php\n$x = FILTER_VALIDATE_BOOL;\n",
		"<?php\n$x = \\FILTER_VALIDATE_BOOL;\n",
		"<?php\nfilter_var($v, FILTER_VALIDATE_BOOL);\n",
		"<?php\n$x = $y ? 1 : FILTER_VALIDATE_BOOL;\n",
		"<?php\nswitch ($f) { case FILTER_VALIDATE_BOOL: break; }\n",
		"<?php\n$x = \"{$a[FILTER_VALIDATE_BOOL]}\";\n",
		"<?php\n$x = <<<TXT\n{$a[FILTER_VALIDATE_BOOL]}\nTXT;\n",
	}
	for _, src := range reported {
		got := unavailableConstants(t, src)
		if len(got) != 1 || got[0] != "Constant 'FILTER_VALIDATE_BOOL' requires PHP >= 8.0" {
			t.Errorf("%q: got %q", src, got)
		}
	}
}
