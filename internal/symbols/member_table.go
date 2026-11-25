package symbols

import (
	"slices"
	"strings"
	"sync"
)

// memberTableLimit bounds how many classes' member tables are kept.
const memberTableLimit = 4096

// memberTables keeps, per class, its members (own, traits', parents',
// interface constants, in GetClassMembers order) grouped by name, so finding
// one member is a lookup instead of collecting and scanning the hierarchy.
// The tables are rebuilt from scratch whenever the index changes.
type memberTables struct {
	mu     sync.Mutex
	gen    uint64
	tables map[string]map[string][]*Symbol
}

// ClassMembersNamed returns the members of classFQN named name or "$"+name,
// in the order GetClassMembers lists them (the class's own first). Methods
// match ignoring case, as PHP calls them; properties, constants and enum
// cases match exactly. The slice may be shared: don't modify it.
func (idx *Index) ClassMembersNamed(classFQN, name string) []*Symbol {
	if strings.ContainsAny(classFQN, "|&(") {
		// Unions and intersections are rare and not worth a table.
		var out []*Symbol
		for _, m := range idx.GetClassMembers(classFQN) {
			if memberNamed(m, name) {
				out = append(out, m)
			}
		}
		return out
	}
	var buf [64]byte
	bucket := idx.memberTable(classFQN)[string(FoldKey(&buf, strings.TrimPrefix(name, "$")))]
	for i, m := range bucket {
		if memberNamed(m, name) {
			continue
		}
		out := slices.Clone(bucket[:i])
		for _, m := range bucket[i+1:] {
			if memberNamed(m, name) {
				out = append(out, m)
			}
		}
		return out
	}
	return bucket
}

func memberNamed(m *Symbol, name string) bool {
	if m.Kind == KindMethod {
		return strings.EqualFold(m.Name, name)
	}
	return m.Name == name || len(m.Name) == len(name)+1 && m.Name[0] == '$' && m.Name[1:] == name
}

func (idx *Index) memberTable(classFQN string) map[string][]*Symbol {
	gen := idx.Generation()
	t := &idx.members
	t.mu.Lock()
	if t.gen != gen || t.tables == nil {
		t.tables = make(map[string]map[string][]*Symbol)
		t.gen = gen
	}
	if table, ok := t.tables[classFQN]; ok {
		t.mu.Unlock()
		return table
	}
	t.mu.Unlock()

	members := idx.GetClassMembers(classFQN)
	table := make(map[string][]*Symbol, len(members))
	for _, m := range members {
		key := strings.ToLower(strings.TrimPrefix(m.Name, "$"))
		table[key] = append(table[key], m)
	}

	t.mu.Lock()
	// Built for the generation it started at; a change since then drops it.
	if t.gen == gen && idx.Generation() == gen {
		if len(t.tables) >= memberTableLimit {
			t.tables = make(map[string]map[string][]*Symbol)
		}
		t.tables[classFQN] = table
	}
	t.mu.Unlock()
	return table
}

// FoldKey lower-cases an ASCII s into buf, so that a lookup such as
// m[string(FoldKey(&buf, s))] doesn't allocate. Longer or non-ASCII names
// fall back to strings.ToLower.
func FoldKey(buf *[64]byte, s string) []byte {
	if len(s) > len(buf) {
		return []byte(strings.ToLower(s))
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 0x80 {
			return []byte(strings.ToLower(s))
		}
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf[i] = c
	}
	return buf[:len(s)]
}
