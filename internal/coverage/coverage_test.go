package coverage

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

const cloverFixture = `<?xml version="1.0" encoding="UTF-8"?>
<coverage generated="1793000000">
  <project timestamp="1793000000">
    <package name="App">
      <file name="/var/www/html/app/Service.php">
        <class name="App\Service" namespace="App">
          <metrics complexity="3" methods="2" coveredmethods="1"/>
        </class>
        <line num="8" type="method" name="handle" visibility="public" complexity="2" crap="6.5" count="0"/>
        <line num="10" type="stmt" count="0"/>
        <line num="11" type="stmt" count="0"/>
        <line num="14" type="method" name="run" visibility="public" complexity="1" crap="1" count="4"/>
        <line num="16" type="stmt" count="4"/>
        <line num="17" type="stmt" count="0"/>
        <metrics loc="20" ncloc="18" statements="4" coveredstatements="1"/>
      </file>
    </package>
    <file name="/var/www/html/app/Other.php">
      <class name="App\A" namespace="App"/>
      <line num="5" type="method" name="handle" complexity="1" crap="1" count="1"/>
      <line num="6" type="stmt" count="1"/>
      <line num="20" type="method" name="handle" complexity="1" crap="2" count="0"/>
      <line num="21" type="stmt" count="0"/>
      <line num="30" type="method" name="noBody" complexity="1" crap="1" count="0"/>
    </file>
  </project>
</coverage>
`

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func setTime(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestParseCloverAssignsLinesToMethods(t *testing.T) {
	report, err := ParseClover(strings.NewReader(cloverFixture))
	if err != nil {
		t.Fatal(err)
	}
	file := report.Files["/var/www/html/app/Service.php"]
	if file == nil {
		t.Fatalf("file not found in %v", report.Files)
	}
	if file.Covered() != 1 || len(file.Lines) != 4 {
		t.Fatalf("expected 1/4 lines covered, got %d/%d", file.Covered(), len(file.Lines))
	}
	if !reflect.DeepEqual(file.Uncovered, []int{10, 11, 17}) {
		t.Errorf("uncovered lines: got %v", file.Uncovered)
	}
	handle, run := file.Methods[0], file.Methods[1]
	if handle.Name != "handle" || handle.Executable != 2 || handle.Covered != 0 || handle.CRAP != 6.5 || handle.Complexity != 2 {
		t.Errorf("unexpected handle: %+v", handle)
	}
	if run.Name != "run" || run.Executable != 2 || run.Covered != 1 || run.Hits != 4 || !reflect.DeepEqual(run.Uncovered, []int{17}) {
		t.Errorf("unexpected run: %+v", run)
	}
}

func TestParseCloverRejectsCobertura(t *testing.T) {
	cobertura := `<?xml version="1.0"?><coverage line-rate="1"><sources/><packages/></coverage>`
	if _, err := ParseClover(strings.NewReader(cobertura)); err != ErrNotClover {
		t.Fatalf("expected ErrNotClover, got %v", err)
	}
}

func TestStoreMatchesContainerPaths(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "clover.xml"), cloverFixture)
	writeFile(t, filepath.Join(root, "app", "Service.php"), "<?php\n")
	writeFile(t, filepath.Join(root, "Service.php"), "<?php\n")
	store := NewStore(root, "", nil)
	store.Refresh()

	file, _ := store.File(filepath.Join(root, "app", "Service.php"))
	if file == nil {
		t.Fatal("expected a report path inside a container to match")
	}
	if got := store.Relative(file); got != "app/Service.php" {
		t.Errorf("relative path: got %q", got)
	}
	if file, _ := store.File(filepath.Join(root, "Service.php")); file != nil {
		t.Fatal("a file name alone must not match a file in another directory")
	}
	if file, _ := store.File(filepath.Join(t.TempDir(), "app", "Service.php")); file != nil {
		t.Fatal("a file outside the workspace must not match by suffix")
	}
}

func TestStoreMethodPicksClosestSameNamedMethod(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "clover.xml"), cloverFixture)
	source := filepath.Join(root, "app", "Other.php")
	writeFile(t, source, "<?php\n")
	store := NewStore(root, "", nil)
	store.Refresh()

	if m, _ := store.Method(source, "handle", 19); m == nil || m.Line != 20 {
		t.Errorf("expected the handle on line 20, got %+v", m)
	}
	if m, _ := store.Method(source, "HANDLE", 5); m == nil || m.Line != 5 {
		t.Errorf("expected a case-insensitive match on line 5, got %+v", m)
	}
	if m, _ := store.Method(source, "noBody", 30); m != nil {
		t.Errorf("a method with no executable lines has no coverage, got %+v", m)
	}
	if m, _ := store.Method(source, "missing", 1); m != nil {
		t.Errorf("expected no method, got %+v", m)
	}
}

func TestStoreMatchesLocalReportPaths(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "src", "Service.php")
	writeFile(t, source, "<?php\n")
	writeFile(t, filepath.Join(root, "clover.xml"), strings.ReplaceAll(cloverFixture, "/var/www/html/app", filepath.ToSlash(root)+"/src"))
	store := NewStore(root, "", nil)
	store.Refresh()

	file, _ := store.File(source)
	if file == nil {
		t.Fatal("expected a report written on this machine to match")
	}
	if got := store.Relative(file); got != "src/Service.php" {
		t.Errorf("relative path: got %q", got)
	}
}

func TestStoreDetectsStaleSource(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "clover.xml")
	source := filepath.Join(root, "app", "Service.php")
	writeFile(t, report, cloverFixture)
	writeFile(t, source, "<?php\n")
	now := time.Now()
	setTime(t, source, now.Add(-time.Hour))
	setTime(t, report, now)

	store := NewStore(root, "", nil)
	store.Refresh()
	if _, stale := store.File(source); stale {
		t.Error("source older than the report must not be stale")
	}
	setTime(t, source, now.Add(time.Hour))
	if file, stale := store.File(source); file == nil || !stale {
		t.Error("source newer than the report must be stale")
	}
}

func TestStoreReadsReportPathFromPHPUnitConfig(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "phpunit.coverage.dist.xml"), `<phpunit>
  <coverage><report><clover outputFile="var/reports/custom.xml"/></report></coverage>
</phpunit>`)
	writeFile(t, filepath.Join(root, "var", "reports", "custom.xml"), cloverFixture)
	store := NewStore(root, "", nil)
	store.Refresh()

	reportPath, _, ok := store.ReportInfo()
	if !ok || reportPath != filepath.Join(root, "var", "reports", "custom.xml") {
		t.Fatalf("expected the configured output file, got %q", reportPath)
	}
}

func TestStoreReadsLegacyLoggingConfig(t *testing.T) {
	outputs := cloverOutputs([]byte(`<phpunit><logging>
  <log type="coverage-html" target="html"/>
  <log type="coverage-clover" target="build/clover.xml"/>
</logging></phpunit>`))
	if !reflect.DeepEqual(outputs, []string{"build/clover.xml"}) {
		t.Fatalf("unexpected outputs: %v", outputs)
	}
}

func TestStoreConfiguredPathReplacesDiscovery(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "clover.xml"), cloverFixture)
	store := NewStore(root, "elsewhere/missing.xml", nil)
	store.Refresh()
	if _, _, ok := store.ReportInfo(); ok {
		t.Fatal("a configured path must not fall back to discovery")
	}
}

func TestStorePrefersNewestReportAndReloads(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "clover.xml")
	second := filepath.Join(root, "build", "logs", "clover.xml")
	writeFile(t, first, cloverFixture)
	writeFile(t, second, cloverFixture)
	writeFile(t, filepath.Join(root, "app", "Renamed.php"), "<?php\n")
	now := time.Now()
	setTime(t, first, now.Add(-time.Hour))
	setTime(t, second, now)

	store := NewStore(root, "", nil)
	store.Refresh()
	if p, _, _ := store.ReportInfo(); p != second {
		t.Fatalf("expected the newest report, got %q", p)
	}

	writeFile(t, first, strings.ReplaceAll(cloverFixture, "Service.php", "Renamed.php"))
	setTime(t, first, now.Add(time.Hour))
	store.Refresh()
	if p, _, _ := store.ReportInfo(); p != first {
		t.Fatalf("expected the rewritten report, got %q", p)
	}
	if file, _ := store.File(filepath.Join(root, "app", "Renamed.php")); file == nil {
		t.Fatal("expected the new report's contents")
	}
}

func TestStoreKeepsLastReportWhenNewOneIsUnreadable(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "clover.xml")
	writeFile(t, report, cloverFixture)
	writeFile(t, filepath.Join(root, "app", "Service.php"), "<?php\n")
	now := time.Now()
	setTime(t, report, now.Add(-time.Hour))
	store := NewStore(root, "", nil)
	store.Refresh()

	// A test run still writing the report leaves truncated XML behind.
	writeFile(t, report, cloverFixture[:len(cloverFixture)/2])
	setTime(t, report, now)
	store.Refresh()
	if file, _ := store.File(filepath.Join(root, "app", "Service.php")); file == nil {
		t.Fatal("expected the previous report to stay loaded")
	}
}

func TestStoreForgetsDeletedReport(t *testing.T) {
	root := t.TempDir()
	report := filepath.Join(root, "clover.xml")
	writeFile(t, report, cloverFixture)
	store := NewStore(root, "", nil)
	store.Refresh()
	if err := os.Remove(report); err != nil {
		t.Fatal(err)
	}
	store.Refresh()
	if _, _, ok := store.ReportInfo(); ok {
		t.Fatal("expected no report after it was deleted")
	}
}

func TestStoreWithNoReport(t *testing.T) {
	store := NewStore(t.TempDir(), "", nil)
	store.Refresh()
	if m, stale := store.Method("/x/y.php", "run", 1); m != nil || stale {
		t.Fatal("expected no coverage without a report")
	}
	if files := store.Files(); files != nil {
		t.Fatalf("expected no files, got %v", files)
	}
}

func TestLocalPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "app/Service.php"), "<?php\n")
	writeFile(t, filepath.Join(root, "clover.xml"), cloverFixture)
	store := NewStore(root, "", nil)
	store.Refresh()

	byName := map[string]string{}
	for _, f := range store.Files() {
		byName[filepath.Base(f.Path)] = store.LocalPath(f)
	}
	if byName["Service.php"] != filepath.Join(root, "app/Service.php") {
		t.Errorf("expected the workspace path, got %q", byName["Service.php"])
	}
	if byName["Other.php"] != "" {
		t.Errorf("expected nothing for a file not on this machine, got %q", byName["Other.php"])
	}
}

func TestMethodEndLine(t *testing.T) {
	report, err := ParseClover(strings.NewReader(cloverFixture))
	if err != nil {
		t.Fatal(err)
	}
	other := report.Files["/var/www/html/app/Other.php"]
	if other.Methods[0].EndLine != 6 {
		t.Errorf("expected the last executable line, got %d", other.Methods[0].EndLine)
	}
	if other.Methods[2].EndLine != 30 {
		t.Errorf("expected the declaration line for a method with no lines, got %d", other.Methods[2].EndLine)
	}
}

func TestOnChangeFiresOncePerReport(t *testing.T) {
	root := t.TempDir()
	reportPath := filepath.Join(root, "clover.xml")
	writeFile(t, reportPath, cloverFixture)
	store := NewStore(root, "", nil)
	changes := 0
	store.OnChange(func() { changes++ })

	store.Refresh()
	store.Refresh()
	if changes != 1 {
		t.Fatalf("expected 1 change for the first load, got %d", changes)
	}

	setTime(t, reportPath, time.Now().Add(time.Minute))
	store.Refresh()
	if changes != 2 {
		t.Fatalf("expected a change for a newer report, got %d", changes)
	}

	if err := os.Remove(reportPath); err != nil {
		t.Fatal(err)
	}
	store.Refresh()
	store.Refresh()
	if changes != 3 {
		t.Fatalf("expected one change when the report goes away, got %d", changes)
	}
}

func TestBadReportIsOnlyReadOnce(t *testing.T) {
	root := t.TempDir()
	reportPath := filepath.Join(root, "clover.xml")
	writeFile(t, reportPath, "<coverage><project>")
	store := NewStore(root, "", nil)
	store.Refresh()
	if store.failedPath != reportPath {
		t.Fatalf("expected the bad report to be remembered, got %q", store.failedPath)
	}

	// A good report written over it is picked up once its time changes.
	writeFile(t, reportPath, cloverFixture)
	setTime(t, reportPath, time.Now().Add(time.Minute))
	store.Refresh()
	if _, _, ok := store.ReportInfo(); !ok {
		t.Fatal("expected the rewritten report to load")
	}
}
