package workspace

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func (ws *Bootstrapped) autoload() []composer.AutoloadEntry {
	ws.autoloadOnce.Do(func() {
		ws.autoloadEntries = composer.GetAutoloadPaths(ws.RootPath)
	})
	return ws.autoloadEntries
}

func (ws *Bootstrapped) ClassLoader() symbols.ClassLoader {
	if ws == nil {
		return nil
	}
	return autoloader{ws}
}

type autoloader struct {
	ws *Bootstrapped
}

func (l autoloader) Load(fqn string) (symbols.LoadedClass, bool) {
	for _, file := range composer.ClassFiles(fqn, l.ws.autoload()) {
		path, ok := onDiskPath(file.Root, file.Parts)
		if !ok || !file.IsVendor && !l.ws.walkerReaches(path) {
			continue
		}
		content, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		from := symbols.SourceVendor
		if !file.IsVendor {
			if !l.ws.projectFiles.admitLoaded(path) {
				continue
			}
			from = symbols.SourceProject
		}
		return symbols.LoadedClass{URI: "file://" + path, Source: string(content), From: from}, true
	}
	return symbols.LoadedClass{}, false
}

func (l autoloader) Loaded(class symbols.LoadedClass) {
	if class.From != symbols.SourceProject {
		return
	}
	models.AnalyzeModelFile(l.ws.Index, l.ws.RootPath, l.ws.Framework, class.URI, class.Source, nil, nil)
}

func (ws *Bootstrapped) walkerReaches(path string) bool {
	rel, err := filepath.Rel(ws.RootPath, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return false
	}
	dirs := strings.Split(filepath.Dir(rel), string(filepath.Separator))
	for _, dir := range dirs {
		if ws.excludedDir(dir) {
			return false
		}
	}
	return true
}

func onDiskPath(root string, parts []string) (string, bool) {
	path := root
	for _, part := range parts {
		entries, err := os.ReadDir(path)
		if err != nil {
			return "", false
		}
		name, ok := onDiskName(entries, part)
		if !ok {
			return "", false
		}
		path = filepath.Join(path, name)
	}
	return path, true
}

func onDiskName(entries []os.DirEntry, name string) (string, bool) {
	folded := ""
	for _, entry := range entries {
		if entry.Name() == name {
			return name, true
		}
		if folded == "" && strings.EqualFold(entry.Name(), name) {
			folded = entry.Name()
		}
	}
	return folded, folded != ""
}
