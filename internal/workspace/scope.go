package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func walkScope(root string, exclude excludeRules, admit func(path string) bool, visit func(path string)) bool {
	complete := true
	walkSources(root, root, exclude, func(path string, _ fs.FileInfo) error {
		if !IsBlade(path) && !admit(path) {
			complete = false
			return filepath.SkipAll
		}
		visit(path)
		return nil
	})
	return complete
}

func scopeChanged(old, cfg *config.Config) bool {
	return old.MaxIndexFiles != cfg.MaxIndexFiles || !slices.Equal(old.ExcludePaths, cfg.ExcludePaths)
}

func (ws *Bootstrapped) scope(s *settings) (paths []string, counted int) {
	limit := s.config.MaxIndexFiles
	walkScope(ws.RootPath, s.exclude, func(string) bool {
		if limit > 0 && counted >= limit {
			return false
		}
		counted++
		return true
	}, func(path string) {
		paths = append(paths, path)
	})
	return paths, counted
}

func (ws *Bootstrapped) rescope(old, next *settings, editorOwns func(path string) bool, out *Refreshed) error {
	before, _ := ws.scope(old)
	after, counted := ws.scope(next)
	ws.projectFiles.rescope(next.config.MaxIndexFiles, counted)
	kept := make(map[string]struct{}, len(after))
	for _, path := range after {
		kept[path] = struct{}{}
	}
	was := make(map[string]struct{}, len(before))
	var errs []error
	for _, path := range before {
		was[path] = struct{}{}
		uri := symbols.PathToURI(path)
		if _, stays := kept[path]; stays || editorOwns(path) || !ws.Index.HasFile(uri) || ws.composerLoads(path) {
			continue
		}
		ws.RemoveFile(uri)
		out.Files = append(out.Files, uri)
	}
	for _, path := range after {
		uri := symbols.PathToURI(path)
		if _, stayed := was[path]; stayed || editorOwns(path) || ws.Index.HasFile(uri) {
			continue
		}
		content, err := os.ReadFile(path)
		if errors.Is(err, fs.ErrNotExist) {
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("index %s: %w", path, err))
			out.Failed = append(out.Failed, path)
			continue
		}
		ws.IndexFile(uri, string(content))
		out.Files = append(out.Files, uri)
		if editorOwns(path) {
			out.EditorOpened = append(out.EditorOpened, uri)
		}
	}
	return errors.Join(errs...)
}

func (ws *Bootstrapped) composerLoads(path string) bool {
	for _, entry := range ws.autoload() {
		switch {
		case entry.IsFile:
			if entry.Path == path {
				return true
			}
		case entry.IsVendor:
			if strings.HasPrefix(path, entry.Path+string(filepath.Separator)) {
				return true
			}
		}
	}
	return false
}
