package analyzer

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// setupBuiltinAnalyzer creates an Analyzer with builtins registered and an
// in-memory PHP source file indexed under the given URI.
func setupBuiltinAnalyzer(t *testing.T, uri, src string) (*Analyzer, string) {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource(uri, src, symbols.SourceProject)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	return NewAnalyzer(idx, ca), src
}

// posOf finds the first occurrence of target on any line and returns its position.
func posOf(t *testing.T, source, target string) protocol.Position {
	t.Helper()
	for i, line := range strings.Split(source, "\n") {
		col := strings.Index(line, target)
		if col >= 0 {
			return protocol.Position{Line: i, Character: col}
		}
	}
	t.Fatalf("could not find %q in source", target)
	return protocol.Position{}
}

// TestFindDefinitionWithManualFlagOff_BuiltinFunction ensures that when
// openManualEnabled is false, calling FindDefinitionWithManual returns the same
// nil location as FindDefinition (no URL emitted for builtins).
func TestFindDefinitionWithManualFlagOff_BuiltinFunction(t *testing.T) {
	const uri = "file:///project/test.php"
	const src = "<?php\n$x = array_map(fn($v) => $v, []);\n"
	a, source := setupBuiltinAnalyzer(t, uri, src)

	pos := posOf(t, source, "array_map")
	result := a.FindDefinitionWithManual(uri, source, pos, false, "")

	if result.Location != nil {
		t.Errorf("expected nil Location for builtin with flag off, got %+v", result.Location)
	}
	if result.ExternalManualURL != "" {
		t.Errorf("expected empty ExternalManualURL with flag off, got %q", result.ExternalManualURL)
	}
}

// TestFindDefinitionWithManualFlagOn_BuiltinFunction ensures that when
// openManualEnabled is true, a builtin function returns an ExternalManualURL
// and a nil Location.
func TestFindDefinitionWithManualFlagOn_BuiltinFunction(t *testing.T) {
	const uri = "file:///project/test.php"
	const src = "<?php\n$x = array_map(fn($v) => $v, []);\n"
	a, source := setupBuiltinAnalyzer(t, uri, src)

	pos := posOf(t, source, "array_map")
	result := a.FindDefinitionWithManual(uri, source, pos, true, "")

	if result.Location != nil {
		t.Errorf("expected nil Location for builtin, got %+v", result.Location)
	}
	want := "https://www.php.net/manual/en/function.array-map.php"
	if result.ExternalManualURL != want {
		t.Errorf("ExternalManualURL = %q, want %q", result.ExternalManualURL, want)
	}
}

// TestFindDefinitionWithManualFlagOn_BuiltinClass ensures that when
// openManualEnabled is true, a builtin class name returns an ExternalManualURL
// pointing at the class manual page.
func TestFindDefinitionWithManualFlagOn_BuiltinClass(t *testing.T) {
	const uri = "file:///project/test.php"
	const src = "<?php\n$d = new DateTime('now');\n"
	a, source := setupBuiltinAnalyzer(t, uri, src)

	pos := posOf(t, source, "DateTime")
	result := a.FindDefinitionWithManual(uri, source, pos, true, "")

	if result.Location != nil {
		t.Errorf("expected nil Location for builtin class, got %+v", result.Location)
	}
	want := "https://www.php.net/manual/en/class.datetime.php"
	if result.ExternalManualURL != want {
		t.Errorf("ExternalManualURL = %q, want %q", result.ExternalManualURL, want)
	}
}

// TestFindDefinitionWithManualFlagOn_BuiltinMethod ensures that when
// openManualEnabled is true, calling a method on a builtin class (ReflectionClass)
// returns the method's manual URL and nil Location.
func TestFindDefinitionWithManualFlagOn_BuiltinMethod(t *testing.T) {
	const uri = "file:///project/test.php"
	const src = "<?php\n$rc = new ReflectionClass('stdClass');\n$name = $rc->getName();\n"
	a, source := setupBuiltinAnalyzer(t, uri, src)

	// Position on "getName" in $rc->getName()
	pos := posOf(t, source, "getName")
	result := a.FindDefinitionWithManual(uri, source, pos, true, "")

	if result.Location != nil {
		t.Errorf("expected nil Location for builtin method, got %+v", result.Location)
	}
	want := "https://www.php.net/manual/en/reflectionclass.getname.php"
	if result.ExternalManualURL != want {
		t.Errorf("ExternalManualURL = %q, want %q", result.ExternalManualURL, want)
	}
}

// TestFindDefinitionWithManualFlagOn_ProjectSymbol ensures that when
// openManualEnabled is true, a user-defined function returns a Location (not a URL).
func TestFindDefinitionWithManualFlagOn_ProjectSymbol(t *testing.T) {
	const defURI = "file:///project/helpers.php"
	const callURI = "file:///project/main.php"
	const defSrc = "<?php\nfunction myFunc(): void {}\n"
	const callSrc = "<?php\nmyFunc();\n"

	idx := symbols.NewIndex()
	idx.IndexFileWithSource(defURI, defSrc, symbols.SourceProject)
	idx.IndexFileWithSource(callURI, callSrc, symbols.SourceProject)
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	a := NewAnalyzer(idx, ca)

	pos := posOf(t, callSrc, "myFunc")
	result := a.FindDefinitionWithManual(callURI, callSrc, pos, true, "")

	if result.ExternalManualURL != "" {
		t.Errorf("expected empty ExternalManualURL for project symbol, got %q", result.ExternalManualURL)
	}
	if result.Location == nil {
		t.Fatal("expected non-nil Location for project symbol")
	}
	if !strings.Contains(result.Location.URI, "helpers.php") {
		t.Errorf("expected Location URI to contain helpers.php, got %q", result.Location.URI)
	}
}

// TestFindDefinitionWithManualFlagOn_UnresolvedChain ensures that when the
// access chain cannot be resolved (unknown variable type), the zero DefinitionResult
// is returned.
func TestFindDefinitionWithManualFlagOn_UnresolvedChain(t *testing.T) {
	const uri = "file:///project/test.php"
	// $x has no known type — the chain cannot be resolved.
	const src = "<?php\n$x->name;\n"
	a, source := setupBuiltinAnalyzer(t, uri, src)

	pos := posOf(t, source, "name")
	result := a.FindDefinitionWithManual(uri, source, pos, true, "")

	if result.Location != nil {
		t.Errorf("expected nil Location for unresolved chain, got %+v", result.Location)
	}
	if result.ExternalManualURL != "" {
		t.Errorf("expected empty ExternalManualURL for unresolved chain, got %q", result.ExternalManualURL)
	}
}

// TestFindDefinitionWithManualLocaleForwarded ensures that the locale parameter
// is forwarded correctly to the URL builder.
func TestFindDefinitionWithManualLocaleForwarded(t *testing.T) {
	const uri = "file:///project/test.php"
	const src = "<?php\n$x = array_map(fn($v) => $v, []);\n"
	a, source := setupBuiltinAnalyzer(t, uri, src)

	pos := posOf(t, source, "array_map")
	result := a.FindDefinitionWithManual(uri, source, pos, true, "es")

	want := "https://www.php.net/manual/es/function.array-map.php"
	if result.ExternalManualURL != want {
		t.Errorf("ExternalManualURL = %q, want %q (locale should be forwarded)", result.ExternalManualURL, want)
	}
}
