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

func writeLock(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "composer.lock"), []byte(content), 0644); err != nil {
		t.Fatalf("write composer.lock: %v", err)
	}
}
