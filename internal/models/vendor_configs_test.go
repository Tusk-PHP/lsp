package models

import (
	"os"
	"path/filepath"
	"slices"
	"sync"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/types"
)

func configKeys(r *FrameworkArrayResolver) []string {
	var keys []string
	for _, f := range r.ListConfigFiles() {
		keys = append(keys, f.Key)
	}
	return keys
}

func TestVendorConfigFallsBackOnceTheProjectCopyIsDeleted(t *testing.T) {
	dir, idx := setupTestProject(t)
	vendorConfigDir := filepath.Join(dir, "vendor", "test", "pkg", "config")
	if err := os.MkdirAll(vendorConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendorConfigDir, "app.php"), []byte("<?php\nreturn ['vendor_key' => 'vendor_value'];\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewFrameworkArrayResolver(idx, dir, "laravel")
	if keys := configKeys(r); !slices.Contains(keys, "app") {
		t.Fatalf("config files are listed: %v", keys)
	}
	if fields := r.ParseConfigFile("app"); slices.ContainsFunc(fields, func(f types.ShapeField) bool { return f.Key == "vendor_key" }) {
		t.Fatalf("the project's config/app.php wins over the package's: %v", fields)
	}

	if err := os.Remove(filepath.Join(dir, "config", "app.php")); err != nil {
		t.Fatal(err)
	}
	fields := r.ParseConfigFile("app")
	if !slices.ContainsFunc(fields, func(f types.ShapeField) bool { return f.Key == "vendor_key" }) {
		t.Fatalf("with the project copy gone, the package's config is read: %v", fields)
	}
}

func TestVendorConfigsAreSafeToReadWhileInvalidated(t *testing.T) {
	dir, idx := setupTestProject(t)
	vendorConfigDir := filepath.Join(dir, "vendor", "laravel", "sanctum", "config")
	if err := os.MkdirAll(vendorConfigDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vendorConfigDir, "sanctum.php"), []byte("<?php\nreturn ['guard' => ['web']];\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	r := NewFrameworkArrayResolver(idx, dir, "laravel")
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(2)
		go func() {
			defer wg.Done()
			if keys := configKeys(r); !slices.Contains(keys, "sanctum") {
				t.Errorf("the package's config is listed: %v", keys)
			}
		}()
		go func() {
			defer wg.Done()
			r.InvalidateVendorConfigs()
		}()
	}
	wg.Wait()
}
