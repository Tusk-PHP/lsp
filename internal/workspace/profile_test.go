package workspace

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
)

// configWithExplicitPHP returns a *config.Config with PHPVersion explicitly set
// (as if loaded from a .tusk-php.json file containing {"phpVersion": version}).
func configWithExplicitPHP(t *testing.T, version string) *config.Config {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".tusk-php.json")
	if err := os.WriteFile(path, []byte(`{"phpVersion":"`+version+`"}`), 0644); err != nil {
		t.Fatalf("configWithExplicitPHP: %v", err)
	}
	cfg, err := config.LoadFromFile(path)
	if err != nil {
		t.Fatalf("configWithExplicitPHP LoadFromFile: %v", err)
	}
	if !cfg.IsPHPVersionExplicitlySet() {
		t.Fatal("configWithExplicitPHP: IsPHPVersionExplicitlySet() is false after loading file with phpVersion key")
	}
	return cfg
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeFile(%q): %v", path, err)
	}
}

// TestResolveBuiltinProfileChainComposerWins verifies that a composer.json
// config.platform.php version takes precedence over an explicit .tusk-php.json
// phpVersion.
func TestResolveBuiltinProfileChainComposerWins(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"config":{"platform":{"php":"8.1.0"}}}`)

	cfgExplicit := configWithExplicitPHP(t, "8.3")

	profile, source := ResolveBuiltinProfileWithConfig(dir, "", time.Second, cfgExplicit, nil)
	if source != "composer" {
		t.Errorf("expected source=composer, got %q", source)
	}
	if profile.PHPVersion != "8.1" {
		t.Errorf("expected PHPVersion=8.1 from composer, got %q", profile.PHPVersion)
	}
}

// TestResolveBuiltinProfileChainConfigSlots verifies that when composer has no
// PHP version constraint the explicitly-set .tusk-php.json value wins over
// phpdetect/fallback.
func TestResolveBuiltinProfileChainConfigSlots(t *testing.T) {
	dir := t.TempDir()
	// composer.json without php constraint — no platform.php, no require.php
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"monolog/monolog":"^3.0"}}`)

	cfgExplicit := configWithExplicitPHP(t, "8.2")
	profile, source := ResolveBuiltinProfileWithConfig(dir, "nonexistent-php-binary-xyz", time.Millisecond, cfgExplicit, nil)
	if source != "config" {
		t.Errorf("expected source=config, got %q", source)
	}
	if profile.PHPVersion != "8.2" {
		t.Errorf("expected PHPVersion=8.2 from config, got %q", profile.PHPVersion)
	}
}

// TestResolveBuiltinProfileChainDefaultDoesNotSlot verifies that the default
// PHPVersion ("8.5") in DefaultConfig does NOT participate in the chain as a
// "config" source — it must not shadow phpdetect or fallback logic.
func TestResolveBuiltinProfileChainDefaultDoesNotSlot(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"monolog/monolog":"^3.0"}}`)

	// DefaultConfig() leaves phpVersionExplicitlySet=false.
	cfg := config.DefaultConfig()
	if cfg.IsPHPVersionExplicitlySet() {
		t.Fatal("DefaultConfig() must not have IsPHPVersionExplicitlySet()=true")
	}

	profile, source := ResolveBuiltinProfileWithConfig(dir, "nonexistent-php-binary-xyz", time.Millisecond, cfg, nil)
	// Source must be "fallback" (phpdetect failed), not "config".
	if source == "config" {
		t.Errorf("DefaultConfig PHPVersion must NOT participate as source=config, got source=%q version=%s", source, profile.PHPVersion)
	}
}

// TestResolveBuiltinProfileExtensionsMerged verifies that config.Extensions
// are merged with composer-derived extensions, with deduplication.
func TestResolveBuiltinProfileExtensionsMerged(t *testing.T) {
	dir := t.TempDir()
	// composer declares ext-json; config adds mbstring and duplicates json.
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"require":{"ext-json":"*"},"config":{"platform":{"php":"8.1"}}}`)

	cfg := config.DefaultConfig()
	cfg.Extensions = []string{"mbstring", "json"} // json duplicates composer-derived

	profile, source := ResolveBuiltinProfileWithConfig(dir, "", time.Second, cfg, nil)
	if source != "composer" {
		t.Errorf("expected source=composer, got %q", source)
	}

	has := func(ext string) bool {
		for _, e := range profile.Extensions {
			if e == ext {
				return true
			}
		}
		return false
	}
	if !has("json") {
		t.Errorf("expected json in merged extensions; got %v", profile.Extensions)
	}
	if !has("mbstring") {
		t.Errorf("expected mbstring in merged extensions; got %v", profile.Extensions)
	}

	// Dedup: json must appear exactly once.
	count := 0
	for _, e := range profile.Extensions {
		if e == "json" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("json extension appeared %d times in %v, want 1", count, profile.Extensions)
	}
}

// TestResolveBuiltinProfileNilConfig falls back gracefully when cfg is nil,
// preserving the original two-step chain (composer → phpdetect → fallback).
func TestResolveBuiltinProfileNilConfig(t *testing.T) {
	dir := t.TempDir()
	writeTestFile(t, filepath.Join(dir, "composer.json"), `{"config":{"platform":{"php":"8.0"}}}`)

	profile, source := ResolveBuiltinProfileWithConfig(dir, "", time.Second, nil, nil)
	if source != "composer" {
		t.Errorf("expected source=composer, got %q", source)
	}
	if profile.PHPVersion != "8.0" {
		t.Errorf("expected PHPVersion=8.0, got %q", profile.PHPVersion)
	}
}
