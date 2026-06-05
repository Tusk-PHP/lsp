package graph

import (
	"github.com/Tusk-PHP/lsp/internal/models"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// BuildModels builds a model/entity relation graph.
// Nodes are one "model" vertex per concrete model class; edges are the
// relation calls between them (HasMany, BelongsTo, OneToMany, …).
//
// If idx is nil, an empty graph with Kind "models" is returned.
//
// Deps post-processing is applied via opts after the graph is assembled;
// g.Sort() is called before returning.
func BuildModels(idx *symbols.Index, rootPath, framework string, opts Options) *Graph {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "models"}

	if idx == nil {
		return g
	}

	// nodeMap tracks known nodes by ID (de-dup by ID).
	nodeMap := map[string]Node{}

	addNode := func(id, kind, label string) {
		if _, ok := nodeMap[id]; ok {
			return
		}
		nodeMap[id] = Node{ID: id, Kind: kind, Label: label}
	}

	// edgeSet de-duplicates edges by (From, To, Kind).
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

	// Step 1: add a node for every concrete model class.
	for _, s := range models.ModelClasses(idx, rootPath, framework) {
		addNode(s.FQN, "model", s.FQN)
	}

	// Step 2: add edges for every detected relation. Ensure both endpoints
	// exist as nodes — a target class may not appear in ModelClasses if it is
	// a co-model not yet indexed or outside the project root (e.g. a pivot).
	for _, r := range models.ModelRelations(idx, rootPath, framework) {
		addNode(r.SourceModel, "model", r.SourceModel)
		addNode(r.TargetModel, "model", r.TargetModel)
		addEdge(r.SourceModel, r.TargetModel, r.RelationType)
	}

	// Assemble slices.
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

	// Apply deps post-processing, then sort for deterministic output.
	g = applyDeps(g, idx, opts)
	g.Sort()
	return g
}
