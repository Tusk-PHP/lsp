package composer

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// writeBundlesFile creates config/bundles.php under root with the supplied PHP
// source content.
func writeBundlesFile(t *testing.T, root, content string) {
	t.Helper()
	dir := filepath.Join(root, "config")
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "bundles.php"), []byte(content), 0644); err != nil {
		t.Fatalf("write bundles.php: %v", err)
	}
}

// buildSymfonyDiscoveryFixture creates a temp project with:
//   - config/bundles.php referencing two vendor bundle classes
//   - vendor/composer/installed.json mapping their PSR-4 prefixes
//
// Returns the root path.
func buildSymfonyDiscoveryFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeBundlesFile(t, root, `<?php

return [
    Symfony\Bundle\FrameworkBundle\FrameworkBundle::class => ['all' => true],
    Symfony\Bundle\TwigBundle\TwigBundle::class => ['all' => true],
    App\MyBundle::class => ['all' => true],
];
`)

	writeInstalledJSON(t, root, []map[string]interface{}{
		{
			"name":         "symfony/framework-bundle",
			"version":      "6.4.0",
			"install-path": "../../vendor/symfony/framework-bundle",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Symfony\\Bundle\\FrameworkBundle\\": ".",
				},
			},
		},
		{
			"name":         "symfony/twig-bundle",
			"version":      "6.4.0",
			"install-path": "../../vendor/symfony/twig-bundle",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Symfony\\Bundle\\TwigBundle\\": ".",
				},
			},
		},
	})

	return root
}

// TestSymfonyBundlePackages_ResolvesBundleClasses verifies that both vendor
// bundle classes are resolved to their owning packages. App\ (first-party,
// unresolvable) must be silently skipped.
func TestSymfonyBundlePackages_ResolvesBundleClasses(t *testing.T) {
	root := buildSymfonyDiscoveryFixture(t)

	got := SymfonyBundlePackages(root)
	want := []string{"symfony/framework-bundle", "symfony/twig-bundle"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SymfonyBundlePackages() = %v, want %v", got, want)
	}
}

// TestSymfonyBundlePackages_MissingFile returns nil gracefully when
// config/bundles.php does not exist.
func TestSymfonyBundlePackages_MissingFile(t *testing.T) {
	got := SymfonyBundlePackages(t.TempDir())
	if got != nil {
		t.Fatalf("expected nil for missing bundles.php, got %v", got)
	}
}

// TestSymfonyBundlePackages_EmptyFile returns nil when the file contains no
// ::class keys.
func TestSymfonyBundlePackages_EmptyFile(t *testing.T) {
	root := t.TempDir()
	writeBundlesFile(t, root, `<?php return [];`)

	got := SymfonyBundlePackages(root)
	if got != nil {
		t.Fatalf("expected nil for bundles.php with no ::class keys, got %v", got)
	}
}

// TestSymfonyBundlePackages_UnresolvableFQN returns nil when the installed.json
// is absent so that no FQN can be resolved to a package.
func TestSymfonyBundlePackages_UnresolvableFQN(t *testing.T) {
	root := t.TempDir()
	writeBundlesFile(t, root, `<?php
return [
    Unknown\Bundle\SomeBundle::class => ['all' => true],
];
`)
	// No installed.json — resolver always returns ok=false.

	got := SymfonyBundlePackages(root)
	if got != nil {
		t.Fatalf("expected nil when FQN is unresolvable, got %v", got)
	}
}

// TestSymfonyBundlePackages_LeadingBackslash verifies that FQNs with a leading
// backslash (e.g. \Symfony\Bundle\...) are resolved correctly.
func TestSymfonyBundlePackages_LeadingBackslash(t *testing.T) {
	root := t.TempDir()

	writeBundlesFile(t, root, `<?php
return [
    \Symfony\Bundle\FrameworkBundle\FrameworkBundle::class => ['all' => true],
];
`)

	writeInstalledJSON(t, root, []map[string]interface{}{
		{
			"name":         "symfony/framework-bundle",
			"version":      "6.4.0",
			"install-path": "../../vendor/symfony/framework-bundle",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Symfony\\Bundle\\FrameworkBundle\\": ".",
				},
			},
		},
	})

	got := SymfonyBundlePackages(root)
	want := []string{"symfony/framework-bundle"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SymfonyBundlePackages() = %v, want %v", got, want)
	}
}

// TestSymfonyBundlePackages_Deduplication verifies that the same package
// appearing via multiple bundle classes is returned only once.
func TestSymfonyBundlePackages_Deduplication(t *testing.T) {
	root := t.TempDir()

	writeBundlesFile(t, root, `<?php
return [
    Symfony\Bundle\FrameworkBundle\FrameworkBundle::class   => ['all' => true],
    Symfony\Bundle\FrameworkBundle\SubBundle\Extra::class   => ['dev' => true],
];
`)

	writeInstalledJSON(t, root, []map[string]interface{}{
		{
			"name":         "symfony/framework-bundle",
			"version":      "6.4.0",
			"install-path": "../../vendor/symfony/framework-bundle",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Symfony\\Bundle\\FrameworkBundle\\": ".",
				},
			},
		},
	})

	got := SymfonyBundlePackages(root)
	want := []string{"symfony/framework-bundle"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("SymfonyBundlePackages() = %v, want %v", got, want)
	}
}
