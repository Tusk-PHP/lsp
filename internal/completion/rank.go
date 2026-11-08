package completion

import (
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Completion items are ordered by how close they are to the cursor, before
// anything is typed and among equally good matches after: what's declared in
// the current scope first, PHP's own names last. Editors re-rank by how well
// an item matches what's typed, so this decides the order of ties.
const (
	rankLocal      = 10 // variables assigned in the current function
	rankParameter  = 20 // the current function's parameters
	rankMember     = 30 // $this and the class's members
	rankImported   = 40 // names imported with `use`
	rankNamespace  = 50 // project names, by distance from the current namespace
	rankOtherRoot  = 60 // the project's other root namespaces (Database\, Tests\)
	rankVendor     = 70 // Composer dependencies
	rankLanguage   = 80 // PHP's builtins, keywords and types
	rankSubMaximum = 99999
)

// rankKey builds a sortText that orders by tier, then by sub-rank (smaller is
// closer), then by label without case. Fixed widths keep the string
// comparison numeric. It allocates the result only.
func rankKey(tier, sub int, label string) string {
	if sub < 0 {
		sub = 0
	}
	if sub > rankSubMaximum {
		sub = rankSubMaximum
	}
	buf := make([]byte, 0, 8+len(label))
	buf = strconv.AppendInt(buf, int64(tier), 10)
	for div := 10000; div > 0; div /= 10 {
		buf = append(buf, byte('0'+sub/div%10))
	}
	buf = append(buf, ' ')
	for i := 0; i < len(label); i++ {
		c := label[i]
		if 'A' <= c && c <= 'Z' {
			c += 'a' - 'A'
		}
		buf = append(buf, c)
	}
	return string(buf)
}

// nameRanker ranks class-likes, functions and constants for the file being
// edited, by where they're declared relative to it.
type nameRanker struct {
	namespace string   // the current namespace, without a leading backslash
	imported  []string // FQNs imported with `use`
}

func newNameRanker(currentNS string, uses []parser.UseNode) nameRanker {
	r := nameRanker{namespace: strings.TrimPrefix(currentNS, "\\"), imported: make([]string, 0, len(uses))}
	for _, u := range uses {
		r.imported = append(r.imported, strings.TrimPrefix(u.FullName, "\\"))
	}
	return r
}

// key returns the sortText for a symbol labelled label.
func (r nameRanker) key(sym *symbols.Symbol, label string) string {
	tier, sub := r.rank(sym)
	return rankKey(tier, sub, label)
}

func (r nameRanker) rank(sym *symbols.Symbol) (tier, sub int) {
	fqn := strings.TrimPrefix(sym.FQN, "\\")
	for _, imp := range r.imported {
		if strings.EqualFold(imp, fqn) {
			return rankImported, 0
		}
	}
	switch sym.Source {
	case symbols.SourceBuiltin:
		return rankLanguage, 0
	case symbols.SourceVendor:
		return rankVendor, 0
	}
	ns := ""
	if i := strings.LastIndexByte(fqn, '\\'); i >= 0 {
		ns = fqn[:i]
	}
	common := commonSegments(ns, r.namespace)
	nsDepth, curDepth := segments(ns), segments(r.namespace)
	if common == 0 && (nsDepth > 0 || curDepth > 0) {
		return rankOtherRoot, nsDepth
	}
	// Same namespace is 0. Its sub-namespaces follow, deeper ones later. Then
	// each level up: siblings under the parent, then under the grandparent.
	up, down := curDepth-common, nsDepth-common
	if up == 0 {
		return rankNamespace, down
	}
	return rankNamespace, up*1000 + down
}

// segments counts the parts of a namespace name: 0 for the global namespace.
func segments(ns string) int {
	if ns == "" {
		return 0
	}
	return strings.Count(ns, "\\") + 1
}

// commonSegments counts the leading namespace parts a and b share, ignoring case.
func commonSegments(a, b string) int {
	n := 0
	for a != "" && b != "" {
		ai, bi := strings.IndexByte(a, '\\'), strings.IndexByte(b, '\\')
		as, bs := a, b
		if ai >= 0 {
			as = a[:ai]
		}
		if bi >= 0 {
			bs = b[:bi]
		}
		if !strings.EqualFold(as, bs) {
			break
		}
		n++
		if ai < 0 || bi < 0 {
			break
		}
		a, b = a[ai+1:], b[bi+1:]
	}
	return n
}

// keywordKey is the sortText for a PHP keyword or type, which rank with PHP's
// other builtins.
func keywordKey(label string) string {
	return rankKey(rankLanguage, 0, label)
}

// memberDistances numbers the classes that declare members, in the order
// GetClassMembers lists them: the class itself 0, then its traits, then its
// parent and the parent's traits, and so on up.
func memberDistances(members []*symbols.Symbol) map[string]int {
	distance := map[string]int{}
	for _, m := range members {
		if _, ok := distance[m.ParentFQN]; !ok {
			distance[m.ParentFQN] = len(distance)
		}
	}
	return distance
}
