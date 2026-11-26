package checks

import (
	"slices"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func unavailableConstantMessages(t *testing.T, version, source string) []string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: version})
	var got []string
	for _, f := range (&BuiltinUnavailableRule{PHPVersion: version}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		got = append(got, f.Message)
	}
	return got
}

func TestBuiltinUnavailableReportsAConstantOfANewerPHP(t *testing.T) {
	got := unavailableConstantMessages(t, "8.1", "<?php\n$format = DATE_ISO8601_EXPANDED;\n$id = T_MATCH;\n")
	want := []string{"Constant 'DATE_ISO8601_EXPANDED' requires PHP >= 8.2"}
	if !slices.Equal(got, want) {
		t.Errorf("on 8.1, only the 8.2 constant is reported: got %v, want %v", got, want)
	}
	if got := unavailableConstantMessages(t, "8.2", "<?php\n$format = DATE_ISO8601_EXPANDED;\n"); len(got) != 0 {
		t.Errorf("on 8.2 it is available: %v", got)
	}
}

func TestBuiltinUnavailableIgnoresConstantsThatDependOnALibrary(t *testing.T) {
	source := "<?php\ncurl_setopt($ch, CURLOPT_ALTSVC_CTRL, CURLALTSVC_H1);\n$s = nl_langinfo(CURRENCY_SYMBOL);\n"
	if got := unavailableConstantMessages(t, "8.0", source); len(got) != 0 {
		t.Errorf("libcurl's and the C library's constants come and go with those libraries, not PHP: %v", got)
	}
}

func TestBuiltinUnavailableHonoursDefinedGuards(t *testing.T) {
	source := "<?php\nif (\\defined('T_MATCH')) {\n    $id = T_MATCH;\n}\n$attr = T_ATTRIBUTE;\n"
	got := unavailableConstantMessages(t, "7.4", source)
	want := []string{"Constant 'T_ATTRIBUTE' requires PHP >= 8.0"}
	if !slices.Equal(got, want) {
		t.Errorf("a constant the file checks with defined() is not reported, another one is: got %v, want %v", got, want)
	}
}

func TestBuiltinUnavailableIgnoresAConstantTheProjectDeclares(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "7.4"})
	idx.IndexFileWithSource("file:///compat.php", "<?php\nconst IMAGETYPE_AVIF = 19;\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///polyfill.php", "<?php\nif (!defined('IMAGETYPE_HEIF')) {\n    define('IMAGETYPE_HEIF', 20);\n}\n", symbols.SourceProject)
	source := "<?php\n$types = [IMAGETYPE_AVIF, IMAGETYPE_HEIF, T_MATCH];\n"
	var got []string
	for _, f := range (&BuiltinUnavailableRule{PHPVersion: "7.4"}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		got = append(got, f.Message)
	}
	if want := []string{"Constant 'T_MATCH' requires PHP >= 8.0"}; !slices.Equal(got, want) {
		t.Errorf("a constant the project declares itself, as a polyfill does, is not reported: got %v, want %v", got, want)
	}
}

func TestBuiltinUnavailableReportsConstantsWithoutAnIndex(t *testing.T) {
	source := "<?php\n$id = T_MATCH;\n"
	var got []string
	for _, f := range (&BuiltinUnavailableRule{PHPVersion: "7.4"}).Check(parser.ParseFile(source, token.Latest), source, nil) {
		got = append(got, f.Message)
	}
	if want := []string{"Constant 'T_MATCH' requires PHP >= 8.0"}; !slices.Equal(got, want) {
		t.Errorf("with no index to find a project's own declaration, the constant is reported: got %v, want %v", got, want)
	}
}

func TestBuiltinUnavailableOnlyTrustsDefinedCalledWithALiteralName(t *testing.T) {
	source := "<?php\n$name = 'T_MATCH';\nif (defined($name) && $flags->defined) {\n    $id = T_MATCH;\n}\n"
	got := unavailableConstantMessages(t, "7.4", source)
	if want := []string{"Constant 'T_MATCH' requires PHP >= 8.0"}; !slices.Equal(got, want) {
		t.Errorf("defined() of a variable, and a property named defined, guard nothing: got %v, want %v", got, want)
	}
}
