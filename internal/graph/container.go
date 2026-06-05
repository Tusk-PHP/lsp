package graph

import (
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BuildContainer builds a container dependency graph from a ContainerAnalyzer.
// It returns an empty graph (never nil) if idx or ca is nil.
//
// Deps post-processing modes:
//   - DepsNone (default / ""): remove vendor and builtin nodes and their edges.
//   - DepsBoundary: collapse vendor FQNs into per-package "dependency-boundary"
//     nodes; builtins are dropped. Package name is resolved via opts.Packages
//     when non-nil; falls back to the top-level namespace segment.
//   - DepsFull: include all nodes and edges unchanged.
func BuildContainer(idx *symbols.Index, ca *container.ContainerAnalyzer, opts Options) *Graph {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "container"}

	if idx == nil || ca == nil {
		return g
	}

	// nodeMap tracks known nodes by ID.  We prefer "binding" over "class" when
	// a FQN appears as both an abstract and a concrete class — a binding node
	// carries the container intent (interface/contract), whereas "class" is
	// merely an implementation detail. Keeping "binding" ensures graph consumers
	// can distinguish service contracts from their implementations.
	nodeMap := map[string]Node{}

	addNode := func(id, kind, label string) {
		if existing, ok := nodeMap[id]; ok {
			// Prefer "binding" over "class" for the same ID.
			if existing.Kind == "binding" {
				return
			}
			if kind != "binding" {
				return
			}
			// incoming kind is "binding", overwrite.
		}
		nodeMap[id] = Node{ID: id, Kind: kind, Label: label}
	}

	// edgeSet de-duplicates edges by their canonical key.
	type edgeKey struct{ from, to, kind string }
	edgeSet := map[edgeKey]Edge{}

	addEdge := func(from, to, kind string) {
		if from == "" || to == "" {
			return
		}
		k := edgeKey{from, to, kind}
		if _, ok := edgeSet[k]; !ok {
			edgeSet[k] = Edge{From: from, To: to, Kind: kind}
		}
	}

	// --- Step 3: bindings ---
	bindings := ca.GetBindings()
	for _, b := range bindings {
		addNode(b.Abstract, "binding", b.Abstract)
		if b.Concrete != "" {
			addNode(b.Concrete, "class", b.Concrete)
			addEdge(b.Abstract, b.Concrete, "binds")
		}
	}

	// --- Step 4: constructor injection for each distinct concrete FQN ---
	// Collect distinct non-empty concretes first to avoid duplicate AnalyzeConstructorInjection calls.
	visitedConcretes := map[string]struct{}{}
	for _, b := range bindings {
		if b.Concrete != "" {
			visitedConcretes[b.Concrete] = struct{}{}
		}
	}

	for concrete := range visitedConcretes {
		deps := ca.AnalyzeConstructorInjection(concrete)
		for _, dep := range deps {
			target := dep.ResolvedConcrete
			if target == "" {
				target = dep.TypeHint
			}
			if target == "" {
				continue
			}
			addNode(target, "class", target)
			addEdge(concrete, target, "injects")
		}
	}

	// Assemble slices before post-processing.
	nodes := make([]Node, 0, len(nodeMap))
	for _, n := range nodeMap {
		nodes = append(nodes, n)
	}
	edges := make([]Edge, 0, len(edgeSet))
	for _, e := range edgeSet {
		edges = append(edges, e)
	}

	g.Nodes = nodes
	g.Edges = edges

	// --- Step 6: deps post-processing ---
	applyDeps(g, idx, opts)

	g.Sort()
	return g
}
