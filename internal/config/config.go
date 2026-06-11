package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// RuleSeverityValue is the value stored per rule code in DiagnosticRules.
// It accepts both the legacy boolean form and a string severity name:
//
//	true / "warning"  → use the rule's default severity
//	false / "off"     → disable the rule
//	"hint"            → override to hint
//	"info"            → override to info
//	"warning"         → override to warning (explicit)
//	"error"           → override to error
type RuleSeverityValue struct {
	enabled  bool
	override checks.Severity // 0 means "use default"
	hasValue bool            // false → missing (treated as default)
}

// MarshalJSON emits the compact form used for writing config back.
func (v RuleSeverityValue) MarshalJSON() ([]byte, error) {
	if !v.hasValue {
		return []byte("true"), nil
	}
	if !v.enabled {
		return []byte(`"off"`), nil
	}
	switch v.override {
	case checks.SeverityError:
		return []byte(`"error"`), nil
	case checks.SeverityWarning:
		return []byte(`"warning"`), nil
	case checks.SeverityInfo:
		return []byte(`"info"`), nil
	case checks.SeverityHint:
		return []byte(`"hint"`), nil
	}
	return []byte("true"), nil
}

// UnmarshalJSON accepts both legacy bool and string severity forms.
func (v *RuleSeverityValue) UnmarshalJSON(data []byte) error {
	v.hasValue = true
	// Boolean form: true / false
	var b bool
	if err := json.Unmarshal(data, &b); err == nil {
		v.enabled = b
		return nil
	}
	// String form: "off" | "hint" | "info" | "warning" | "error"
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	switch s {
	case "off", "":
		v.enabled = false
	case "hint":
		v.enabled = true
		v.override = checks.SeverityHint
	case "info":
		v.enabled = true
		v.override = checks.SeverityInfo
	case "warning":
		v.enabled = true
		v.override = checks.SeverityWarning
	case "error":
		v.enabled = true
		v.override = checks.SeverityError
	default:
		// Unknown string → treat as enabled with default severity
		v.enabled = true
	}
	return nil
}

// InlayHintsConfig holds the configuration for inlay hint display.
type InlayHintsConfig struct {
	Enabled             bool `json:"enabled"`
	VariableTypes       bool `json:"variableTypes"`
	ForeachTypes        bool `json:"foreachTypes"`
	ClosureReturnTypes  bool `json:"closureReturnTypes"`
	ReturnTypes         bool `json:"returnTypes"`
	ParameterNames      bool `json:"parameterNames"`
	SuppressSingleParam bool `json:"suppressSingleParam"`
	SuppressNameMatch   bool `json:"suppressNameMatch"`
}

// ComposerHoverConfig controls hover behavior on composer.json files.
//
// FetchRemote / FetchVCS / CacheTTLHours / RequestTimeoutMs are stubbed for
// v1 (lockfile-only). Their zero values mean "off" — keeping them in the
// struct now ensures user .tusk-php.json files written against v1 stay
// valid when v2/v3 wire those paths.
type ComposerHoverConfig struct {
	Enable           bool `json:"enable"`
	FetchRemote      bool `json:"fetchRemote,omitempty"`
	FetchVCS         bool `json:"fetchVCS,omitempty"`
	CacheTTLHours    int  `json:"cacheTTLHours,omitempty"`
	RequestTimeoutMs int  `json:"requestTimeoutMs,omitempty"`
}

// ComposerConfig groups composer.json-aware features.
type ComposerConfig struct {
	Hover            ComposerHoverConfig `json:"hover"`
	OpenOnDefinition bool                `json:"openOnDefinition,omitempty"`
}

type DBConfig struct {
	Enabled           *bool    `json:"enabled,omitempty"`
	Mode              string   `json:"mode,omitempty"`
	Source            string   `json:"source,omitempty"`
	AllowDataSampling *bool    `json:"allowDataSampling,omitempty"`
	ConnectTimeoutMs  int      `json:"connectTimeoutMs,omitempty"`
	RedactColumns     []string `json:"redactColumns,omitempty"`
}

type AIConfig struct {
	MCP          *bool    `json:"mcp,omitempty"`
	WriteTools   string   `json:"writeTools,omitempty"`
	AllowedRoots []string `json:"allowedRoots,omitempty"`
	DenyPaths    []string `json:"denyPaths,omitempty"`
}

// Config holds the LSP server configuration.
type Config struct {
	PHPVersion string `json:"phpVersion"`
	// Extensions lists additional PHP extensions (e.g. ["json", "mbstring"])
	// to include when resolving the builtin profile. When set in the file or
	// via initializationOptions these are merged with the Composer-detected
	// extension set; an empty slice is treated as "not set" so it does not
	// suppress composer-derived extensions.
	Extensions []string `json:"extensions,omitempty"`
	// phpVersionExplicitlySet is true when PHPVersion was loaded from a file
	// or set via initializationOptions, distinguishing it from the default
	// "8.5" so the resolution chain can skip the default correctly.
	phpVersionExplicitlySet   bool
	Framework                 string                       `json:"framework"`
	ComposerPath              string                       `json:"composerPath"`
	IncludePaths              []string                     `json:"includePaths"`
	ExcludePaths              []string                     `json:"excludePaths"`
	ContainerAware            bool                         `json:"containerAware"`
	DiagnosticsEnabled        bool                         `json:"diagnosticsEnabled"`
	PHPBinary                 string                       `json:"phpBinary,omitempty"`
	PHPDetectTimeoutMs        int                          `json:"phpDetectTimeoutMs,omitempty"`
	PHPStanEnabled            *bool                        `json:"phpstanEnabled,omitempty"`
	PHPStanPath               string                       `json:"phpstanPath,omitempty"`
	PHPStanLevel              string                       `json:"phpstanLevel,omitempty"`
	PHPStanConfig             string                       `json:"phpstanConfig,omitempty"`
	PintEnabled               *bool                        `json:"pintEnabled,omitempty"`
	PintPath                  string                       `json:"pintPath,omitempty"`
	PintConfig                string                       `json:"pintConfig,omitempty"`
	DatabaseEnabled           *bool                        `json:"databaseEnabled,omitempty"`
	DatabaseSource            string                       `json:"databaseSource,omitempty"`
	DiagnosticRules           map[string]RuleSeverityValue `json:"diagnosticRules,omitempty"`
	MaxIndexFiles             int                          `json:"maxIndexFiles"`
	StubsPath                 string                       `json:"stubsPath"`
	LogLevel                  string                       `json:"logLevel"`
	LogFile                   string                       `json:"logFile"`
	InlayHints                InlayHintsConfig             `json:"inlayHints"`
	PHPManualLocale           string                       `json:"php_manual_locale,omitempty"`
	PHPManualOpenOnDefinition bool                         `json:"php_manual_open_on_definition,omitempty"`
	Composer                  ComposerConfig               `json:"composer,omitempty"`
	DB                        DBConfig                     `json:"db,omitempty"`
	AI                        AIConfig                     `json:"ai,omitempty"`
}

// SetDiagnosticRulesJSON parses a JSON object of rule code → severity value
// (bool or string) and stores it as DiagnosticRules. Returns any parse error.
func (c *Config) SetDiagnosticRulesJSON(raw string) error {
	var m map[string]RuleSeverityValue
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return err
	}
	c.DiagnosticRules = m
	return nil
}

// IsPHPVersionExplicitlySet reports whether PHPVersion was supplied by a
// .tusk-php.json file or by client initializationOptions. When false the
// value is the compiled-in default ("8.5") and must not shadow a Composer
// or local-PHP detection result.
func (c *Config) IsPHPVersionExplicitlySet() bool {
	return c.phpVersionExplicitlySet
}

// IsRuleEnabled returns whether a diagnostic rule is enabled.
// Rules default to enabled if not explicitly configured.
func (c *Config) IsRuleEnabled(code string) bool {
	if c.DiagnosticRules == nil {
		return true
	}
	v, ok := c.DiagnosticRules[code]
	if !ok {
		return true
	}
	return v.enabled
}

// RuleSeverity returns the effective severity for a rule code.
// If the config has an explicit severity override it is returned; otherwise
// defaultSeverity is returned. If the rule is disabled, defaultSeverity is
// still returned — callers should check IsRuleEnabled first.
func (c *Config) RuleSeverity(code string, defaultSeverity checks.Severity) checks.Severity {
	if c.DiagnosticRules == nil {
		return defaultSeverity
	}
	v, ok := c.DiagnosticRules[code]
	if !ok || v.override == 0 {
		return defaultSeverity
	}
	return v.override
}

func DefaultConfig() *Config {
	return &Config{
		PHPVersion:         "8.5",
		Framework:          "auto",
		IncludePaths:       []string{"src", "app", "lib"},
		ExcludePaths:       []string{"vendor", "node_modules", ".git", "storage", "var/cache"},
		ContainerAware:     true,
		DiagnosticsEnabled: true,
		MaxIndexFiles:      10000,
		LogLevel:           "info",
		LogFile:            "",
		InlayHints: InlayHintsConfig{
			Enabled:             true,
			VariableTypes:       true,
			ForeachTypes:        true,
			ClosureReturnTypes:  true,
			ReturnTypes:         true,
			ParameterNames:      true,
			SuppressSingleParam: true,
			SuppressNameMatch:   true,
		},
		Composer: ComposerConfig{
			Hover: ComposerHoverConfig{Enable: true, CacheTTLHours: 6, RequestTimeoutMs: 3000},
		},
		DB: DBConfig{
			Mode: "schema_only",
		},
		AI: AIConfig{
			WriteTools: "disabled",
			DenyPaths:  []string{".env", "vendor", "storage"},
		},
	}
}

func LoadFromFile(path string) (*Config, error) {
	cfg := DefaultConfig()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(data, cfg); err != nil {
		return nil, err
	}
	// Detect which keys were explicitly present in the file so the profile
	// resolution chain can distinguish "file-set" from "default" values.
	var keys map[string]json.RawMessage
	if json.Unmarshal(data, &keys) == nil {
		if _, ok := keys["phpVersion"]; ok {
			cfg.phpVersionExplicitlySet = true
		}
	}
	return cfg, nil
}

// MergeClientOptions applies client-provided initializationOptions over
// the current config. Only non-zero values from the client override.
func (c *Config) MergeClientOptions(opts *protocol.InitializationOptions) {
	if opts.PHPVersion != "" {
		c.PHPVersion = opts.PHPVersion
		c.phpVersionExplicitlySet = true
	}
	if len(opts.Extensions) > 0 {
		c.Extensions = opts.Extensions
	}
	if opts.Framework != "" {
		c.Framework = opts.Framework
	}
	if opts.ContainerAware != nil {
		c.ContainerAware = *opts.ContainerAware
	}
	if opts.DiagnosticsEnabled != nil {
		c.DiagnosticsEnabled = *opts.DiagnosticsEnabled
	}
	if opts.PHPStanEnabled != nil {
		c.PHPStanEnabled = opts.PHPStanEnabled
	}
	if opts.PHPStanPath != "" {
		c.PHPStanPath = opts.PHPStanPath
	}
	if opts.PHPStanLevel != "" {
		c.PHPStanLevel = opts.PHPStanLevel
	}
	if opts.PHPStanConfig != "" {
		c.PHPStanConfig = opts.PHPStanConfig
	}
	if opts.PintEnabled != nil {
		c.PintEnabled = opts.PintEnabled
	}
	if opts.PintPath != "" {
		c.PintPath = opts.PintPath
	}
	if opts.PintConfig != "" {
		c.PintConfig = opts.PintConfig
	}
	if opts.DatabaseEnabled != nil {
		c.DatabaseEnabled = opts.DatabaseEnabled
	}
	if opts.MaxIndexFiles != nil {
		c.MaxIndexFiles = *opts.MaxIndexFiles
	}
	if len(opts.ExcludePaths) > 0 {
		c.ExcludePaths = opts.ExcludePaths
	}
	if opts.InlayHints != nil {
		ih := opts.InlayHints
		if ih.Enabled != nil {
			c.InlayHints.Enabled = *ih.Enabled
		}
		if ih.VariableTypes != nil {
			c.InlayHints.VariableTypes = *ih.VariableTypes
		}
		if ih.ForeachTypes != nil {
			c.InlayHints.ForeachTypes = *ih.ForeachTypes
		}
		if ih.ClosureReturnTypes != nil {
			c.InlayHints.ClosureReturnTypes = *ih.ClosureReturnTypes
		}
		if ih.ReturnTypes != nil {
			c.InlayHints.ReturnTypes = *ih.ReturnTypes
		}
		if ih.ParameterNames != nil {
			c.InlayHints.ParameterNames = *ih.ParameterNames
		}
		if ih.SuppressSingleParam != nil {
			c.InlayHints.SuppressSingleParam = *ih.SuppressSingleParam
		}
		if ih.SuppressNameMatch != nil {
			c.InlayHints.SuppressNameMatch = *ih.SuppressNameMatch
		}
	}
	if opts.PHPManualLocale != "" {
		c.PHPManualLocale = opts.PHPManualLocale
	}
	if opts.PHPManualOpenOnDefinition != nil {
		c.PHPManualOpenOnDefinition = *opts.PHPManualOpenOnDefinition
	}
	if opts.Composer != nil {
		if opts.Composer.OpenOnDefinition != nil {
			c.Composer.OpenOnDefinition = *opts.Composer.OpenOnDefinition
		}
		if h := opts.Composer.Hover; h != nil {
			if h.Enable != nil {
				c.Composer.Hover.Enable = *h.Enable
			}
			if h.FetchRemote != nil {
				c.Composer.Hover.FetchRemote = *h.FetchRemote
			}
			if h.FetchVCS != nil {
				c.Composer.Hover.FetchVCS = *h.FetchVCS
			}
			if h.CacheTTLHours != nil {
				c.Composer.Hover.CacheTTLHours = *h.CacheTTLHours
			}
			if h.RequestTimeoutMs != nil {
				c.Composer.Hover.RequestTimeoutMs = *h.RequestTimeoutMs
			}
		}
	}
}

// IsDatabaseEnabled returns whether database introspection is enabled (default: true).
func (c *Config) IsDatabaseEnabled() bool {
	if c.DatabaseEnabled == nil {
		if c.DB.Enabled == nil {
			return true
		}
		return *c.DB.Enabled
	}
	return *c.DatabaseEnabled
}

func DetectFramework(rootPath string) string {
	if fileExists(filepath.Join(rootPath, "artisan")) &&
		fileExists(filepath.Join(rootPath, "app", "Providers", "AppServiceProvider.php")) {
		return "laravel"
	}
	if fileExists(filepath.Join(rootPath, "bin", "console")) &&
		(dirExists(filepath.Join(rootPath, "config", "packages")) ||
			fileExists(filepath.Join(rootPath, "symfony.lock"))) {
		return "symfony"
	}
	composerPath := filepath.Join(rootPath, "composer.json")
	if data, err := os.ReadFile(composerPath); err == nil {
		var composer struct {
			Require map[string]string `json:"require"`
		}
		if json.Unmarshal(data, &composer) == nil {
			if _, ok := composer.Require["laravel/framework"]; ok {
				return "laravel"
			}
			if _, ok := composer.Require["symfony/framework-bundle"]; ok {
				return "symfony"
			}
		}
	}
	return "none"
}

// DatabaseSourceMode returns the configured schema source mode:
// "auto" (default), "live", or "migrations".
func (c *Config) DatabaseSourceMode() string {
	source := c.DatabaseSource
	if source == "" {
		source = c.DB.Source
	}
	switch source {
	case "", "auto":
		return "auto"
	case "live":
		return "live"
	case "migrations":
		return "migrations"
	default:
		return "auto"
	}
}

func (c *Config) DatabaseMode() string {
	switch c.DB.Mode {
	case "", "schema_only":
		return "schema_only"
	case "sample_safe":
		return "sample_safe"
	case "full_query":
		return "full_query"
	default:
		return "schema_only"
	}
}

// AllowsDataSampling reports whether opt-in redacted row sampling is permitted.
// Defaults to false — schema-only is the safe default.
func (c *Config) AllowsDataSampling() bool {
	return c.DB.AllowDataSampling != nil && *c.DB.AllowDataSampling
}

// DatabaseConnectTimeout returns the DB connect/query timeout. Defaults to 5s;
// values below 1s are clamped up to avoid pathological near-zero timeouts.
func (c *Config) DatabaseConnectTimeout() time.Duration {
	if c.DB.ConnectTimeoutMs <= 0 {
		return 5 * time.Second
	}
	d := time.Duration(c.DB.ConnectTimeoutMs) * time.Millisecond
	if d < time.Second {
		return time.Second
	}
	return d
}

func (c *Config) AIRoots() []string {
	return append([]string(nil), c.AI.AllowedRoots...)
}

func (c *Config) AIDenyPaths() []string {
	return append([]string(nil), c.AI.DenyPaths...)
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
