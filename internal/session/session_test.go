package session

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/diagnostics"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

const undefinedVariable = "<?php\nfunction f() {\n    echo $missing;\n}\n"

func codes(diags []diagnostics.Diagnostic) []string {
	var out []string
	for _, d := range diags {
		out = append(out, d.Code)
	}
	return out
}

func newSession(t *testing.T, opts Options) *Session {
	t.Helper()
	ws := workspace.New(workspace.Options{RootPath: t.TempDir(), Framework: "none"})
	return New(ws, opts)
}

func TestAnalyzeReportsNativeFindings(t *testing.T) {
	s := newSession(t, Options{})
	const uri = "file:///probe/a.php"
	for name, diags := range map[string][]diagnostics.Diagnostic{
		"Analyze":    s.Analyze(uri, undefinedVariable),
		"AnalyzeAll": s.AnalyzeAll(uri, undefinedVariable),
	} {
		if !slices.Contains(codes(diags), string(rule.UndefinedVariable)) {
			t.Errorf("%s: got %v, want %s", name, codes(diags), rule.UndefinedVariable)
		}
	}
	found := false
	for _, f := range s.NativeFindings(undefinedVariable) {
		found = found || f.Code == rule.UndefinedVariable
	}
	if !found {
		t.Errorf("NativeFindings didn't report %s", rule.UndefinedVariable)
	}
	s.Saved(uri, undefinedVariable)
	s.Closed(uri)
	if s.Config() == nil {
		t.Error("no configuration")
	}
}

func TestOutsideLaravelViewsAreUntyped(t *testing.T) {
	s := newSession(t, Options{})
	if s.ViewTypes() != nil {
		t.Error("view types outside a Laravel app")
	}
	if s.Views() == nil || s.BladeProviders() == nil || s.Components() == nil || s.Blade() == nil {
		t.Error("the Blade pieces aren't wired")
	}
	if got := s.Blade().Read; got == nil {
		t.Error("Blade reads no files")
	}
}

func TestBladeReadsThroughTheGivenReader(t *testing.T) {
	var asked []string
	s := newSession(t, Options{Read: func(uri string) string {
		asked = append(asked, uri)
		return "unsaved"
	}})
	if got := s.Blade().Read("file:///x.blade.php"); got != "unsaved" || len(asked) != 1 {
		t.Errorf("Read = %q after %v; want the given reader's answer", got, asked)
	}
}

func TestReadFileReadsFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "a.php")
	if err := os.WriteFile(path, []byte("<?php"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile("file://" + path); got != "<?php" {
		t.Errorf("readFile = %q", got)
	}
	if got := readFile("file://" + path + ".missing"); got != "" {
		t.Errorf("readFile of a missing file = %q", got)
	}
}

func TestOpenIndexesTheLaravelFixture(t *testing.T) {
	root, err := filepath.Abs("../../testdata/laravel")
	if err != nil {
		t.Fatal(err)
	}
	s, err := Open(context.Background(), root, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if s.ViewTypes() == nil {
		t.Error("no view types in a Laravel app")
	}
	files := s.SourceFiles()
	welcome := filepath.Join(root, "resources", "views", "welcome.blade.php")
	if !slices.Contains(files, welcome) {
		t.Errorf("SourceFiles leaves out %s", welcome)
	}
	if !slices.ContainsFunc(files, func(p string) bool {
		return strings.HasPrefix(p, filepath.Join(root, "app")+string(filepath.Separator))
	}) {
		t.Error("SourceFiles leaves out app/")
	}
}

func TestOpenFailsOnABrokenConfiguration(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".tusk-php.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(context.Background(), root, Options{}); err == nil {
		t.Error("opened a project whose configuration doesn't parse")
	}
}
