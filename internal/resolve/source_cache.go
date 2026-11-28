package resolve

import (
	"strings"
	"sync"
	"unsafe"

	"github.com/Tusk-PHP/lsp/internal/scope"
)

// The providers pass a file around both as its source and as its lines, and
// resolving one expression can go back and forth between the two many times.
// Over a long file, joining and splitting it each time costs more than the
// resolution itself. Each file the Engine sees gets a fileState holding both
// forms, its scopes and the answers remembered for them; the Engine keeps the
// most recently used ones, and each Resolver remembers the one it is on.

// maxCachedSourceBytes bounds the source an Engine keeps cached files for. A
// file's scopes and remembered answers grow with its source, and every
// keystroke makes a new version of the edited file, so a bound on bytes keeps
// typing in a big file to a few versions while many small files fit for a
// request that resolves them in parallel. The newest file always stays, even
// when it is bigger than the bound.
const maxCachedSourceBytes = 1 << 20

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

// fileCache is an Engine's set of fileStates, keyed by source text: a new
// version of a file is a new key, so nothing in it goes stale, and the least
// recently used files leave once the cached sources pass
// maxCachedSourceBytes. Lines split from a cached file's source are found by
// where their first line starts, without joining them. recency is the head of
// a ring of the cached files: its older neighbour is the newest file, its
// newer neighbour the oldest.
type fileCache struct {
	mu       sync.Mutex
	bySource map[string]*cachedFile
	byLines  map[linesID]*cachedFile
	recency  cachedFile
	bytes    int
}

type cachedFile struct {
	state *fileState
	newer *cachedFile
	older *cachedFile
}

// linesID names a split of a source by where its first line's bytes start
// and how many lines it has. Lines split from the same source string share
// it; a match still has to hold the same lines.
type linesID struct {
	first *byte
	count int
}

func idOfLines(lines []string) linesID {
	if len(lines) == 0 {
		return linesID{}
	}
	return linesID{first: unsafe.StringData(lines[0]), count: len(lines)}
}

func (c *fileCache) ofLines(lines []string) *fileState {
	c.mu.Lock()
	entry := c.byLines[idOfLines(lines)]
	if entry != nil {
		c.touchLocked(entry)
	}
	c.mu.Unlock()
	if entry != nil && sameLines(entry.state.lines, lines) {
		return entry.state
	}
	return c.add(strings.Join(lines, "\n"), lines)
}

func (c *fileCache) ofSource(source string) *fileState {
	c.mu.Lock()
	entry := c.bySource[source]
	if entry != nil {
		c.touchLocked(entry)
	}
	c.mu.Unlock()
	if entry != nil {
		return entry.state
	}
	return c.add(source, SplitLines(source))
}

func (c *fileCache) add(source string, lines []string) *fileState {
	c.mu.Lock()
	defer c.mu.Unlock()
	if entry, ok := c.bySource[source]; ok {
		c.touchLocked(entry)
		return entry.state
	}
	if c.bySource == nil {
		c.bySource = make(map[string]*cachedFile)
		c.byLines = make(map[linesID]*cachedFile)
		c.recency.newer, c.recency.older = &c.recency, &c.recency
	}
	entry := &cachedFile{state: &fileState{source: source, lines: lines}}
	c.bySource[source] = entry
	c.byLines[idOfLines(lines)] = entry
	c.pushNewestLocked(entry)
	c.bytes += len(source)
	for c.bytes > maxCachedSourceBytes && c.recency.newer != entry {
		c.removeLocked(c.recency.newer)
	}
	return entry.state
}

func (c *fileCache) touchLocked(entry *cachedFile) {
	c.unlinkLocked(entry)
	c.pushNewestLocked(entry)
}

func (c *fileCache) pushNewestLocked(entry *cachedFile) {
	entry.newer, entry.older = &c.recency, c.recency.older
	entry.older.newer = entry
	c.recency.older = entry
}

func (c *fileCache) unlinkLocked(entry *cachedFile) {
	entry.newer.older = entry.older
	entry.older.newer = entry.newer
}

func (c *fileCache) removeLocked(entry *cachedFile) {
	c.unlinkLocked(entry)
	delete(c.bySource, entry.state.source)
	if id := idOfLines(entry.state.lines); c.byLines[id] == entry {
		delete(c.byLines, id)
	}
	c.bytes -= len(entry.state.source)
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
	r.file = r.files.ofLines(lines)
	return r.file
}

func (r *Resolver) fileOfSource(source string) *fileState {
	if r.file != nil && r.file.source == source {
		return r.file
	}
	r.file = r.files.ofSource(source)
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
