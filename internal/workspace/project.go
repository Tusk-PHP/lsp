package workspace

import (
	"path/filepath"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Project is what a root's own files say about it before indexing: its
// configuration, framework and PHP profile.
type Project struct {
	Config    *config.Config
	Framework string
	Profile   symbols.BuiltinProfile
	// ProfileSource says where the PHP profile came from (see
	// ResolveBuiltinProfileWithConfig).
	ProfileSource string
}

// LoadProject reads root's .tusk-php.json (defaults when there is none),
// detects its framework unless the configuration names one, and resolves its
// PHP version and extensions. logf, when not nil, receives what profile
// detection reports. The configuration's Problems are left for the caller
// to report.
func LoadProject(root string, logf func(string)) (Project, error) {
	cfg, err := config.LoadFromFile(filepath.Join(root, config.FileName))
	if err != nil {
		return Project{}, err
	}
	framework := cfg.Framework
	if framework == "" || framework == "auto" {
		framework = config.DetectFramework(root)
	}
	timeout := time.Duration(cfg.PHPDetectTimeoutMs) * time.Millisecond
	profile, source := ResolveBuiltinProfileWithConfig(root, cfg.PHPBinary, timeout, cfg, logf)
	return Project{Config: cfg, Framework: framework, Profile: profile, ProfileSource: source}, nil
}
