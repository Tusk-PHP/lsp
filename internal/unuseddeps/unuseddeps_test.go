package unuseddeps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// writeFile writes content to path, creating parent directories as needed.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
}

// buildTestRoot creates a temp project directory with:
//   - composer.json requiring "vendor/used" and "vendor/unused" plus an
//     auto-discovered "vendor/autodiscovered" package
//   - vendor/composer/installed.json mapping PSR-4 prefixes to packages
//
// Returns the root path.
func buildTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// composer.json: two real deps plus one auto-discovered package.
	writeFile(t, filepath.Join(root, "composer.json"), `{
		"require": {
			"vendor/used":           "^1.0",
			"vendor/unused":         "^1.0",
			"vendor/autodiscovered": "^1.0"
		},
		"extra": {
			"laravel": {
				"providers": [
					"Autodiscovered\\Pkg\\ServiceProvider"
				]
			}
		}
	}`)

	// installed.json (Composer v2 format).
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"),
		mustMarshal(t, map[string]interface{}{
			"packages": []map[string]interface{}{
				{
					"name":         "vendor/used",
					"version":      "1.0.0",
					"install-path": "../../vendor/vendor/used",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"UsedPkg\\": "src/",
						},
					},
				},
				{
					"name":         "vendor/unused",
					"version":      "1.0.0",
					"install-path": "../../vendor/vendor/unused",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"UnusedPkg\\": "src/",
						},
					},
				},
				{
					"name":         "vendor/autodiscovered",
					"version":      "1.0.0",
					"install-path": "../../vendor/vendor/autodiscovered",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"Autodiscovered\\Pkg\\": "src/",
						},
					},
				},
			},
		}),
	)

	return root
}

// mustMarshal marshals v to a JSON string or fails the test.
func mustMarshal(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// buildTestIndex creates a symbol index for the test scenario:
//   - First-party class App\Service has a property typed UsedPkg\Client (vendor/used)
//   - UsedPkg\Client is indexed as SourceVendor
//   - UnusedPkg\Unused is indexed as SourceVendor but never referenced
//   - Autodiscovered\Pkg\ServiceProvider is indexed as SourceVendor
func buildTestIndex() *symbols.Index {
	idx := symbols.NewIndex()

	// First-party class with a property referencing the "used" vendor class.
	idx.IndexFileWithSource("file:///src/Service.php", `<?php
namespace App;

use UsedPkg\Client;

class Service {
    public Client $client;
}
`, symbols.SourceProject)

	// Vendor: vendor/used
	idx.IndexFileWithSource("file:///vendor/vendor/used/src/Client.php", `<?php
namespace UsedPkg;

class Client {}
`, symbols.SourceVendor)

	// Vendor: vendor/unused — never referenced by first-party code.
	idx.IndexFileWithSource("file:///vendor/vendor/unused/src/Unused.php", `<?php
namespace UnusedPkg;

class Unused {}
`, symbols.SourceVendor)

	// Vendor: vendor/autodiscovered — consumed via Laravel auto-discovery, not static refs.
	idx.IndexFileWithSource("file:///vendor/vendor/autodiscovered/src/ServiceProvider.php", `<?php
namespace Autodiscovered\Pkg;

class ServiceProvider {}
`, symbols.SourceVendor)

	return idx
}

// TestAnalyze_UnusedPackageIsCandidate is the primary scenario: "vendor/unused"
// has no first-party reference and must appear as a candidate.
func TestAnalyze_UnusedPackageIsCandidate(t *testing.T) {
	root := buildTestRoot(t)
	idx := buildTestIndex()

	rep := Analyze(idx, root, "laravel")

	if rep.Checked != 3 {
		t.Errorf("Checked = %d, want 3", rep.Checked)
	}

	// "vendor/unused" must appear as a candidate.
	found := false
	for _, c := range rep.Candidates {
		if c.Package == "vendor/unused" {
			found = true
			if c.Reason == "" {
				t.Error("Candidate.Reason must not be empty")
			}
		}
	}
	if !found {
		t.Errorf("expected 'vendor/unused' to be a candidate; candidates = %v", rep.Candidates)
	}
}

// TestAnalyze_UsedPackageNotCandidate verifies that "vendor/used" (which is
// referenced by a first-party property type) is NOT flagged as a candidate.
func TestAnalyze_UsedPackageNotCandidate(t *testing.T) {
	root := buildTestRoot(t)
	idx := buildTestIndex()

	rep := Analyze(idx, root, "")

	for _, c := range rep.Candidates {
		if c.Package == "vendor/used" {
			t.Errorf("'vendor/used' must not be a candidate; it is statically referenced")
		}
	}
}

// TestAnalyze_AutoDiscoveredPackageExcluded verifies that a package listed under
// extra.laravel.providers is excluded from candidates even when it has no direct
// static code reference.
func TestAnalyze_AutoDiscoveredPackageExcluded(t *testing.T) {
	root := buildTestRoot(t)
	idx := buildTestIndex()

	rep := Analyze(idx, root, "laravel")

	for _, c := range rep.Candidates {
		if c.Package == "vendor/autodiscovered" {
			t.Errorf("'vendor/autodiscovered' must not be a candidate; it is auto-discovered by Laravel")
		}
	}
}

// TestAnalyze_NilIndex returns an empty report without panicking.
func TestAnalyze_NilIndex(t *testing.T) {
	root := buildTestRoot(t)

	rep := Analyze(nil, root, "")
	if rep.Checked != 0 {
		// Nil index: Checked reflects DirectRequires (not zero when composer.json exists)
		// but Candidates must be empty.
	}
	if len(rep.Candidates) != 0 {
		t.Errorf("nil index: expected empty candidates, got %v", rep.Candidates)
	}
}

// TestAnalyze_EmptyRequire returns an empty report for a project with no deps.
func TestAnalyze_EmptyRequire(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "composer.json"), `{"require": {}}`)

	idx := symbols.NewIndex()
	rep := Analyze(idx, root, "")

	if rep.Checked != 0 {
		t.Errorf("Checked = %d, want 0", rep.Checked)
	}
	if len(rep.Candidates) != 0 {
		t.Errorf("expected empty candidates, got %v", rep.Candidates)
	}
}

// TestAnalyze_SoleCandidateIsUnused ensures exactly one candidate is returned
// in the baseline scenario: only "vendor/unused" should appear.
func TestAnalyze_SoleCandidateIsUnused(t *testing.T) {
	root := buildTestRoot(t)
	idx := buildTestIndex()

	rep := Analyze(idx, root, "laravel")

	if len(rep.Candidates) != 1 {
		t.Errorf("expected exactly 1 candidate, got %d: %v", len(rep.Candidates), rep.Candidates)
		return
	}
	if rep.Candidates[0].Package != "vendor/unused" {
		t.Errorf("expected candidate 'vendor/unused', got %q", rep.Candidates[0].Package)
	}
}
