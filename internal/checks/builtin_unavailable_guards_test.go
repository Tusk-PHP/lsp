package checks

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func unavailableAndUnknown(t *testing.T, src, version string) []string {
	t.Helper()
	file := parser.ParseFile(src)
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: version})
	var got []string
	for _, f := range (&BuiltinUnavailableRule{PHPVersion: version}).Check(file, src, idx) {
		got = append(got, f.Code+": "+f.Message)
	}
	for _, f := range (&UnknownFunctionRule{}).Check(file, src, idx) {
		got = append(got, f.Code+": "+f.Message)
	}
	for _, f := range (&UnknownClassRule{}).Check(file, src, idx) {
		got = append(got, f.Code+": "+f.Message)
	}
	return got
}

// Symfony on PHP 8.4: #[\NoDiscard('…')] is an attribute an older PHP
// ignores, not a call to a function named NoDiscard.
func TestBuiltinUnavailableSkipsAttributeArguments(t *testing.T) {
	src := "<?php\nfinal class P {\n    #[\\NoDiscard('as P is immutable')]\n    public function with(): static { return clone $this; }\n}\n"
	for _, msg := range unavailableAndUnknown(t, src, "8.4") {
		if strings.Contains(msg, "Function") || strings.Contains(msg, "unknown") {
			t.Errorf("attribute reported as a call: %s", msg)
		}
	}
}

// Symfony's CurlClientState returns early on older PHP before calling an 8.5
// function, and a newer builtin is reported once, not also as unknown.
func TestBuiltinUnavailableFollowsVersionChecksAndReportsOnce(t *testing.T) {
	guarded := "<?php\nfunction share() {\n    if (\\PHP_VERSION_ID < 80500) {\n        return null;\n    }\n    return curl_share_init_persistent([]);\n}\n"
	if got := unavailableAndUnknown(t, guarded, "8.4"); len(got) != 0 {
		t.Errorf("version-checked file: got %q", got)
	}
	plain := "<?php\n$c = clamp(5, 1, 3);\n$d = \\Time\\Duration::fromSeconds(1);\n$e = \\Uri\\WhatWg\\url_percent_encode('a', $mode);\n"
	want := []string{
		"builtin-unavailable: Function 'clamp' requires PHP >= 8.6",
		"builtin-unavailable: Class 'Time\\Duration' requires PHP >= 8.6",
		"builtin-unavailable: Function 'Uri\\WhatWg\\url_percent_encode' requires PHP >= 8.6",
	}
	got := unavailableAndUnknown(t, plain, "8.5")
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Errorf("got %q", got)
	}
}
