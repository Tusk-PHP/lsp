package workspace

import (
	"fmt"
	"time"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/phpdetect"
	"github.com/Tusk-PHP/lsp/internal/stubs"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ResolveBuiltinProfile applies the PHP profile fallback chain:
//  1. composer.json (config.platform.php or require.php)
//  2. locally installed php binary (via phpdetect)
//  3. bundled default
//
// It returns the resolved profile and a source string ("composer", "local", or
// "fallback"). If logf is non-nil, it receives a single human-readable
// provenance message.
func ResolveBuiltinProfile(rootPath, phpBinary string, timeout time.Duration, logf func(string)) (symbols.BuiltinProfile, string) {
	platform := composer.GetPlatform(rootPath)
	profile := symbols.BuiltinProfile{Extensions: platform.Extensions}
	var source string

	switch {
	case platform.PHPVersion != "":
		profile.PHPVersion = platform.PHPVersion
		source = "composer"
	default:
		if local, err := phpdetect.Detect(phpBinary, timeout); err == nil && local.Version != "" {
			profile.PHPVersion = local.Version
			source = "local"
		} else {
			profile.PHPVersion = stubs.DefaultProfile().PHPVersion
			source = "fallback"
		}
	}

	if logf != nil {
		logf(fmt.Sprintf("PHP LSP: PHP %s (source: %s)", profile.PHPVersion, source))
	}
	return profile, source
}
