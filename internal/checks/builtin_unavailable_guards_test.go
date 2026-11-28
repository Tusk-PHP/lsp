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
		got = append(got, string(f.Code)+": "+f.Message)
	}
	for _, f := range (&UnknownFunctionRule{}).Check(file, src, idx) {
		got = append(got, string(f.Code)+": "+f.Message)
	}
	for _, f := range (&UnknownClassRule{}).Check(file, src, idx) {
		got = append(got, string(f.Code)+": "+f.Message)
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

// Symfony's cache adapters test `$connection instanceof \Redis` without
// ext-redis: the test is just false where PHP has no Redis class, so it is
// never reported. Static access and `new` still are.
func TestBuiltinUnavailableSkipsTheClassAnInstanceofTests(t *testing.T) {
	src := "<?php\nfunction is_redis($c) {\n    return $c instanceof \\Redis || $c instanceof Redis;\n}\nfunction make() { return new \\Redis(); }\n"
	got := unavailableAndUnknown(t, src, "8.4")
	if len(got) != 1 || !strings.Contains(got[0], "Class 'Redis' requires ext-redis") {
		t.Errorf("only the `new` is reported, got %q", got)
	}
}

func TestBuiltinUnavailableKnowsClassesFromTheGeneratedTable(t *testing.T) {
	cases := []struct{ src, version, want string }{
		{"<?php\nnew \\WeakMap();\n", "7.4", "Class 'WeakMap' requires PHP >= 8.0"},
		{"<?php\nnew \\ReflectionEnum('E');\n", "8.0", "Class 'ReflectionEnum' requires PHP >= 8.1"},
		{"<?php\nfunction f($h) { return $h instanceof \\CurlHandle; }\nnew \\CurlHandle();\n", "7.4", "Class 'CurlHandle' requires PHP >= 8.0"},
		{"<?php\nnew \\Brotli\\Compress\\Context();\n", "8.4", "Class 'Brotli\\Compress\\Context' requires ext-brotli"},
		{"<?php\nnew \\Redis();\n", "8.4", "Class 'Redis' requires ext-redis"},
	}
	for _, c := range cases {
		got := unavailableAndUnknown(t, c.src, c.version)
		if len(got) != 1 || !strings.Contains(got[0], c.want) {
			t.Errorf("%q on PHP %s: got %q, want one %q", c.src, c.version, got, c.want)
		}
	}
	if got := unavailableAndUnknown(t, "<?php\nnew \\Collator('en');\nnew \\NumberFormatter('en', 1);\n", "8.4"); len(got) != 0 {
		t.Errorf("intl ships with PHP's source; its classes aren't tied to ext-intl being declared: %q", got)
	}
}
