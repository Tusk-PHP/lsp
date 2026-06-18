package deps

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/composer/lockfile"
	"github.com/Tusk-PHP/lsp/internal/graph"
)

// TreeOptions controls how deeply BuildTree walks transitive dependencies.
type TreeOptions struct {
	// Depth is the maximum traversal depth from the direct packages (depth 0).
	// A value < 0 means unlimited. Full overrides Depth when set.
	Depth int

	// Full ignores Depth and expands all transitive dependencies.
	Full bool
}

// isPlatformRequirement reports whether a Composer requirement key is a
// platform pseudo-package that should be excluded from the dependency graph:
//   - the literal "php" (case-insensitive)
//   - anything starting with "ext-" or "lib-"
//   - anything without a "/" (i.e. not in vendor/name form)
func isPlatformRequirement(name string) bool {
	lower := strings.ToLower(name)
	if lower == "php" {
		return true
	}
	if strings.HasPrefix(lower, "ext-") {
		return true
	}
	if strings.HasPrefix(lower, "lib-") {
		return true
	}
	// Packages must be in "vendor/name" form; anything without a slash is platform.
	if !strings.Contains(name, "/") {
		return true
	}
	return false
}

// nodeLabel builds the node label for a package: "name@version" when version
// is non-empty, or just "name" otherwise.
func nodeLabel(name, version string) string {
	if version == "" {
		return name
	}
	return name + "@" + version
}

// BuildTree walks composer.lock transitively from the provided direct package
// names and produces a dependency-tree graph.
//
// Graph.Kind is "deps-tree", SchemaVersion is graph.SchemaVersion.
//
// Node: ID=name, Kind="package", Label="name@version" (or just "name" when
// version is absent), Meta{"direct": bool, "depth": int}. Packages referenced
// but absent from the lockfile are still emitted with Meta{"missing": true}.
//
// Edge: Kind="requires", From=parent name, To=required name. Platform
// requirements (php, ext-*, lib-*, anything without a "/") are skipped.
//
// Depth semantics: direct packages are at depth 0. When opts.Full is false and
// opts.Depth >= 0, packages at depth > opts.Depth are not expanded (their
// Require entries are not walked), but the node itself is still emitted so no
// inbound edge is left dangling. opts.Full expands everything regardless of
// opts.Depth.
//
// A package is expanded at most once (cycle-safe); multiple inbound edges to
// the same node are allowed.
//
// A nil lf returns an empty (non-nil) graph with Kind set.
func BuildTree(lf *lockfile.Lockfile, direct []string, opts TreeOptions) *graph.Graph {
	g := &graph.Graph{
		SchemaVersion: graph.SchemaVersion,
		Kind:          "deps-tree",
	}

	if lf == nil || len(direct) == 0 {
		return g
	}

	// nodes and edges are keyed by their IDs / (from,to) pairs to prevent
	// duplicates. We keep insertion order via separate slices, but use maps as
	// membership tests.
	addedNodes := make(map[string]struct{})
	// edgeKey: "from\x00to" — deduplicates parallel edges of the same kind.
	addedEdges := make(map[string]struct{})

	// visited tracks packages whose Require block has already been expanded.
	// A package is added to visited before its children are queued so that
	// cycles terminate immediately.
	visited := make(map[string]struct{})

	addNode := func(name, version string, direct bool, depth int, missing bool) {
		if _, ok := addedNodes[name]; ok {
			return
		}
		addedNodes[name] = struct{}{}
		meta := map[string]any{
			"direct": direct,
			"depth":  depth,
		}
		if missing {
			meta["missing"] = true
		}
		g.Nodes = append(g.Nodes, graph.Node{
			ID:    name,
			Kind:  "package",
			Label: nodeLabel(name, version),
			Meta:  meta,
		})
	}

	addEdge := func(from, to string) {
		key := from + "\x00" + to
		if _, ok := addedEdges[key]; ok {
			return
		}
		addedEdges[key] = struct{}{}
		g.Edges = append(g.Edges, graph.Edge{
			From: from,
			To:   to,
			Kind: "requires",
		})
	}

	// BFS queue entry.
	type item struct {
		name  string
		depth int
	}

	// Sort direct packages for a deterministic traversal order.
	sortedDirect := make([]string, len(direct))
	copy(sortedDirect, direct)
	sort.Strings(sortedDirect)

	queue := make([]item, 0, len(sortedDirect))
	for _, name := range sortedDirect {
		queue = append(queue, item{name: name, depth: 0})
	}

	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]

		pkg, found := lf.Find(cur.name)
		isDirect := cur.depth == 0

		// Emit the node (deduped internally).
		version := ""
		if found {
			version = pkg.Version
		}
		addNode(cur.name, version, isDirect, cur.depth, !found)

		// Mark as visited before expanding so cycles terminate.
		if _, alreadyVisited := visited[cur.name]; alreadyVisited {
			continue
		}
		visited[cur.name] = struct{}{}

		// Determine whether to expand this package's children.
		// We expand when: Full is true, OR Depth < 0 (unlimited), OR cur.depth < Depth.
		shouldExpand := opts.Full || opts.Depth < 0 || cur.depth < opts.Depth
		if !found || !shouldExpand {
			// Not expanding: still emit the node (done above), just skip children.
			continue
		}

		// Collect and sort child requires for deterministic ordering.
		children := make([]string, 0, len(pkg.Require))
		for dep := range pkg.Require {
			if isPlatformRequirement(dep) {
				continue
			}
			children = append(children, dep)
		}
		sort.Strings(children)

		for _, dep := range children {
			addEdge(cur.name, dep)
			// Only enqueue children that haven't been visited yet.
			// (We still emit the edge; just don't re-expand visited nodes.)
			if _, alreadyVisited := visited[dep]; !alreadyVisited {
				queue = append(queue, item{name: dep, depth: cur.depth + 1})
			}
		}
	}

	g.Sort()
	return g
}
