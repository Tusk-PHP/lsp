package config

// ClientOptions are the settings an editor sends, in the LSP's
// initializationOptions or in workspace/didChangeConfiguration, applied over
// the configuration file by WithClientOptions. Pointers and empty values
// mean "not set", so a setting the client leaves out keeps the file's value.
type ClientOptions struct {
	PHPVersion                string                      `json:"phpVersion,omitempty"`
	Extensions                []string                    `json:"extensions,omitempty"`
	Framework                 string                      `json:"framework,omitempty"`
	DiagnosticsEnabled        *bool                       `json:"diagnosticsEnabled,omitempty"`
	PHPStanEnabled            *bool                       `json:"phpstanEnabled,omitempty"`
	PHPStanPath               string                      `json:"phpstanPath,omitempty"`
	PHPStanLevel              string                      `json:"phpstanLevel,omitempty"`
	PHPStanConfig             string                      `json:"phpstanConfig,omitempty"`
	CoverageReportPath        string                      `json:"coverageReportPath,omitempty"`
	PintEnabled               *bool                       `json:"pintEnabled,omitempty"`
	PintPath                  string                      `json:"pintPath,omitempty"`
	PintConfig                string                      `json:"pintConfig,omitempty"`
	DatabaseEnabled           *bool                       `json:"databaseEnabled,omitempty"`
	MaxIndexFiles             *int                        `json:"maxIndexFiles,omitempty"`
	ExcludePaths              []string                    `json:"excludePaths,omitempty"`
	InlayHints                *InlayHintsClientOptions    `json:"inlayHints,omitempty"`
	AutoImport                *AutoImportClientOptions    `json:"autoImport,omitempty"`
	PHPManualLocale           string                      `json:"phpManualLocale,omitempty"`
	PHPManualOpenOnDefinition *bool                       `json:"phpManualOpenOnDefinition,omitempty"`
	Composer                  *ComposerClientOptions      `json:"composer,omitempty"`
	Introspection             *IntrospectionClientOptions `json:"introspection,omitempty"`
}

// ComposerClientOptions mirrors ComposerConfig but uses pointers so
// "unset" can be distinguished from "explicitly false" when merging over a
// file-based default.
type ComposerClientOptions struct {
	Hover            *ComposerHoverClientOptions `json:"hover,omitempty"`
	OpenOnDefinition *bool                       `json:"openOnDefinition,omitempty"`
}

// ComposerHoverClientOptions are the client-tunable hover knobs. v1 only
// reads Enable; the remaining fields are accepted so v2/v3 client configs
// don't need a server bump.
type ComposerHoverClientOptions struct {
	Enable           *bool `json:"enable,omitempty"`
	FetchRemote      *bool `json:"fetchRemote,omitempty"`
	FetchVCS         *bool `json:"fetchVCS,omitempty"`
	CacheTTLHours    *int  `json:"cacheTTLHours,omitempty"`
	RequestTimeoutMs *int  `json:"requestTimeoutMs,omitempty"`
}

// IntrospectionClientOptions carries the verbosity setting for the document
// introspection feature. The Verbosity field uses a pointer so "unset" is
// distinguishable from an explicit value when merging over a file-based
// default.
type IntrospectionClientOptions struct {
	Verbosity *string `json:"verbosity,omitempty"`
}

// AutoImportClientOptions carries the client's auto-import settings.
type AutoImportClientOptions struct {
	// AliasStyle is "suffix", "prefix" or "fullyQualified".
	AliasStyle string `json:"aliasStyle,omitempty"`
}

// InlayHintsClientOptions carries per-flag inlay hint toggles from the client.
// All fields use *bool so an unset flag is distinguishable from an explicit false.
type InlayHintsClientOptions struct {
	Enabled             *bool `json:"enabled,omitempty"`
	VariableTypes       *bool `json:"variableTypes,omitempty"`
	ForeachTypes        *bool `json:"foreachTypes,omitempty"`
	ClosureReturnTypes  *bool `json:"closureReturnTypes,omitempty"`
	ReturnTypes         *bool `json:"returnTypes,omitempty"`
	ParameterNames      *bool `json:"parameterNames,omitempty"`
	SuppressSingleParam *bool `json:"suppressSingleParam,omitempty"`
	SuppressNameMatch   *bool `json:"suppressNameMatch,omitempty"`
}
