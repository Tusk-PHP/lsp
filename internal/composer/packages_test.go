package composer

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// writeInstalledJSON creates vendor/composer/installed.json in dir with the
// supplied packages slice encoded as a Composer v2 object ({"packages":[...]}).
func writeInstalledJSON(t *testing.T, dir string, packages interface{}) {
	t.Helper()
	composerDir := filepath.Join(dir, "vendor", "composer")
	if err := os.MkdirAll(composerDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(map[string]interface{}{"packages": packages})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composerDir, "installed.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

// writeInstalledJSONLegacy creates vendor/composer/installed.json in dir with
// the supplied packages slice encoded as a Composer v1 top-level array.
func writeInstalledJSONLegacy(t *testing.T, dir string, packages interface{}) {
	t.Helper()
	composerDir := filepath.Join(dir, "vendor", "composer")
	if err := os.MkdirAll(composerDir, 0755); err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(packages)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(composerDir, "installed.json"), data, 0644); err != nil {
		t.Fatal(err)
	}
}

func fakePackages() []map[string]interface{} {
	return []map[string]interface{}{
		{
			"name":    "monolog/monolog",
			"version": "3.5.0",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Monolog\\": "src/Monolog/",
				},
			},
		},
		{
			"name":    "psr/log",
			"version": "3.0.0",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Psr\\Log\\": "src/",
					"Psr\\":     "src-base/",
				},
			},
		},
	}
}

func TestInstalledPackages(t *testing.T) {
	root := t.TempDir()
	writeInstalledJSON(t, root, fakePackages())

	pkgs := InstalledPackages(root)
	if len(pkgs) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(pkgs))
	}

	byName := make(map[string]Package, len(pkgs))
	for _, p := range pkgs {
		byName[p.Name] = p
	}

	t.Run("monolog name and version", func(t *testing.T) {
		p, ok := byName["monolog/monolog"]
		if !ok {
			t.Fatal("monolog/monolog not found")
		}
		if p.Version != "3.5.0" {
			t.Errorf("expected version 3.5.0, got %q", p.Version)
		}
		if len(p.PSR4Prefixes) != 1 || p.PSR4Prefixes[0] != "Monolog\\" {
			t.Errorf("unexpected PSR4Prefixes: %v", p.PSR4Prefixes)
		}
	})

	t.Run("psr/log name and version", func(t *testing.T) {
		p, ok := byName["psr/log"]
		if !ok {
			t.Fatal("psr/log not found")
		}
		if p.Version != "3.0.0" {
			t.Errorf("expected version 3.0.0, got %q", p.Version)
		}
		if len(p.PSR4Prefixes) != 2 {
			t.Errorf("expected 2 PSR4 prefixes, got %d: %v", len(p.PSR4Prefixes), p.PSR4Prefixes)
		}
	})
}

func TestInstalledPackagesLegacyFormat(t *testing.T) {
	root := t.TempDir()
	writeInstalledJSONLegacy(t, root, fakePackages())

	pkgs := InstalledPackages(root)
	if len(pkgs) != 2 {
		t.Fatalf("legacy format: expected 2 packages, got %d", len(pkgs))
	}
}

func TestInstalledPackagesMissing(t *testing.T) {
	pkgs := InstalledPackages(t.TempDir())
	if pkgs != nil {
		t.Errorf("expected nil for missing installed.json, got %v", pkgs)
	}
}

func TestNewPackageResolver(t *testing.T) {
	root := t.TempDir()
	writeInstalledJSON(t, root, fakePackages())

	resolve := NewPackageResolver(root)

	t.Run("matches monolog FQN", func(t *testing.T) {
		pkg, ver, ok := resolve("Monolog\\Logger")
		if !ok {
			t.Fatal("expected ok=true for Monolog\\Logger")
		}
		if pkg != "monolog/monolog" {
			t.Errorf("expected monolog/monolog, got %q", pkg)
		}
		if ver != "3.5.0" {
			t.Errorf("expected version 3.5.0, got %q", ver)
		}
	})

	t.Run("leading backslash normalised", func(t *testing.T) {
		pkg, _, ok := resolve("\\Monolog\\Logger")
		if !ok {
			t.Fatal("expected ok=true for \\Monolog\\Logger")
		}
		if pkg != "monolog/monolog" {
			t.Errorf("expected monolog/monolog, got %q", pkg)
		}
	})

	t.Run("longest prefix wins", func(t *testing.T) {
		// Psr\Log\ (8 chars) is longer than Psr\ (4 chars).
		pkg, ver, ok := resolve("Psr\\Log\\LoggerInterface")
		if !ok {
			t.Fatal("expected ok=true for Psr\\Log\\LoggerInterface")
		}
		if pkg != "psr/log" {
			t.Errorf("expected psr/log, got %q", pkg)
		}
		if ver != "3.0.0" {
			t.Errorf("expected version 3.0.0, got %q", ver)
		}
		// Even the shorter Psr\ prefix FQN resolves to psr/log.
		pkg2, _, ok2 := resolve("Psr\\Container\\ContainerInterface")
		if !ok2 {
			t.Fatal("expected ok=true for Psr\\Container\\ContainerInterface")
		}
		if pkg2 != "psr/log" {
			t.Errorf("expected psr/log, got %q", pkg2)
		}
	})

	t.Run("unrelated FQN returns ok=false", func(t *testing.T) {
		_, _, ok := resolve("Illuminate\\Support\\Facades\\Log")
		if ok {
			t.Error("expected ok=false for unrelated FQN")
		}
	})

	t.Run("empty FQN returns ok=false", func(t *testing.T) {
		_, _, ok := resolve("")
		if ok {
			t.Error("expected ok=false for empty FQN")
		}
	})
}

func TestNewPackageResolverMissing(t *testing.T) {
	resolve := NewPackageResolver(t.TempDir())

	_, _, ok := resolve("Monolog\\Logger")
	if ok {
		t.Error("expected ok=false when installed.json is missing")
	}
	// Must not panic.
	_, _, _ = resolve("")
}
