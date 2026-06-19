// Package graph defines the versioned DTO used to represent dependency/container
// graphs, plus JSON serialisation with deterministic (sorted) output.
package graph

import (
	"encoding/json"
	"io"
	"sort"
)

// SchemaVersion is the current wire-format version embedded in every Graph.
const SchemaVersion = 1

// Graph is the top-level export container. Kind identifies what the graph
// represents (e.g. "container").
type Graph struct {
	SchemaVersion int    `json:"schemaVersion"`
	Kind          string `json:"kind"` // e.g. "container"
	Nodes         []Node `json:"nodes"`
	Edges         []Edge `json:"edges"`
}

// Node is a single vertex. Kind is one of "binding", "class", or
// "dependency-boundary". Meta carries optional renderer/consumer hints.
type Node struct {
	ID    string         `json:"id"`
	Kind  string         `json:"kind"` // "binding" | "class" | "dependency-boundary"
	Label string         `json:"label"`
	Meta  map[string]any `json:"meta,omitempty"`
}

// Edge is a directed relationship between two nodes. Kind is one of "binds"
// or "injects".
type Edge struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // "binds" | "injects"
}

// DepsMode controls how vendor/builtin dependencies are represented in a graph.
type DepsMode string

const (
	// DepsNone (default) omits vendor and builtin nodes entirely.
	DepsNone DepsMode = "none"
	// DepsBoundary collapses each vendor package into a single boundary node.
	DepsBoundary DepsMode = "boundary"
	// DepsFull includes every vendor and builtin node individually.
	DepsFull DepsMode = "full"
)

// PackageResolver maps a vendor FQN to its composer package name and version.
// Returns ok=false when the FQN cannot be resolved to a known package.
type PackageResolver func(fqn string) (pkg, version string, ok bool)

// Options parameterise graph builders (e.g. BuildReferences).
type Options struct {
	Deps     DepsMode
	Packages PackageResolver // may be nil; required only for DepsBoundary
}

// Sort reorders Nodes by ID ascending and Edges by the tuple (From, To, Kind)
// ascending. The result is deterministic across runs.
func (g *Graph) Sort() {
	sort.Slice(g.Nodes, func(i, j int) bool {
		return g.Nodes[i].ID < g.Nodes[j].ID
	})
	sort.Slice(g.Edges, func(i, j int) bool {
		a, b := g.Edges[i], g.Edges[j]
		if a.From != b.From {
			return a.From < b.From
		}
		if a.To != b.To {
			return a.To < b.To
		}
		return a.Kind < b.Kind
	})
}

// EncodeJSON calls Sort and then writes the graph as 2-space-indented JSON to w.
// Nodes and Edges always serialise as arrays (never JSON null) so consumers can
// treat them as a stable list contract even for empty graphs.
func (g *Graph) EncodeJSON(w io.Writer) error {
	g.Sort()
	if g.Nodes == nil {
		g.Nodes = []Node{}
	}
	if g.Edges == nil {
		g.Edges = []Edge{}
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(g)
}
