package config

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

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
	Enabled       *bool    `json:"enabled,omitempty"`
	Mode          string   `json:"mode,omitempty"`
	Source        string   `json:"source,omitempty"`
	RedactColumns []string `json:"redactColumns,omitempty"`
}

type AIConfig struct {
	MCP          *bool    `json:"mcp,omitempty"`
	WriteTools   string   `json:"writeTools,omitempty"`
	AllowedRoots []string `json:"allowedRoots,omitempty"`
	DenyPaths    []string `json:"denyPaths,omitempty"`
}

// Config holds the LSP server configuration.
type Config struct {
	PHPVersion                string           `json:"phpVersion"`
	Framework                 string           `json:"framework"`
	ComposerPath              string           `json:"composerPath"`
	IncludePaths              []string         `json:"includePaths"`
	ExcludePaths              []string         `json:"excludePaths"`
	ContainerAware            bool             `json:"containerAware"`
	DiagnosticsEnabled        bool             `json:"diagnosticsEnabled"`
	PHPBinary                 string           `json:"phpBinary,omitempty"`
	PHPDetectTimeoutMs        int              `json:"phpDetectTimeoutMs,omitempty"`
	PHPStanEnabled            *bool            `json:"phpstanEnabled,omitempty"`
	PHPStanPath               string           `json:"phpstanPath,omitempty"`
	PHPStanLevel              string           `json:"phpstanLevel,omitempty"`
	PHPStanConfig             string           `json:"phpstanConfig,omitempty"`
	PintEnabled               *bool            `json:"pintEnabled,omitempty"`
	PintPath                  string           `json:"pintPath,omitempty"`
	PintConfig                string           `json:"pintConfig,omitempty"`
	DatabaseEnabled           *bool            `json:"databaseEnabled,omitempty"`
	DatabaseSource            string           `json:"databaseSource,omitempty"`
	DiagnosticRules           map[string]bool  `json:"diagnosticRules,omitempty"`
	MaxIndexFiles             int              `json:"maxIndexFiles"`
	StubsPath                 string           `json:"stubsPath"`
	LogLevel                  string           `json:"logLevel"`
	LogFile                   string           `json:"logFile"`
	InlayHints                InlayHintsConfig `json:"inlayHints"`
	PHPManualLocale           string           `json:"php_manual_locale,omitempty"`
	PHPManualOpenOnDefinition bool             `json:"php_manual_open_on_definition,omitempty"`
	Composer                  ComposerConfig   `json:"composer,omitempty"`
	DB                        DBConfig         `json:"db,omitempty"`
	AI                        AIConfig         `json:"ai,omitempty"`
}

// IsRuleEnabled returns whether a diagnostic rule is enabled.
// Rules default to enabled if not explicitly configured.
func (c *Config) IsRuleEnabled(code string) bool {
	if c.DiagnosticRules == nil {
		return true
	}
	enabled, ok := c.DiagnosticRules[code]
	if !ok {
		return true
	}
	return enabled
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
	return cfg, nil
}

// MergeClientOptions applies client-provided initializationOptions over
// the current config. Only non-zero values from the client override.
func (c *Config) MergeClientOptions(opts *protocol.InitializationOptions) {
	if opts.PHPVersion != "" {
		c.PHPVersion = opts.PHPVersion
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
