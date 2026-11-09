package resolve

import (
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// parseCacheSize bounds each of the resolver's parse caches. When one fills
// up it starts over, which keeps it to what the current requests touch.
const parseCacheSize = 4096

// parseCache remembers parsed docblocks and PHPDoc types by their text.
// Resolving a chain reads the same members' PHPDoc again and again; the text
// is the key, so an entry never goes stale and nothing has to invalidate it.
// Results are shared: callers must not modify them.
type parseCache struct {
	mu    sync.Mutex
	docs  map[string]*parser.DocBlock
	types map[string]ResolvedType
}

// parseDoc is parser.ParseDocBlock, cached. The result is read-only.
func (r *Resolver) parseDoc(raw string) *parser.DocBlock {
	if raw == "" {
		return nil
	}
	c := &r.parsed
	c.mu.Lock()
	if doc, ok := c.docs[raw]; ok {
		c.mu.Unlock()
		return doc
	}
	c.mu.Unlock()
	doc := parser.ParseDocBlock(raw)
	c.mu.Lock()
	if c.docs == nil || len(c.docs) >= parseCacheSize {
		c.docs = make(map[string]*parser.DocBlock, 256)
	}
	c.docs[raw] = doc
	c.mu.Unlock()
	return doc
}

// parseType is ParseGenericType, cached. The result is read-only: transform
// it (applyEnv and the like build new types) rather than changing it.
func (r *Resolver) parseType(raw string) ResolvedType {
	if raw == "" {
		return ResolvedType{}
	}
	c := &r.parsed
	c.mu.Lock()
	if t, ok := c.types[raw]; ok {
		c.mu.Unlock()
		return t
	}
	c.mu.Unlock()
	t := ParseGenericType(raw)
	c.mu.Lock()
	if c.types == nil || len(c.types) >= parseCacheSize {
		c.types = make(map[string]ResolvedType, 256)
	}
	c.types[raw] = t
	c.mu.Unlock()
	return t
}
