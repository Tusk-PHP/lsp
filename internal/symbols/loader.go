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
	attempts map[string]chan struct{}
}

type loadAttempt int

const (
	attemptRefused loadAttempt = iota
	attemptOwned
	attemptJoined
)

func (h *classLoaderHolder) begin(fqn string) (chan struct{}, loadAttempt) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := FoldName(fqn)
	if done, ok := h.attempts[key]; ok {
		return done, attemptJoined
	}
	if len(h.attempts) >= loaderAttemptLimit {
		return nil, attemptRefused
	}
	done := make(chan struct{})
	h.attempts[key] = done
	return done, attemptOwned
}

func (idx *Index) SetClassLoader(loader ClassLoader) {
	if loader == nil || idx.Ready() {
		idx.loader.Store(nil)
		return
	}
	idx.loader.Store(&classLoaderHolder{loader: loader, attempts: map[string]chan struct{}{}})
}

func (idx *Index) hasClassLike(fqn string) bool {
	idx.mu.RLock()
	defer idx.mu.RUnlock()
	return idx.classLikeFoldLocked(fqn)
}

func (idx *Index) ensureLoaded(holder *classLoaderHolder, fqn string) bool {
	if idx.hasClassLike(fqn) {
		return false
	}
	done, attempt := holder.begin(fqn)
	switch attempt {
	case attemptRefused:
		return false
	case attemptJoined:
		<-done
		return idx.hasClassLike(fqn)
	}
	class, ok := idx.loadAndIndex(holder.loader, fqn, done)
	if !ok {
		return false
	}
	holder.loader.Loaded(class)
	return idx.hasClassLike(fqn)
}

func (idx *Index) loadAndIndex(loader ClassLoader, fqn string, done chan struct{}) (LoadedClass, bool) {
	defer close(done)
	class, ok := loader.Load(fqn)
	if ok {
		idx.IndexFileWithSource(class.URI, class.Source, class.From)
	}
	return class, ok
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
	seen := map[string]bool{FoldName(fqn): true}
	for queue := []string{fqn}; len(queue) > 0; queue = queue[1:] {
		var ancestors []string
		idx.mu.RLock()
		idx.walkAncestors(queue[0], func(next string, _ bool) { ancestors = append(ancestors, next) })
		idx.mu.RUnlock()
		for _, ancestor := range ancestors {
			if key := FoldName(ancestor); !seen[key] {
				seen[key] = true
				idx.ensureLoaded(holder, ancestor)
				queue = append(queue, ancestor)
			}
		}
	}
	return found
}
