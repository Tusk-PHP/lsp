package workspace

import (
	"fmt"
	"time"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/phpdetect"
	"github.com/Tusk-PHP/lsp/internal/stubs"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ResolveBuiltinProfile applies the PHP profile fallback chain for the version:
//  1. composer.json (config.platform.php or require.php) — highest precedence
//  2. .tusk-php.json / initializationOptions phpVersion (only when explicitly set)
//  3. locally installed php binary (via phpdetect)
//  4. bundled default
//
// Extensions from all sources that do fire are unioned: Composer-derived
// extensions are always included; config extensions (when present) are
// merged on top.
//
// It returns the resolved profile and a source string ("composer",
// "config", "local", or "fallback"). If logf is non-nil, it receives a
// single human-readable provenance message.
func ResolveBuiltinProfile(rootPath, phpBinary string, timeout time.Duration, logf func(string)) (symbols.BuiltinProfile, string) {
	return ResolveBuiltinProfileWithConfig(rootPath, phpBinary, timeout, nil, logf)
}

// ResolveBuiltinProfileWithConfig is like ResolveBuiltinProfile but also
// accepts the loaded workspace config so the .tusk-php.json phpVersion and
// extensions fields can participate in the chain.
func ResolveBuiltinProfileWithConfig(rootPath, phpBinary string, timeout time.Duration, cfg *config.Config, logf func(string)) (symbols.BuiltinProfile, string) {
	platform := composer.GetPlatform(rootPath)
	profile := symbols.BuiltinProfile{Extensions: platform.Extensions}
	var source string

	switch {
	case platform.PHPVersion != "":
		// Composer wins: config.platform.php or require.php is the most
		// authoritative constraint because it reflects the actual deployment
		// target declared in the project manifest.
		profile.PHPVersion = platform.PHPVersion
		source = "composer"

	case cfg != nil && cfg.IsPHPVersionExplicitlySet():
		// .tusk-php.json / initializationOptions: the developer explicitly
		// declared a version override. Only participates when composer has no
		// opinion (no config.platform.php and no require.php constraint).
		profile.PHPVersion = cfg.PHPVersion
		source = "config"

	default:
		if local, err := phpdetect.Detect(phpBinary, timeout); err == nil && local.Version != "" {
			profile.PHPVersion = local.Version
			source = "local"
		} else {
			profile.PHPVersion = stubs.DefaultProfile().PHPVersion
			source = "fallback"
		}
	}

	// Merge config extensions on top of composer-derived extensions.
	// Config extensions are additive: they can enable additional stubs but
	// cannot remove extensions that composer declared.
	if cfg != nil && len(cfg.Extensions) > 0 {
		profile.Extensions = mergeExtensions(profile.Extensions, cfg.Extensions)
	}

	if logf != nil {
		logf(fmt.Sprintf("PHP LSP: PHP %s (source: %s)", profile.PHPVersion, source))
	}
	return profile, source
}

// mergeExtensions returns the union of base and extra extension lists,
// deduplicating by lowercased name.
func mergeExtensions(base, extra []string) []string {
	seen := make(map[string]struct{}, len(base)+len(extra))
	result := make([]string, 0, len(base)+len(extra))
	for _, ext := range base {
		lc := toLower(ext)
		if _, ok := seen[lc]; !ok {
			seen[lc] = struct{}{}
			result = append(result, lc)
		}
	}
	for _, ext := range extra {
		lc := toLower(ext)
		if _, ok := seen[lc]; !ok {
			seen[lc] = struct{}{}
			result = append(result, lc)
		}
	}
	return result
}

func toLower(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + ('a' - 'A')
		}
	}
	return string(b)
}
