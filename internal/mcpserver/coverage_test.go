package mcpserver

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/coverage"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

const coverageTestReport = `<?xml version="1.0"?>
<coverage generated="1"><project timestamp="1">
  <file name="/var/www/html/app/Service.php">
    <line num="8" type="method" name="handle" complexity="2" crap="6" count="0"/>
    <line num="10" type="stmt" count="0"/>
    <line num="11" type="stmt" count="0"/>
    <line num="14" type="method" name="run" complexity="1" crap="1" count="4"/>
    <line num="16" type="stmt" count="4"/>
    <line num="18" type="stmt" count="0"/>
    <line num="20" type="method" name="done" complexity="1" crap="1" count="1"/>
    <line num="21" type="stmt" count="1"/>
  </file>
  <file name="/var/www/html/app/Full.php">
    <line num="3" type="method" name="ok" complexity="1" crap="1" count="1"/>
    <line num="4" type="stmt" count="1"/>
  </file>
</project></coverage>
`

func coverageTestService(t *testing.T, report string) *Service {
	t.Helper()
	root, err := workspace.ResolveRoot(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "app"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"app/Service.php", "app/Full.php", "app/NotInReport.php"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("<?php\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if report != "" {
		// Written last so the sources aren't newer than the report.
		if err := os.WriteFile(filepath.Join(root, "clover.xml"), []byte(report), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return &Service{RootPath: root, coverage: coverage.NewStore(root, "", nil)}
}

func TestPhpCoverageFile(t *testing.T) {
	out, err := coverageTestService(t, coverageTestReport).phpCoverage("app/Service.php")
	if err != nil {
		t.Fatal(err)
	}
	if !out.ReportFound || out.ReportPath != "clover.xml" || out.File == nil {
		t.Fatalf("unexpected output: %+v", out)
	}
	f := out.File
	if f.Path != "app/Service.php" || f.CoveredLines != 2 || f.ExecutableLines != 5 || f.Percent != 40 {
		t.Errorf("unexpected totals: %+v", f)
	}
	if !reflect.DeepEqual(f.UncoveredLines, []string{"10-11", "18"}) {
		t.Errorf("uncovered lines: %v", f.UncoveredLines)
	}
	if len(f.UntestedMethods) != 1 || f.UntestedMethods[0].Name != "handle" || f.UntestedMethods[0].CRAP != 6 {
		t.Errorf("untested methods: %+v", f.UntestedMethods)
	}
	if len(f.PartialMethods) != 1 || f.PartialMethods[0].Name != "run" {
		t.Errorf("partial methods: %+v", f.PartialMethods)
	}
}

func TestPhpCoverageProject(t *testing.T) {
	out, err := coverageTestService(t, coverageTestReport).phpCoverage("")
	if err != nil {
		t.Fatal(err)
	}
	if out.CoveredLines != 3 || out.ExecutableLines != 6 || out.Percent != 50 {
		t.Errorf("unexpected totals: %+v", out)
	}
	if len(out.LeastCovered) != 2 || out.LeastCovered[0].Path != "app/Service.php" {
		t.Errorf("least covered: %+v", out.LeastCovered)
	}
}

func TestPhpCoverageUnknownIsNotUntested(t *testing.T) {
	out, err := coverageTestService(t, coverageTestReport).phpCoverage("app/NotInReport.php")
	if err != nil {
		t.Fatal(err)
	}
	if out.File != nil || !strings.Contains(out.Note, "unknown") {
		t.Errorf("expected unknown coverage, got %+v", out)
	}
}

func TestPhpCoverageWithoutReport(t *testing.T) {
	out, err := coverageTestService(t, "").phpCoverage("")
	if err != nil {
		t.Fatal(err)
	}
	if out.ReportFound || !strings.Contains(out.Note, "--coverage-clover") {
		t.Errorf("expected a note on creating a report, got %+v", out)
	}
}

func TestLineRanges(t *testing.T) {
	got := lineRanges([]int{3, 4, 5, 9, 11, 12})
	if !reflect.DeepEqual(got, []string{"3-5", "9", "11-12"}) {
		t.Errorf("got %v", got)
	}
}

func TestPhpCoverageOfAMissingFileIsAnError(t *testing.T) {
	if out, err := coverageTestService(t, coverageTestReport).phpCoverage("app/Gone.php"); err == nil || !strings.Contains(err.Error(), "resolving") {
		t.Errorf("phpCoverage of a missing file = %+v, %v", out, err)
	}
}

func TestPhpCoverageToolAnswersWithoutAReport(t *testing.T) {
	session, _ := connectedService(t, map[string]string{".tusk-php.json": plainProjectConfig})
	out, err := callTool(t, session, "php_coverage", map[string]any{})
	if err != nil {
		t.Fatal(err)
	}
	if note, _ := out["note"].(string); !strings.Contains(note, "--coverage-clover") {
		t.Errorf("php_coverage without a report = %+v", out)
	}
}
