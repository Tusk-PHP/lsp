package symbols

import (
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
// in the order GetClassMembers lists them (the class's own first). The
// slice is shared: don't modify it.
func (idx *Index) ClassMembersNamed(classFQN, name string) []*Symbol {
	if strings.ContainsAny(classFQN, "|&(") {
		// Unions and intersections are rare and not worth a table.
		var out []*Symbol
		for _, m := range idx.GetClassMembers(classFQN) {
			if m.Name == name || m.Name == "$"+name {
				out = append(out, m)
			}
		}
		return out
	}
	table := idx.memberTable(classFQN)
	own, prop := table[name], table["$"+name]
	switch {
	case len(prop) == 0:
		return own
	case len(own) == 0:
		return prop
	}
	// Both a `name` and a `$name` member: keep the hierarchy's order.
	return mergeByOrder(idx.GetClassMembers(classFQN), name)
}

func mergeByOrder(members []*Symbol, name string) []*Symbol {
	var out []*Symbol
	for _, m := range members {
		if m.Name == name || m.Name == "$"+name {
			out = append(out, m)
		}
	}
	return out
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
		table[m.Name] = append(table[m.Name], m)
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
