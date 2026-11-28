package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"time"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type Refresh struct {
	Paths      []string
	Profile    bool
	Config     func() (*config.Config, error)
	EditorOwns func(path string) bool
}

type Refreshed struct {
	Files          []string
	EditorOpened   []string
	Failed         []string
	Composer       bool
	ProfileChanged bool
	Profile        symbols.BuiltinProfile
	ProfileSource  string
}

type ConfigError struct {
	Err error
}

func (e *ConfigError) Error() string {
	return "load the configuration to resolve the PHP version: " + e.Err.Error()
}

func (e *ConfigError) Unwrap() error {
	return e.Err
}

const composerManifest = "composer.json"

var vendorStateFiles = []string{"composer.lock", "vendor/composer/installed.json"}

var composerStateFiles = append([]string{composerManifest}, vendorStateFiles...)

func (ws *Bootstrapped) ComposerStatePaths() []string {
	return ws.rootPaths(composerStateFiles)
}

func (ws *Bootstrapped) VendorStatePaths() []string {
	return ws.rootPaths(vendorStateFiles)
}

func (ws *Bootstrapped) rootPaths(names []string) []string {
	paths := make([]string, len(names))
	for i, name := range names {
		paths[i] = filepath.Join(ws.RootPath, filepath.FromSlash(name))
	}
	return paths
}

func (ws *Bootstrapped) ConfigPath() string {
	return filepath.Join(ws.RootPath, config.FileName)
}

func WatchGlobs() []string {
	globs := make([]string, 0, len(phpSourceExtensions)+len(composerStateFiles)+1)
	for _, ext := range phpSourceExtensions {
		globs = append(globs, "**/*"+ext)
	}
	for _, name := range append(slices.Clone(composerStateFiles), config.FileName) {
		globs = append(globs, "**/"+name)
	}
	return globs
}

func (ws *Bootstrapped) Refresh(r Refresh) (Refreshed, error) {
	var out Refreshed
	var errs []error
	manifest := filepath.Join(ws.RootPath, composerManifest)
	configPath := ws.ConfigPath()
	composerState := ws.ComposerStatePaths()
	seen := make(map[string]struct{}, len(r.Paths))
	var sources []string
	profile := r.Profile
	for _, path := range r.Paths {
		path = filepath.Clean(path)
		switch {
		case path == configPath:
			profile = true
		case slices.Contains(composerState, path):
			out.Composer = true
			profile = profile || path == manifest
		default:
			found, err := ws.sourcesAt(path)
			if err != nil {
				errs = append(errs, err)
				out.Failed = append(out.Failed, path)
			}
			for _, source := range found {
				if _, dup := seen[source]; !dup {
					seen[source] = struct{}{}
					sources = append(sources, source)
				}
			}
		}
	}
	for _, path := range sources {
		if r.editorOwns(path) {
			continue
		}
		uri := symbols.PathToURI(path)
		changed, err := ws.refreshFile(path, uri)
		if err != nil {
			errs = append(errs, err)
			out.Failed = append(out.Failed, path)
		}
		if !changed {
			continue
		}
		out.Files = append(out.Files, uri)
		if r.editorOwns(path) {
			out.EditorOpened = append(out.EditorOpened, uri)
		}
	}
	if out.Composer {
		ws.rereadAutoload()
	}
	if profile {
		if err := ws.refreshProfile(r.Config, &out); err != nil {
			errs = append(errs, err)
		}
	}
	if len(out.Files) > 0 || out.Composer {
		ws.Container.Analyze()
	}
	return out, errors.Join(errs...)
}

func (ws *Bootstrapped) refreshProfile(load func() (*config.Config, error), out *Refreshed) error {
	cfg, err := load()
	if err != nil {
		return &ConfigError{Err: err}
	}
	timeout := time.Duration(cfg.PHPDetectTimeoutMs) * time.Millisecond
	out.Profile, out.ProfileSource = ResolveBuiltinProfileWithConfig(ws.RootPath, cfg.PHPBinary, timeout, cfg, nil)
	out.ProfileChanged, err = ws.SetProfile(out.Profile)
	return err
}

func (r Refresh) editorOwns(path string) bool {
	return r.EditorOwns != nil && r.EditorOwns(path)
}

func (ws *Bootstrapped) sourcesAt(path string) ([]string, error) {
	if IsPHPSource(path) {
		if !ws.walkerReaches(path) {
			return nil, nil
		}
		return []string{path}, nil
	}
	if !ws.walkerReaches(path) || ws.excludedDir(path) {
		return nil, nil
	}
	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		uris := ws.Index.FilesUnder(path)
		paths := make([]string, 0, len(uris))
		for _, uri := range uris {
			if file, err := symbols.URIToPath(uri); err == nil {
				paths = append(paths, file)
			}
		}
		return paths, nil
	case err != nil:
		return nil, fmt.Errorf("look at %s: %w", path, err)
	case !info.IsDir():
		return nil, nil
	}
	var paths []string
	ws.WalkSources(path, func(source string, _ fs.FileInfo) error {
		paths = append(paths, source)
		return nil
	})
	return paths, nil
}

func (ws *Bootstrapped) refreshFile(path, uri string) (bool, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		if !ws.Index.HasFile(uri) {
			return false, nil
		}
		ws.RemoveFile(uri)
		ws.projectFiles.release(path)
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("re-index %s: %w", path, err)
	}
	if !ws.Index.HasFile(uri) && !ws.projectFiles.admitCreated(path) {
		return false, nil
	}
	ws.IndexFile(uri, string(content))
	return true, nil
}
