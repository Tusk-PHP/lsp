package lockfile

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadMissingReturnsNil(t *testing.T) {
	l, err := Load(t.TempDir())
	if err != nil {
		t.Fatalf("expected no error for missing composer.lock, got %v", err)
	}
	if l != nil {
		t.Fatalf("expected nil lockfile, got %+v", l)
	}
	if _, ok := l.Find("anything"); ok {
		t.Errorf("nil lockfile Find should return ok=false")
	}
}

func TestLoadParsesPackages(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, `{
		"packages": [
			{
				"name": "laravel/framework",
				"version": "v10.48.4",
				"description": "The Laravel Framework.",
				"license": ["MIT"],
				"homepage": "https://laravel.com",
				"require": {"php": "^8.1", "ext-json": "*"},
				"source": {"url": "https://github.com/laravel/framework.git", "type": "git"}
			}
		],
		"packages-dev": [
			{
				"name": "phpunit/phpunit",
				"version": "10.5.0",
				"require": {"php": ">=8.1"}
			}
		]
	}`)

	l, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	framework, ok := l.Find("laravel/framework")
	if !ok {
		t.Fatal("expected laravel/framework in lockfile")
	}
	if framework.Version != "v10.48.4" {
		t.Errorf("version = %q", framework.Version)
	}
	if framework.PHPRequire != "^8.1" {
		t.Errorf("php require = %q, want ^8.1", framework.PHPRequire)
	}
	if framework.SourceURL != "https://github.com/laravel/framework.git" {
		t.Errorf("source url = %q", framework.SourceURL)
	}
	if len(framework.License) != 1 || framework.License[0] != "MIT" {
		t.Errorf("license = %v", framework.License)
	}

	phpunit, ok := l.Find("phpunit/phpunit")
	if !ok {
		t.Fatal("expected phpunit/phpunit (dev) in lockfile")
	}
	if phpunit.PHPRequire != ">=8.1" {
		t.Errorf("phpunit php require = %q", phpunit.PHPRequire)
	}
}

func TestLockedPackageRequireField(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, `{
		"packages": [
			{
				"name": "laravel/framework",
				"version": "v10.48.4",
				"require": {
					"php":      "^8.1",
					"ext-json": "*",
					"psr/log":  "^3.0"
				}
			}
		],
		"packages-dev": []
	}`)

	l, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	pkg, ok := l.Find("laravel/framework")
	if !ok {
		t.Fatal("laravel/framework not found")
	}

	// PHPRequire must still be set correctly.
	if pkg.PHPRequire != "^8.1" {
		t.Errorf("PHPRequire = %q, want ^8.1", pkg.PHPRequire)
	}

	// Require must carry all entries including package edges and platform reqs.
	wantRequire := map[string]string{
		"php":      "^8.1",
		"ext-json": "*",
		"psr/log":  "^3.0",
	}
	if len(pkg.Require) != len(wantRequire) {
		t.Fatalf("Require len = %d, want %d; got %v", len(pkg.Require), len(wantRequire), pkg.Require)
	}
	for k, want := range wantRequire {
		if got := pkg.Require[k]; got != want {
			t.Errorf("Require[%q] = %q, want %q", k, got, want)
		}
	}
}

func TestAllReturnsSortedPackagesFromBothSections(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, `{
		"packages": [
			{
				"name": "psr/log",
				"version": "3.0.0",
				"require": {"php": ">=8.0"}
			},
			{
				"name": "laravel/framework",
				"version": "v10.48.4",
				"require": {"php": "^8.1", "psr/log": "^3.0"}
			}
		],
		"packages-dev": [
			{
				"name": "phpunit/phpunit",
				"version": "10.5.0",
				"require": {"php": ">=8.1"}
			}
		]
	}`)

	l, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	all := l.All()

	if len(all) != 3 {
		t.Fatalf("All() len = %d, want 3", len(all))
	}

	// Must be sorted by Name ascending.
	wantNames := []string{"laravel/framework", "phpunit/phpunit", "psr/log"}
	for i, pkg := range all {
		if pkg.Name != wantNames[i] {
			t.Errorf("All()[%d].Name = %q, want %q", i, pkg.Name, wantNames[i])
		}
	}

	// Spot-check that Require is populated on All() results.
	if all[0].Require["psr/log"] != "^3.0" {
		t.Errorf("laravel/framework Require[psr/log] = %q, want ^3.0", all[0].Require["psr/log"])
	}
}

func TestAllNilSafety(t *testing.T) {
	var l *Lockfile
	got := l.All()
	if got == nil {
		t.Error("All() on nil *Lockfile should return empty non-nil slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("All() on nil *Lockfile should return empty slice, got len=%d", len(got))
	}
}

func TestAllEmptyLockfile(t *testing.T) {
	root := t.TempDir()
	writeLock(t, root, `{"packages": [], "packages-dev": []}`)

	l, err := Load(root)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	got := l.All()
	if got == nil {
		t.Error("All() on empty lockfile should return empty non-nil slice, got nil")
	}
	if len(got) != 0 {
		t.Errorf("All() on empty lockfile should return empty slice, got len=%d", len(got))
	}
}

func writeLock(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(content), 0644); err != nil {
		t.Fatalf("write composer.lock: %v", err)
	}
}
