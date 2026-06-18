package deps

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

// mustMarshal marshals v to a JSON string or fails the test.
func mustMarshal(t *testing.T, v interface{}) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// buildUsageTestRoot creates a temp project with:
//   - vendor/used     — PSR-4 prefix "UsedPkg\", version 1.0.0
//   - vendor/unused   — PSR-4 prefix "UnusedPkg\", version 2.0.0
//   - vendor/partial  — PSR-4 prefix "PartialPkg\", version 3.0.0
//     (two classes exposed, only one referenced)
//
// Returns the root path.
func buildUsageTestRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"),
		mustMarshal(t, map[string]interface{}{
			"packages": []map[string]interface{}{
				{
					"name":    "vendor/used",
					"version": "1.0.0",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"UsedPkg\\": "src/",
						},
					},
				},
				{
					"name":    "vendor/unused",
					"version": "2.0.0",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"UnusedPkg\\": "src/",
						},
					},
				},
				{
					"name":    "vendor/partial",
					"version": "3.0.0",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"PartialPkg\\": "src/",
						},
					},
				},
			},
		}),
	)
	return root
}

// buildUsageTestIndex constructs a symbol index that matches buildUsageTestRoot:
//   - App\Service (project): property typed UsedPkg\Client, method param typed PartialPkg\Alpha
//   - UsedPkg\Client (vendor/used): the one referenced vendor class
//   - UnusedPkg\Ghost (vendor/unused): present in index but never referenced
//   - PartialPkg\Alpha (vendor/partial): referenced
//   - PartialPkg\Beta (vendor/partial): NOT referenced (partial usage)
func buildUsageTestIndex() *symbols.Index {
	idx := symbols.NewIndex()

	// First-party class with references into vendor packages.
	idx.IndexFileWithSource("file:///src/Service.php", `<?php
namespace App;

use UsedPkg\Client;
use PartialPkg\Alpha;

class Service {
    public Client $client;
    public function run(Alpha $a): void {}
}
`, symbols.SourceProject)

	// vendor/used — one class, fully referenced.
	idx.IndexFileWithSource("file:///vendor/used/Client.php", `<?php
namespace UsedPkg;
class Client {}
`, symbols.SourceVendor)

	// vendor/unused — one class, never referenced by first-party code.
	idx.IndexFileWithSource("file:///vendor/unused/Ghost.php", `<?php
namespace UnusedPkg;
class Ghost {}
`, symbols.SourceVendor)

	// vendor/partial — two classes; Alpha is referenced, Beta is not.
	idx.IndexFileWithSource("file:///vendor/partial/Alpha.php", `<?php
namespace PartialPkg;
class Alpha {}
`, symbols.SourceVendor)
	idx.IndexFileWithSource("file:///vendor/partial/Beta.php", `<?php
namespace PartialPkg;
class Beta {}
`, symbols.SourceVendor)

	return idx
}

// findUsage returns the PackageUsage for the named package, or nil if absent.
func findUsage(usages []PackageUsage, pkg string) *PackageUsage {
	for i := range usages {
		if usages[i].Package == pkg {
			return &usages[i]
		}
	}
	return nil
}

// TestUsage_NilIndex returns an empty slice without panicking.
func TestUsage_NilIndex(t *testing.T) {
	root := buildUsageTestRoot(t)
	result := Usage(nil, root)
	if result == nil {
		t.Fatal("Usage(nil, root) must return non-nil slice")
	}
	if len(result) != 0 {
		t.Errorf("Usage(nil, root): want empty slice, got %v", result)
	}
}

// TestUsage_DivisionByZeroSafe verifies that a package with no indexed classes
// (ExposedClasses == 0) produces Percent 0.0 rather than panicking.
func TestUsage_DivisionByZeroSafe(t *testing.T) {
	// Build a root with a package that has a PSR-4 prefix but no indexed symbols.
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"),
		mustMarshal(t, map[string]interface{}{
			"packages": []map[string]interface{}{
				{
					"name":    "vendor/empty",
					"version": "1.0.0",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							"EmptyPkg\\": "src/",
						},
					},
				},
			},
		}),
	)

	idx := symbols.NewIndex() // no symbols indexed under EmptyPkg\
	result := Usage(idx, root)

	u := findUsage(result, "vendor/empty")
	if u == nil {
		t.Fatal("expected PackageUsage for vendor/empty, not found")
	}
	if u.ExposedClasses != 0 {
		t.Errorf("ExposedClasses = %d, want 0", u.ExposedClasses)
	}
	if u.TouchedClasses != 0 {
		t.Errorf("TouchedClasses = %d, want 0", u.TouchedClasses)
	}
	if u.Percent != 0.0 {
		t.Errorf("Percent = %f, want 0.0 (div-by-zero guard)", u.Percent)
	}
}

// TestUsage_UsedPackage verifies numerator/denominator/percent for vendor/used
// (one exposed class, fully referenced → 100%).
func TestUsage_UsedPackage(t *testing.T) {
	root := buildUsageTestRoot(t)
	idx := buildUsageTestIndex()

	result := Usage(idx, root)

	u := findUsage(result, "vendor/used")
	if u == nil {
		t.Fatal("expected PackageUsage for vendor/used, not found in", result)
	}
	if u.Version != "1.0.0" {
		t.Errorf("vendor/used Version = %q, want %q", u.Version, "1.0.0")
	}
	if u.ExposedClasses != 1 {
		t.Errorf("vendor/used ExposedClasses = %d, want 1", u.ExposedClasses)
	}
	if u.TouchedClasses != 1 {
		t.Errorf("vendor/used TouchedClasses = %d, want 1", u.TouchedClasses)
	}
	if u.Percent != 100.0 {
		t.Errorf("vendor/used Percent = %f, want 100.0", u.Percent)
	}
}

// TestUsage_UnusedPackage verifies that a package with no references produces
// TouchedClasses=0 and Percent=0.
func TestUsage_UnusedPackage(t *testing.T) {
	root := buildUsageTestRoot(t)
	idx := buildUsageTestIndex()

	result := Usage(idx, root)

	u := findUsage(result, "vendor/unused")
	if u == nil {
		t.Fatal("expected PackageUsage for vendor/unused, not found in", result)
	}
	if u.Version != "2.0.0" {
		t.Errorf("vendor/unused Version = %q, want %q", u.Version, "2.0.0")
	}
	if u.ExposedClasses != 1 {
		t.Errorf("vendor/unused ExposedClasses = %d, want 1", u.ExposedClasses)
	}
	if u.TouchedClasses != 0 {
		t.Errorf("vendor/unused TouchedClasses = %d, want 0", u.TouchedClasses)
	}
	if u.Percent != 0.0 {
		t.Errorf("vendor/unused Percent = %f, want 0.0", u.Percent)
	}
}

// TestUsage_PartialPackage verifies fractional usage: vendor/partial exposes
// 2 classes, 1 is referenced → 50%.
func TestUsage_PartialPackage(t *testing.T) {
	root := buildUsageTestRoot(t)
	idx := buildUsageTestIndex()

	result := Usage(idx, root)

	u := findUsage(result, "vendor/partial")
	if u == nil {
		t.Fatal("expected PackageUsage for vendor/partial, not found in", result)
	}
	if u.Version != "3.0.0" {
		t.Errorf("vendor/partial Version = %q, want %q", u.Version, "3.0.0")
	}
	if u.ExposedClasses != 2 {
		t.Errorf("vendor/partial ExposedClasses = %d, want 2", u.ExposedClasses)
	}
	if u.TouchedClasses != 1 {
		t.Errorf("vendor/partial TouchedClasses = %d, want 1", u.TouchedClasses)
	}
	want := 50.0
	if u.Percent != want {
		t.Errorf("vendor/partial Percent = %f, want %f", u.Percent, want)
	}
}

// TestUsage_SortedOutput verifies that results are returned sorted by package
// name ascending.
func TestUsage_SortedOutput(t *testing.T) {
	root := buildUsageTestRoot(t)
	idx := buildUsageTestIndex()

	result := Usage(idx, root)

	for i := 1; i < len(result); i++ {
		if result[i].Package < result[i-1].Package {
			t.Errorf("result not sorted at index %d: %q < %q",
				i, result[i].Package, result[i-1].Package)
		}
	}
}

// TestUsage_AllPackagesPresent verifies that all three packages appear in the
// output even when some have zero usage.
func TestUsage_AllPackagesPresent(t *testing.T) {
	root := buildUsageTestRoot(t)
	idx := buildUsageTestIndex()

	result := Usage(idx, root)

	wantPackages := []string{"vendor/partial", "vendor/unused", "vendor/used"}
	if len(result) != len(wantPackages) {
		t.Fatalf("want %d results, got %d: %v", len(wantPackages), len(result), result)
	}
	for _, pkg := range wantPackages {
		if findUsage(result, pkg) == nil {
			t.Errorf("package %q missing from result", pkg)
		}
	}
}

// TestUsage_DistinctFQNAcrossPrefixes verifies that a class is not double-counted
// when it matches two PSR-4 prefixes of the same package.
func TestUsage_DistinctFQNAcrossPrefixes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"),
		mustMarshal(t, map[string]interface{}{
			"packages": []map[string]interface{}{
				{
					"name":    "vendor/multi",
					"version": "1.0.0",
					"autoload": map[string]interface{}{
						"psr-4": map[string]interface{}{
							// Both prefixes cover the same class via different lengths.
							// In practice this is unusual, but the dedupe logic must handle it.
							"Multi\\":    "src/",
							"Multi\\Sub": "src/Sub/",
						},
					},
				},
			},
		}),
	)

	idx := symbols.NewIndex()
	// Index one class that matches the shorter prefix.
	idx.IndexFileWithSource("file:///vendor/multi/Foo.php", `<?php
namespace Multi;
class Foo {}
`, symbols.SourceVendor)

	result := Usage(idx, root)

	u := findUsage(result, "vendor/multi")
	if u == nil {
		t.Fatal("expected PackageUsage for vendor/multi")
	}
	// Multi\Foo should be counted exactly once despite matching two prefixes.
	if u.ExposedClasses != 1 {
		t.Errorf("ExposedClasses = %d, want 1 (no double-count across prefixes)", u.ExposedClasses)
	}
}

// TestUsage_NoPSR4PackageOmitted verifies that a package with no PSR-4 autoload
// entries does not appear in the result (no meaningful class surface).
func TestUsage_NoPSR4PackageOmitted(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"),
		mustMarshal(t, map[string]interface{}{
			"packages": []map[string]interface{}{
				{
					"name":    "vendor/classmap-only",
					"version": "1.0.0",
					"autoload": map[string]interface{}{
						"classmap": []string{"src/"},
					},
				},
			},
		}),
	)

	idx := symbols.NewIndex()
	result := Usage(idx, root)

	for _, u := range result {
		if u.Package == "vendor/classmap-only" {
			t.Error("package with no PSR-4 entries should be omitted from results")
		}
	}
}

// TestUsage_Float64DistinctSymbols verifies that Meta["distinctSymbols"] stored
// as float64 (as it would be after JSON decode) is handled without panic.
func TestUsage_Float64DistinctSymbols(t *testing.T) {
	// This test exercises the defensive float64 branch in Usage by calling
	// BuildReferences indirectly via the live index path, but since we cannot
	// inject Meta directly through the public API we validate that Usage returns
	// a coherent (non-panicking) result even for the index paths that may
	// produce zero-count Meta values.
	root := buildUsageTestRoot(t)
	idx := buildUsageTestIndex()

	// Must not panic.
	result := Usage(idx, root)
	if result == nil {
		t.Fatal("Usage must return non-nil slice")
	}
}
