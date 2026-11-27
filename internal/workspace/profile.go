package workspace

import (
	"fmt"
	"time"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/config"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/phpdetect"
	"github.com/Tusk-PHP/lsp/internal/stubs"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func ResolveBuiltinProfileWithConfig(rootPath, phpBinary string, timeout time.Duration, cfg *config.Config, logf func(string)) (symbols.BuiltinProfile, string) {
	platform := composer.GetPlatform(rootPath)
	profile := symbols.BuiltinProfile{Extensions: platform.Extensions}
	var source string
	configVersion, configGrammar := explicitConfigVersion(cfg, logf)

	switch {
	case platform.PHP != nil:
		// Composer wins: config.platform.php or require.php is the most
		// authoritative constraint because it reflects the actual deployment
		// target declared in the project manifest.
		profile.PHPVersion = platform.PHP.Oldest.String()
		profile.Grammars = platform.PHP.Grammars
		source = "composer"

	case configVersion != "":
		// .tusk-php.json / initializationOptions: the developer explicitly
		// declared a version override. Only participates when composer has no
		// opinion (no config.platform.php and no require.php constraint).
		profile.PHPVersion = configVersion
		profile.Grammars = phpparser.OnlyGrammar(configGrammar)
		source = "config"

	default:
		if local, grammar, ok := localVersion(phpBinary, timeout, logf); ok {
			profile.PHPVersion = local
			profile.Grammars = phpparser.OnlyGrammar(grammar)
			source = "local"
		} else {
			profile.PHPVersion = stubs.DefaultProfile().PHPVersion
			profile.Grammars = phpparser.AnyGrammar()
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
		logf(fmt.Sprintf("PHP LSP: PHP %s, grammar %s (source: %s)", profile.PHPVersion, profile.Grammars, source))
	}
	return profile, source
}

func explicitConfigVersion(cfg *config.Config, logf func(string)) (string, token.Version) {
	if cfg == nil || !cfg.IsPHPVersionExplicitlySet() {
		return "", 0
	}
	grammar, err := token.ParseVersion(cfg.PHPVersion)
	if err != nil {
		if logf != nil {
			logf(fmt.Sprintf("PHP LSP: ignoring phpVersion from the configuration: %v", err))
		}
		return "", 0
	}
	return cfg.PHPVersion, grammar
}

func localVersion(phpBinary string, timeout time.Duration, logf func(string)) (string, token.Version, bool) {
	local, err := phpdetect.Detect(phpBinary, timeout)
	if err != nil || local.Version == "" {
		return "", 0, false
	}
	grammar, err := token.ParseVersion(local.Version)
	if err != nil {
		if logf != nil {
			logf(fmt.Sprintf("PHP LSP: ignoring the version of the php on PATH: %v", err))
		}
		return "", 0, false
	}
	return local.Version, grammar, true
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

func (ws *Bootstrapped) Profile() symbols.BuiltinProfile {
	return *ws.profile.Load()
}

type ProfileChange int

const (
	ProfileKept ProfileChange = iota
	BuiltinsChanged
	GrammarsChanged
)

func (ws *Bootstrapped) SetProfile(profile symbols.BuiltinProfile) (ProfileChange, error) {
	ws.profileMu.Lock()
	defer ws.profileMu.Unlock()
	previous := ws.Profile()
	if previous.Equal(profile) {
		return ProfileKept, nil
	}
	if err := ws.Index.ReplaceBuiltins(profile); err != nil {
		return ProfileKept, fmt.Errorf("replace the builtins for PHP %s %v: %w", profile.PHPVersion, profile.Extensions, err)
	}
	ws.profile.Store(&profile)
	if previous.Grammars != profile.Grammars {
		return GrammarsChanged, nil
	}
	return BuiltinsChanged, nil
}

func (ws *Bootstrapped) ReparseForGrammars() int {
	reparsed := 0
	ws.Index.ReparseForGrammars(func(uri, source string) {
		ws.updateFromTree(uri, source)
		reparsed++
	})
	if reparsed > 0 {
		ws.Container.Analyze()
	}
	return reparsed
}
