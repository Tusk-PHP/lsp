package workspace

import (
	"slices"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// castDependents knows which model files' attribute cast types were read
// from which classes, so that an edit to one of those classes can refresh
// them. It holds one entry per class a project model's casts are typed
// from, and a model file's entries are replaced each time the file is
// analysed and dropped when it is removed.
type castDependents struct {
	mu      sync.Mutex
	byModel map[string][]string
	byClass map[string]map[string]struct{}
}

func (d *castDependents) replaceAll(dependencies models.CastDependencies) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.byModel, d.byClass = nil, nil
	for uri, classes := range dependencies {
		d.setLocked(uri, classes)
	}
}

func (d *castDependents) set(uri string, classes []string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.setLocked(uri, classes)
}

func (d *castDependents) setLocked(uri string, classes []string) {
	for _, class := range d.byModel[uri] {
		delete(d.byClass[class], uri)
		if len(d.byClass[class]) == 0 {
			delete(d.byClass, class)
		}
	}
	delete(d.byModel, uri)
	if len(classes) == 0 {
		return
	}
	keys := make([]string, 0, len(classes))
	for _, class := range classes {
		key := strings.ToLower(class)
		if slices.Contains(keys, key) {
			continue
		}
		keys = append(keys, key)
		if d.byClass == nil {
			d.byModel, d.byClass = map[string][]string{}, map[string]map[string]struct{}{}
		}
		if d.byClass[key] == nil {
			d.byClass[key] = map[string]struct{}{}
		}
		d.byClass[key][uri] = struct{}{}
	}
	d.byModel[uri] = keys
}

// tracking reports whether any model's cast types were read from a class.
func (d *castDependents) tracking() bool {
	d.mu.Lock()
	defer d.mu.Unlock()
	return len(d.byClass) > 0
}

// of returns the model files, other than except, whose cast types were read
// from one of classes, sorted.
func (d *castDependents) of(classes []string, except string) []string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(d.byClass) == 0 {
		return nil
	}
	seen := map[string]bool{except: true}
	var out []string
	for _, class := range classes {
		for uri := range d.byClass[strings.ToLower(class)] {
			if !seen[uri] {
				seen[uri] = true
				out = append(out, uri)
			}
		}
	}
	slices.Sort(out)
	return out
}

// declaredClasses lists the classes, interfaces, traits and enums the index
// has from uri.
func declaredClasses(index *symbols.Index, uri string) []string {
	var classes []string
	for _, sym := range index.GetFileSymbols(uri) {
		switch sym.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
			classes = append(classes, sym.FQN)
		}
	}
	return classes
}

// RefreshModels re-analyses the models declared in the files uris, from
// the text the index has for them, so that what they read from other files
// (the type of a cast class) is current. Its work grows with those files,
// so it runs in the background, like re-indexing.
func (ws *Bootstrapped) RefreshModels(uris []string) {
	if ws == nil {
		return
	}
	for _, uri := range uris {
		if source := ws.Index.GetFileSource(uri); source != "" {
			ws.AnalyzeModelFile(uri, source)
		}
	}
}
