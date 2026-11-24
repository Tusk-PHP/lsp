package symbols

import (
	"strings"
	"sync"
)

const loaderAttemptLimit = 10000

type LoadedClass struct {
	URI    string
	Source string
	From   SymbolSource
}

type ClassLoader interface {
	Load(fqn string) (LoadedClass, bool)
	Loaded(class LoadedClass)
}

type classLoaderHolder struct {
	loader   ClassLoader
	mu       sync.Mutex
	attempts map[string]bool
}

func (h *classLoaderHolder) firstAttempt(fqn string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := strings.ToLower(fqn)
	if h.attempts[key] || len(h.attempts) >= loaderAttemptLimit {
		return false
	}
	h.attempts[key] = true
	return true
}

func (idx *Index) SetClassLoader(loader ClassLoader) {
	if loader == nil || idx.Ready() {
		idx.loader.Store(nil)
		return
	}
	idx.loader.Store(&classLoaderHolder{loader: loader, attempts: map[string]bool{}})
}

func (idx *Index) ensureLoaded(holder *classLoaderHolder, fqn string) bool {
	idx.mu.RLock()
	exists := idx.classLikeFoldLocked(fqn)
	idx.mu.RUnlock()
	if exists || !holder.firstAttempt(fqn) {
		return false
	}
	class, ok := holder.loader.Load(fqn)
	if !ok {
		return false
	}
	idx.IndexFileWithSource(class.URI, class.Source, class.From)
	holder.loader.Loaded(class)
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.classLikeFoldLocked(fqn)
}

func classPart(fqn string) string {
	fqn = strings.TrimPrefix(fqn, "\\")
	if i := strings.Index(fqn, "::"); i >= 0 {
		fqn = fqn[:i]
	}
	return fqn
}

func (idx *Index) ensureClass(fqn string) bool {
	fqn = classPart(fqn)
	if fqn == "" {
		return false
	}
	holder := idx.loader.Load()
	if holder == nil || idx.Ready() {
		return idx.ensureGenerated(fqn)
	}
	found := idx.ensureLoaded(holder, fqn) || idx.ensureGenerated(fqn)
	seen := map[string]bool{strings.ToLower(fqn): true}
	for queue := []string{fqn}; len(queue) > 0; queue = queue[1:] {
		var ancestors []string
		idx.mu.RLock()
		idx.walkAncestors(queue[0], func(next string, _ bool) { ancestors = append(ancestors, next) })
		idx.mu.RUnlock()
		for _, ancestor := range ancestors {
			if key := strings.ToLower(ancestor); !seen[key] {
				seen[key] = true
				idx.ensureLoaded(holder, ancestor)
				queue = append(queue, ancestor)
			}
		}
	}
	return found
}
