package resolve

import (
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/scope"
)

// The providers pass a file around both as its source and as its lines, and
// resolving one expression can go back and forth between the two many times.
// Over a long file, joining and splitting it each time costs more than the
// resolution itself. Each file the Engine sees gets a fileState holding both
// forms, its scopes and the answers remembered for them; the Engine keeps the
// most recently used ones, and each Resolver remembers the one it is on.

// maxCachedFiles bounds the files an Engine keeps. A request works on one file
// at a time, or on one per worker when it resolves many files in parallel.
const maxCachedFiles = 32

const (
	maxVarMemos     = 8192
	maxClosureMemos = 4096
)

type fileState struct {
	source string
	lines  []string

	docOnce sync.Once
	doc     *scope.Document

	mu           sync.Mutex
	closureMemos generationMemo[*scope.Scope, closureMemo]
	varMemos     generationMemo[varMemoKey, ResolvedType]
}

func (f *fileState) document() *scope.Document {
	f.docOnce.Do(func() { f.doc = scope.Collect(f.source) })
	return f.doc
}

// generationMemo holds answers worked out against one index generation, and
// forgets them all once the generation changes or it reaches its limit.
type generationMemo[K comparable, V any] struct {
	gen     uint64
	answers map[K]V
}

func (m *generationMemo[K, V]) get(gen uint64, key K) (V, bool) {
	if m.gen != gen {
		var none V
		return none, false
	}
	v, ok := m.answers[key]
	return v, ok
}

func (m *generationMemo[K, V]) put(gen uint64, key K, v V, limit int) {
	if m.answers == nil || m.gen != gen || len(m.answers) >= limit {
		m.answers = make(map[K]V)
		m.gen = gen
	}
	m.answers[key] = v
}

// fileCache is an Engine's bounded set of fileStates, keyed by source text: a
// new version of a file is a new key, so nothing in it goes stale, and the
// least recently used file leaves when the cache is full.
type fileCache struct {
	mu    sync.Mutex
	files map[string]*cachedFile
	clock uint64
}

type cachedFile struct {
	state *fileState
	used  uint64
}

func (c *fileCache) get(source string, lines []string) *fileState {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.clock++
	if entry, ok := c.files[source]; ok {
		entry.used = c.clock
		return entry.state
	}
	if lines == nil {
		lines = SplitLines(source)
	}
	if c.files == nil {
		c.files = make(map[string]*cachedFile, maxCachedFiles)
	}
	if len(c.files) >= maxCachedFiles {
		c.evictLocked()
	}
	state := &fileState{source: source, lines: lines}
	c.files[source] = &cachedFile{state: state, used: c.clock}
	return state
}

func (c *fileCache) evictLocked() {
	var oldest string
	var oldestUse uint64
	first := true
	for source, entry := range c.files {
		if first || entry.used < oldestUse {
			oldest, oldestUse, first = source, entry.used, false
		}
	}
	delete(c.files, oldest)
}

// sameLines reports whether a and b hold the same lines. Lines split from the
// same source share their bytes, so this is usually a pointer comparison per
// line rather than a comparison of their text.
func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 || &a[0] == &b[0] {
		return true // the same slice, as Lines hands out
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func (r *Resolver) fileOfLines(lines []string) *fileState {
	if r.file != nil && sameLines(r.file.lines, lines) {
		return r.file
	}
	r.file = r.files.get(strings.Join(lines, "\n"), lines)
	return r.file
}

func (r *Resolver) fileOfSource(source string) *fileState {
	if r.file != nil && r.file.source == source {
		return r.file
	}
	r.file = r.files.get(source, nil)
	return r.file
}

// JoinLines returns lines joined into the source they were split from,
// reusing the result while the lines don't change.
func (r *Resolver) JoinLines(lines []string) string {
	return r.fileOfLines(lines).source
}

// Lines returns the source split into lines, reusing the result while the
// source doesn't change. Treat the result as read-only.
func (r *Resolver) Lines(source string) []string {
	return r.fileOfSource(source).lines
}
