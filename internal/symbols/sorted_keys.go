package symbols

import (
	"cmp"
	"slices"
	"sort"
	"strings"
)

// sortedKeys keeps a set of strings in case-insensitive order for prefix
// search (symbol names, class FQNs). The index reports each key that appears
// or disappears; the change is merged into the sorted list on the next
// search. Re-indexing a file whose names didn't change therefore leaves the
// list alone, and one that did costs a merge instead of a re-sort of every
// key.
//
// Until the list has been built once there is nothing to keep in step, so
// changes aren't recorded and the first search builds it from the index.
// When changes pile up (bulk indexing after a search), recording stops and
// the next search rebuilds the list the same way.
type sortedKeys struct {
	keys    []string
	built   bool
	added   map[string]struct{}
	removed map[string]struct{}
}

// maxPendingKeys is how many changes are recorded before a full rebuild
// becomes cheaper than merging them one by one.
func (s *sortedKeys) maxPendingKeys() int {
	return max(1024, len(s.keys)/8)
}

func (s *sortedKeys) add(key string) {
	if !s.built {
		return
	}
	if _, ok := s.removed[key]; ok {
		delete(s.removed, key)
		return
	}
	if s.added == nil {
		s.added = make(map[string]struct{})
	}
	s.added[key] = struct{}{}
	if len(s.added) > s.maxPendingKeys() {
		s.invalidate()
	}
}

func (s *sortedKeys) remove(key string) {
	if !s.built {
		return
	}
	if _, ok := s.added[key]; ok {
		delete(s.added, key)
		return
	}
	if s.removed == nil {
		s.removed = make(map[string]struct{})
	}
	s.removed[key] = struct{}{}
	if len(s.removed) > s.maxPendingKeys() {
		s.invalidate()
	}
}

// invalidate drops the list; the next search rebuilds it from the index.
func (s *sortedKeys) invalidate() {
	s.built = false
	s.added, s.removed = nil, nil
}

func (s *sortedKeys) needsUpdate() bool {
	return !s.built || len(s.added) > 0 || len(s.removed) > 0
}

// update brings the list in step with the index. all yields every key the
// list should hold; it is only called when the list has to be rebuilt.
// The caller holds the index's write lock.
func (s *sortedKeys) update(all func(yield func(string))) {
	if !s.built {
		s.keys = s.keys[:0]
		all(func(key string) { s.keys = append(s.keys, key) })
		slices.SortFunc(s.keys, compareKeys)
		s.built = true
		s.added, s.removed = nil, nil
		return
	}
	if len(s.removed) > 0 {
		s.keys = deleteSortedKeys(s.keys, s.removed)
		s.removed = nil
	}
	if len(s.added) > 0 {
		s.keys = mergeSortedKeys(s.keys, s.added)
		s.added = nil
	}
}

// deleteSortedKeys removes removed from keys, which is sorted by compareKeys,
// finding each by binary search and moving each run of kept keys once.
func deleteSortedKeys(keys []string, removed map[string]struct{}) []string {
	at := make([]int, 0, len(removed))
	for key := range removed {
		i := sort.Search(len(keys), func(i int) bool { return compareKeys(keys[i], key) >= 0 })
		if i < len(keys) && keys[i] == key {
			at = append(at, i)
		}
	}
	if len(at) == 0 {
		return keys
	}
	slices.Sort(at)
	w := at[0]
	for n, i := range at {
		end := len(keys)
		if n+1 < len(at) {
			end = at[n+1]
		}
		w += copy(keys[w:], keys[i+1:end])
	}
	clear(keys[w:])
	return keys[:w]
}

// mergeSortedKeys inserts added into keys, which is sorted by compareKeys,
// moving each run of existing keys once.
func mergeSortedKeys(keys []string, added map[string]struct{}) []string {
	adds := make([]string, 0, len(added))
	for key := range added {
		adds = append(adds, key)
	}
	slices.SortFunc(adds, compareKeys)

	n, m := len(keys), len(adds)
	keys = slices.Grow(keys, m)[:n+m]
	hi := n // keys[:hi] are the existing keys not yet moved
	for j := m - 1; j >= 0; j-- {
		pos := sort.Search(hi, func(i int) bool { return compareKeys(keys[i], adds[j]) > 0 })
		copy(keys[pos+j+1:hi+j+1], keys[pos:hi])
		keys[pos+j] = adds[j]
		hi = pos
	}
	return keys
}

// compareKeys orders keys case-insensitively, then by their exact spelling so
// the order doesn't depend on the order keys were added in.
func compareKeys(a, b string) int {
	if c := compareFold(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

// compareFold compares FoldName(a) with FoldName(b) without allocating.
func compareFold(a, b string) int {
	for i := 0; i < len(a) && i < len(b); i++ {
		ca, cb := lowerASCII(a[i]), lowerASCII(b[i])
		if ca != cb {
			return cmp.Compare(ca, cb)
		}
	}
	return cmp.Compare(len(a), len(b))
}

// hasFoldPrefix reports strings.HasPrefix(FoldName(s), foldedPrefix) without
// allocating.
func hasFoldPrefix(s, foldedPrefix string) bool {
	if len(s) < len(foldedPrefix) {
		return false
	}
	for i := 0; i < len(foldedPrefix); i++ {
		if lowerASCII(s[i]) != foldedPrefix[i] {
			return false
		}
	}
	return true
}
