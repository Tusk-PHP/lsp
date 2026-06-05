package composer

import (
	"reflect"
	"testing"
)

// buildLaravelDiscoveryFixture creates a temp directory with:
//   - composer.json referencing extra.laravel.providers from "vendor/acmepkg"
//   - vendor/composer/installed.json so the PSR-4 resolver can map the FQN
func buildLaravelDiscoveryFixture(t *testing.T) (root string) {
	t.Helper()
	root = t.TempDir()

	writeComposerJSON(t, root, `{
		"require": {
			"vendor/acmepkg": "^1.0"
		},
		"extra": {
			"laravel": {
				"providers": [
					"Acme\\AcmePkg\\AcmeServiceProvider"
				],
				"aliases": {
					"Acme": "Acme\\AcmePkg\\Facades\\AcmeFacade"
				}
			}
		}
	}`)

	writeInstalledJSON(t, root, []map[string]interface{}{
		{
			"name":         "vendor/acmepkg",
			"version":      "1.0.0",
			"install-path": "../../vendor/acmepkg",
			"autoload": map[string]interface{}{
				"psr-4": map[string]interface{}{
					"Acme\\AcmePkg\\": "src/",
				},
			},
		},
	})

	return root
}

// TestLaravelAutoDiscoveredPackages_ProviderAndAlias verifies that both the
// providers list and the aliases map values are resolved to their owning package.
func TestLaravelAutoDiscoveredPackages_ProviderAndAlias(t *testing.T) {
	root := buildLaravelDiscoveryFixture(t)

	got := LaravelAutoDiscoveredPackages(root)
	want := []string{"vendor/acmepkg"}

	if !reflect.DeepEqual(got, want) {
		t.Fatalf("LaravelAutoDiscoveredPackages() = %v, want %v", got, want)
	}
}

// TestLaravelAutoDiscoveredPackages_MissingComposerJSON returns nil gracefully.
func TestLaravelAutoDiscoveredPackages_MissingComposerJSON(t *testing.T) {
	got := LaravelAutoDiscoveredPackages(t.TempDir())
	if got != nil {
		t.Fatalf("expected nil for missing composer.json, got %v", got)
	}
}

// TestLaravelAutoDiscoveredPackages_NoExtraSection returns nil when the composer.json
// has no extra.laravel section.
func TestLaravelAutoDiscoveredPackages_NoExtraSection(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{"require": {"vendor/foo": "^1.0"}}`)

	got := LaravelAutoDiscoveredPackages(root)
	if got != nil {
		t.Fatalf("expected nil for composer.json without extra.laravel, got %v", got)
	}
}

// TestLaravelAutoDiscoveredPackages_UnresolvableFQN skips FQNs that the
// package resolver cannot map (no installed.json present).
func TestLaravelAutoDiscoveredPackages_UnresolvableFQN(t *testing.T) {
	root := t.TempDir()
	writeComposerJSON(t, root, `{
		"extra": {
			"laravel": {
				"providers": ["Unknown\\Package\\ServiceProvider"]
			}
		}
	}`)
	// No installed.json — resolver always returns ok=false.

	got := LaravelAutoDiscoveredPackages(root)
	if got != nil {
		t.Fatalf("expected nil when FQN is unresolvable, got %v", got)
	}
}
