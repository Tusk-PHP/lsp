package lsp

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fileCoverageClover = `<?xml version="1.0" encoding="UTF-8"?>
<coverage generated="1793000000">
  <project timestamp="1793000000">
    <file name="/var/www/html/app/Service.php">
      <line num="3" type="method" name="handle" complexity="1" crap="1" count="2"/>
      <line num="4" type="stmt" count="2"/>
      <line num="5" type="stmt" count="0"/>
      <line num="8" type="method" name="noBody" complexity="1" crap="1" count="0"/>
    </file>
    <file name="/var/www/html/app/Fresh Name.php">
      <line num="3" type="stmt" count="1"/>
    </file>
    <file name="/somewhere/else/Gone.php">
      <line num="3" type="stmt" count="0"/>
    </file>
  </project>
</coverage>
`

// coverageProject writes a project whose report is newer than its source files.
func coverageProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, name := range []string{"app/Service.php", "app/Fresh Name.php", "app/Untested.php"} {
		p := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte("<?php\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		old := time.Now().Add(-time.Hour)
		if err := os.Chtimes(p, old, old); err != nil {
			t.Fatal(err)
		}
	}
	writeReport(t, root, fileCoverageClover)
	return root
}

func writeReport(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "clover.xml"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func coverageHarness(t *testing.T, root string, options map[string]interface{}) (*lspHarness, map[string]interface{}) {
	t.Helper()
	h := newHarness(t)
	id := h.send("initialize", map[string]interface{}{
		"rootUri":               (&url.URL{Scheme: "file", Path: root}).String(),
		"capabilities":          map[string]interface{}{},
		"initializationOptions": options,
	})
	init := h.readResponse(id)
	h.notify("initialized", map[string]interface{}{})
	return h, init
}

func fileCoverageRequest(h *lspHarness, path string) interface{} {
	id := h.send("tusk/fileCoverage", map[string]interface{}{
		"textDocument": map[string]interface{}{"uri": (&url.URL{Scheme: "file", Path: path}).String()},
	})
	return h.readResponse(id)["result"]
}

func TestCoverageIsAdvertised(t *testing.T) {
	h, init := coverageHarness(t, coverageProject(t), nil)
	defer h.close()
	caps := init["result"].(map[string]interface{})["capabilities"].(map[string]interface{})
	experimental, _ := caps["experimental"].(map[string]interface{})
	if experimental["tuskCoverage"] != true {
		t.Fatalf("tuskCoverage not advertised: %v", caps["experimental"])
	}
}

func TestFileCoverageReturnsLinesAndMethods(t *testing.T) {
	root := coverageProject(t)
	h, _ := coverageHarness(t, root, nil)
	defer h.close()
	h.send("tusk/coverageReport", nil) // waits for the report to load
	h.readResponse(h.nextID - 1)

	result, ok := fileCoverageRequest(h, filepath.Join(root, "app/Service.php")).(map[string]interface{})
	if !ok {
		t.Fatal("expected coverage for a file in the report")
	}
	if result["stale"] != false {
		t.Errorf("expected fresh coverage, got %v", result["stale"])
	}
	lines := result["lines"].([]interface{})
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %v", lines)
	}
	first := lines[0].(map[string]interface{})
	if first["line"] != float64(3) || first["hits"] != float64(2) {
		t.Errorf("expected 0-based line 3 with 2 hits, got %v", first)
	}
	second := lines[1].(map[string]interface{})
	if second["line"] != float64(4) || second["hits"] != float64(0) {
		t.Errorf("expected 0-based line 4 with no hits, got %v", second)
	}
	methods := result["methods"].([]interface{})
	if len(methods) != 1 {
		t.Fatalf("a method with no executable lines must not be listed: %v", methods)
	}
	m := methods[0].(map[string]interface{})
	if m["name"] != "handle" || m["line"] != float64(2) || m["endLine"] != float64(4) || m["covered"] != float64(1) || m["executable"] != float64(2) {
		t.Errorf("unexpected method: %v", m)
	}
}

func TestFileCoverageUnknownIsNull(t *testing.T) {
	root := coverageProject(t)
	h, _ := coverageHarness(t, root, nil)
	defer h.close()
	h.send("tusk/coverageReport", nil)
	h.readResponse(h.nextID - 1)

	if got := fileCoverageRequest(h, filepath.Join(root, "app/Untested.php")); got != nil {
		t.Fatalf("a file missing from the report must be unknown, got %v", got)
	}
}

func TestFileCoverageWithNoReportIsNull(t *testing.T) {
	root := t.TempDir()
	h, _ := coverageHarness(t, root, nil)
	defer h.close()
	id := h.send("tusk/coverageReport", nil)
	if got := h.readResponse(id)["result"]; got != nil {
		t.Fatalf("expected no report, got %v", got)
	}
	if got := fileCoverageRequest(h, filepath.Join(root, "a.php")); got != nil {
		t.Fatalf("expected unknown coverage, got %v", got)
	}
}

func TestFileCoverageMarksStale(t *testing.T) {
	root := coverageProject(t)
	source := filepath.Join(root, "app/Service.php")
	future := time.Now().Add(time.Hour)
	if err := os.Chtimes(source, future, future); err != nil {
		t.Fatal(err)
	}
	h, _ := coverageHarness(t, root, nil)
	defer h.close()
	id := h.send("tusk/coverageReport", nil)
	report := h.readResponse(id)["result"].(map[string]interface{})
	var staleFiles []string
	for _, f := range report["files"].([]interface{}) {
		file := f.(map[string]interface{})
		if file["stale"] == true {
			staleFiles = append(staleFiles, file["uri"].(string))
		}
	}
	if len(staleFiles) != 1 || !strings.HasSuffix(staleFiles[0], "/app/Service.php") {
		t.Fatalf("expected only Service.php to be stale, got %v", staleFiles)
	}
	if result := fileCoverageRequest(h, source).(map[string]interface{}); result["stale"] != true {
		t.Fatalf("expected stale coverage, got %v", result["stale"])
	}
}

func TestCoverageReportListsLocalFiles(t *testing.T) {
	root := coverageProject(t)
	h, _ := coverageHarness(t, root, nil)
	defer h.close()
	id := h.send("tusk/coverageReport", nil)
	report := h.readResponse(id)["result"].(map[string]interface{})
	if report["path"] != filepath.Join(root, "clover.xml") {
		t.Errorf("unexpected path: %v", report["path"])
	}
	if _, err := time.Parse(time.RFC3339, report["generated"].(string)); err != nil {
		t.Errorf("generated isn't RFC 3339: %v", err)
	}
	files := report["files"].([]interface{})
	if len(files) != 2 {
		t.Fatalf("expected the 2 local files, not the one outside the workspace: %v", files)
	}
	var service, spaced map[string]interface{}
	for _, f := range files {
		file := f.(map[string]interface{})
		switch {
		case strings.HasSuffix(file["uri"].(string), "/app/Service.php"):
			service = file
		case strings.HasSuffix(file["uri"].(string), "/app/Fresh%20Name.php"):
			spaced = file
		}
	}
	if spaced == nil {
		t.Errorf("expected a percent-encoded URI in %v", files)
	}
	if service == nil {
		t.Fatalf("Service.php missing from %v", files)
	}
	if service["coveredLines"] != float64(1) || service["executableLines"] != float64(2) || service["coveredMethods"] != float64(1) || service["methods"] != float64(1) {
		t.Errorf("unexpected totals: %v", service)
	}
}

func TestCoverageChangedNotifiesOnNewReport(t *testing.T) {
	root := coverageProject(t)
	h, _ := coverageHarness(t, root, map[string]interface{}{"coverageNotifications": true})
	defer h.close()
	h.readNotification("tusk/coverageChanged", 3*time.Second) // the first report

	later := time.Now().Add(time.Minute)
	writeReport(t, root, fileCoverageClover)
	if err := os.Chtimes(filepath.Join(root, "clover.xml"), later, later); err != nil {
		t.Fatal(err)
	}
	h.readNotification("tusk/coverageChanged", 5*time.Second) // the new report

	if err := os.Remove(filepath.Join(root, "clover.xml")); err != nil {
		t.Fatal(err)
	}
	h.readNotification("tusk/coverageChanged", 5*time.Second) // the report going away
}

func TestCoverageChangedIsOptIn(t *testing.T) {
	h, _ := coverageHarness(t, coverageProject(t), nil)
	defer h.close()
	id := h.send("tusk/coverageReport", nil) // loads the report
	h.readResponse(id)
	deadline := time.After(500 * time.Millisecond)
	for {
		select {
		case note := <-h.notes:
			if note["method"] == "tusk/coverageChanged" {
				t.Fatal("clients that didn't ask must not get the notification")
			}
		case <-deadline:
			return
		}
	}
}
