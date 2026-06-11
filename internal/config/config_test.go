package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PHPVersion != "8.5" {
		t.Errorf("PHPVersion = %q", cfg.PHPVersion)
	}
	if cfg.Framework != "auto" {
		t.Errorf("Framework = %q", cfg.Framework)
	}
	if !cfg.DiagnosticsEnabled {
		t.Error("DiagnosticsEnabled should default to true")
	}
	if !cfg.ContainerAware {
		t.Error("ContainerAware should default to true")
	}
	if cfg.MaxIndexFiles != 10000 {
		t.Errorf("MaxIndexFiles = %d", cfg.MaxIndexFiles)
	}
}

func TestLoadFromFile(t *testing.T) {
	t.Run("valid JSON", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".tusk-php.json")
		os.WriteFile(path, []byte(`{"phpVersion":"8.3","framework":"laravel"}`), 0644)
		cfg, err := LoadFromFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PHPVersion != "8.3" {
			t.Errorf("PHPVersion = %q", cfg.PHPVersion)
		}
		if cfg.Framework != "laravel" {
			t.Errorf("Framework = %q", cfg.Framework)
		}
		// Defaults preserved for unset fields
		if !cfg.DiagnosticsEnabled {
			t.Error("DiagnosticsEnabled should still be true")
		}
	})

	t.Run("missing file returns defaults", func(t *testing.T) {
		cfg, err := LoadFromFile("/nonexistent/.tusk-php.json")
		if err != nil {
			t.Fatal(err)
		}
		if cfg.PHPVersion != "8.5" {
			t.Errorf("expected default PHPVersion, got %q", cfg.PHPVersion)
		}
	})

	t.Run("invalid JSON returns error", func(t *testing.T) {
		dir := t.TempDir()
		path := filepath.Join(dir, ".tusk-php.json")
		os.WriteFile(path, []byte(`{invalid`), 0644)
		_, err := LoadFromFile(path)
		if err == nil {
			t.Error("expected error for invalid JSON")
		}
	})
}

func TestMergeClientOptions(t *testing.T) {
	cfg := DefaultConfig()

	boolTrue := true
	boolFalse := false
	maxFiles := 5000

	opts := &protocol.InitializationOptions{
		PHPVersion:         "8.2",
		Framework:          "symfony",
		ContainerAware:     &boolFalse,
		DiagnosticsEnabled: &boolTrue,
		PHPStanEnabled:     &boolTrue,
		PHPStanPath:        "/usr/bin/phpstan",
		PHPStanLevel:       "9",
		PHPStanConfig:      "phpstan.neon",
		PintEnabled:        &boolFalse,
		PintPath:           "/usr/bin/pint",
		PintConfig:         "pint.json",
		DatabaseEnabled:    &boolTrue,
		MaxIndexFiles:      &maxFiles,
		ExcludePaths:       []string{"vendor"},
	}
	cfg.MergeClientOptions(opts)

	if cfg.PHPVersion != "8.2" {
		t.Errorf("PHPVersion = %q", cfg.PHPVersion)
	}
	if cfg.Framework != "symfony" {
		t.Errorf("Framework = %q", cfg.Framework)
	}
	if cfg.ContainerAware {
		t.Error("ContainerAware should be false")
	}
	if cfg.MaxIndexFiles != 5000 {
		t.Errorf("MaxIndexFiles = %d", cfg.MaxIndexFiles)
	}

	t.Run("zero values don't override", func(t *testing.T) {
		cfg2 := DefaultConfig()
		cfg2.MergeClientOptions(&protocol.InitializationOptions{})
		if cfg2.PHPVersion != "8.5" {
			t.Error("empty string shouldn't override PHPVersion")
		}
	})
}

func makeRuleSeverityEnabled() RuleSeverityValue  { return RuleSeverityValue{enabled: true, hasValue: true} }
func makeRuleSeverityDisabled() RuleSeverityValue { return RuleSeverityValue{enabled: false, hasValue: true} }

func TestIsRuleEnabled(t *testing.T) {
	t.Run("nil map returns true", func(t *testing.T) {
		cfg := &Config{}
		if !cfg.IsRuleEnabled("unused-import") {
			t.Error("nil map should return true")
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]RuleSeverityValue{"unused-import": makeRuleSeverityEnabled()}}
		if !cfg.IsRuleEnabled("unused-import") {
			t.Error("explicit true should return true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]RuleSeverityValue{"unused-import": makeRuleSeverityDisabled()}}
		if cfg.IsRuleEnabled("unused-import") {
			t.Error("explicit false should return false")
		}
	})

	t.Run("missing key returns true", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]RuleSeverityValue{"other-rule": makeRuleSeverityDisabled()}}
		if !cfg.IsRuleEnabled("unused-import") {
			t.Error("missing key should default to true")
		}
	})
}

func TestRuleSeverity(t *testing.T) {
	t.Run("nil map returns default", func(t *testing.T) {
		cfg := &Config{}
		if got := cfg.RuleSeverity("unused-import", checks.SeverityWarning); got != checks.SeverityWarning {
			t.Errorf("expected SeverityWarning, got %d", got)
		}
	})

	t.Run("missing key returns default", func(t *testing.T) {
		cfg := &Config{DiagnosticRules: map[string]RuleSeverityValue{}}
		if got := cfg.RuleSeverity("unused-import", checks.SeverityHint); got != checks.SeverityHint {
			t.Errorf("expected SeverityHint, got %d", got)
		}
	})

	t.Run("override error from JSON string", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		_ = json.Unmarshal([]byte(`{"unknown-class":"error"}`), &m)
		cfg := &Config{DiagnosticRules: m}
		if got := cfg.RuleSeverity("unknown-class", checks.SeverityWarning); got != checks.SeverityError {
			t.Errorf("expected SeverityError, got %d", got)
		}
	})

	t.Run("override hint from JSON string", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		_ = json.Unmarshal([]byte(`{"unused-import":"hint"}`), &m)
		cfg := &Config{DiagnosticRules: m}
		if got := cfg.RuleSeverity("unused-import", checks.SeverityHint); got != checks.SeverityHint {
			t.Errorf("expected SeverityHint, got %d", got)
		}
	})
}

func TestRuleSeverityValueJSON(t *testing.T) {
	t.Run("legacy true JSON is enabled", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		if err := json.Unmarshal([]byte(`{"unused-import":true}`), &m); err != nil {
			t.Fatal(err)
		}
		v := m["unused-import"]
		if !v.enabled {
			t.Error("true should be enabled")
		}
	})

	t.Run("legacy false JSON is disabled", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		if err := json.Unmarshal([]byte(`{"unused-import":false}`), &m); err != nil {
			t.Fatal(err)
		}
		v := m["unused-import"]
		if v.enabled {
			t.Error("false should be disabled")
		}
	})

	t.Run("string off is disabled", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		if err := json.Unmarshal([]byte(`{"unused-import":"off"}`), &m); err != nil {
			t.Fatal(err)
		}
		v := m["unused-import"]
		if v.enabled {
			t.Error("off should be disabled")
		}
	})

	t.Run("string warning is enabled with warning override", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		if err := json.Unmarshal([]byte(`{"unused-import":"warning"}`), &m); err != nil {
			t.Fatal(err)
		}
		v := m["unused-import"]
		if !v.enabled {
			t.Error("warning should be enabled")
		}
		if v.override != checks.SeverityWarning {
			t.Errorf("expected SeverityWarning override, got %d", v.override)
		}
	})

	t.Run("string error enables with error override", func(t *testing.T) {
		var m map[string]RuleSeverityValue
		if err := json.Unmarshal([]byte(`{"unknown-class":"error"}`), &m); err != nil {
			t.Fatal(err)
		}
		v := m["unknown-class"]
		if !v.enabled {
			t.Error("error should be enabled")
		}
		if v.override != checks.SeverityError {
			t.Errorf("expected SeverityError override, got %d", v.override)
		}
	})
}

func TestIsDatabaseEnabled(t *testing.T) {
	t.Run("nil returns true", func(t *testing.T) {
		cfg := &Config{}
		if !cfg.IsDatabaseEnabled() {
			t.Error("nil should return true")
		}
	})

	t.Run("explicit true", func(t *testing.T) {
		b := true
		cfg := &Config{DatabaseEnabled: &b}
		if !cfg.IsDatabaseEnabled() {
			t.Error("should be true")
		}
	})

	t.Run("explicit false", func(t *testing.T) {
		b := false
		cfg := &Config{DatabaseEnabled: &b}
		if cfg.IsDatabaseEnabled() {
			t.Error("should be false")
		}
	})
}

func TestDatabaseSourceMode(t *testing.T) {
	t.Run("default auto", func(t *testing.T) {
		cfg := &Config{}
		if got := cfg.DatabaseSourceMode(); got != "auto" {
			t.Fatalf("DatabaseSourceMode() = %q", got)
		}
	})

	t.Run("live", func(t *testing.T) {
		cfg := &Config{DatabaseSource: "live"}
		if got := cfg.DatabaseSourceMode(); got != "live" {
			t.Fatalf("DatabaseSourceMode() = %q", got)
		}
	})

	t.Run("migrations", func(t *testing.T) {
		cfg := &Config{DatabaseSource: "migrations"}
		if got := cfg.DatabaseSourceMode(); got != "migrations" {
			t.Fatalf("DatabaseSourceMode() = %q", got)
		}
	})
}

func TestDefaultConfigAISafety(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.DB.Mode != "schema_only" {
		t.Fatalf("expected schema_only DB mode, got %q", cfg.DB.Mode)
	}
	if cfg.AI.WriteTools != "disabled" {
		t.Fatalf("expected disabled write tools, got %q", cfg.AI.WriteTools)
	}
	if len(cfg.AI.DenyPaths) == 0 {
		t.Fatal("expected non-empty deny paths")
	}
}

func TestDefaultConfigPHPManual(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.PHPManualLocale != "" {
		t.Errorf("PHPManualLocale should default to empty, got %q", cfg.PHPManualLocale)
	}
	if cfg.PHPManualOpenOnDefinition {
		t.Error("PHPManualOpenOnDefinition should default to false")
	}
}

func TestLoadFromFilePHPManualLocale(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".tusk-php.json")
	os.WriteFile(path, []byte(`{"php_manual_locale":"es"}`), 0644)
	cfg, err := LoadFromFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.PHPManualLocale != "es" {
		t.Errorf("PHPManualLocale = %q, want %q", cfg.PHPManualLocale, "es")
	}
}

func TestMergeClientOptionsPHPManual(t *testing.T) {
	t.Run("locale overrides file value", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.PHPManualLocale = "de"
		opts := &protocol.InitializationOptions{PHPManualLocale: "fr"}
		cfg.MergeClientOptions(opts)
		if cfg.PHPManualLocale != "fr" {
			t.Errorf("PHPManualLocale = %q, want %q", cfg.PHPManualLocale, "fr")
		}
	})

	t.Run("empty locale does not override", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.PHPManualLocale = "ja"
		opts := &protocol.InitializationOptions{}
		cfg.MergeClientOptions(opts)
		if cfg.PHPManualLocale != "ja" {
			t.Errorf("PHPManualLocale = %q, want %q", cfg.PHPManualLocale, "ja")
		}
	})

	t.Run("open on definition set to true", func(t *testing.T) {
		cfg := DefaultConfig()
		trueVal := true
		opts := &protocol.InitializationOptions{PHPManualOpenOnDefinition: &trueVal}
		cfg.MergeClientOptions(opts)
		if !cfg.PHPManualOpenOnDefinition {
			t.Error("PHPManualOpenOnDefinition should be true after merge")
		}
	})

	t.Run("open on definition nil does not override", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.PHPManualOpenOnDefinition = true
		opts := &protocol.InitializationOptions{}
		cfg.MergeClientOptions(opts)
		if !cfg.PHPManualOpenOnDefinition {
			t.Error("PHPManualOpenOnDefinition should remain true when opts is nil")
		}
	})
}

func TestDetectFramework(t *testing.T) {
	t.Run("laravel with artisan", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "artisan"), []byte("#!/usr/bin/env php"), 0644)
		os.MkdirAll(filepath.Join(dir, "app", "Providers"), 0755)
		os.WriteFile(filepath.Join(dir, "app", "Providers", "AppServiceProvider.php"), []byte("<?php"), 0644)
		if f := DetectFramework(dir); f != "laravel" {
			t.Errorf("expected laravel, got %q", f)
		}
	})

	t.Run("symfony with bin/console", func(t *testing.T) {
		dir := t.TempDir()
		os.MkdirAll(filepath.Join(dir, "bin"), 0755)
		os.WriteFile(filepath.Join(dir, "bin", "console"), []byte("#!/usr/bin/env php"), 0644)
		os.MkdirAll(filepath.Join(dir, "config", "packages"), 0755)
		if f := DetectFramework(dir); f != "symfony" {
			t.Errorf("expected symfony, got %q", f)
		}
	})

	t.Run("laravel from composer.json", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "composer.json"), []byte(`{"require":{"laravel/framework":"^11.0"}}`), 0644)
		if f := DetectFramework(dir); f != "laravel" {
			t.Errorf("expected laravel, got %q", f)
		}
	})

	t.Run("no framework", func(t *testing.T) {
		dir := t.TempDir()
		if f := DetectFramework(dir); f != "none" {
			t.Errorf("expected none, got %q", f)
		}
	})
}
