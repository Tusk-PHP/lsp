package graph

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// classify returns the classification string for a PHP FQN based on how it is
// indexed. nil symbol or SourceBuiltin => "builtin"; SourceVendor => "vendor";
// SourceProject => "project". A nil lookup result is "unresolved" (not vendor)
// so that framework strings (e.g. "cache", "auth") that are unresolvable are
// still kept in DepsNone output rather than treated as external packages.
func classify(idx *symbols.Index, fqn string) string {
	sym := idx.Lookup(fqn)
	if sym == nil {
		return "unresolved"
	}
	switch sym.Source {
	case symbols.SourceBuiltin:
		return "builtin"
	case symbols.SourceVendor:
		return "vendor"
	default:
		return "project"
	}
}

// topLevelNamespace returns the first namespace segment of an FQN, used as a
// fallback boundary node ID when PackageResolver returns ok=false.
// e.g. "Monolog\\Logger" => "Monolog", "Illuminate\\Contracts\\Auth\\Factory" => "Illuminate".
func topLevelNamespace(fqn string) string {
	parts := strings.SplitN(fqn, "\\", 2)
	return parts[0]
}

// applyDeps applies the deps post-processing mode from opts to g, mutating and
// returning it. The graph must already have Nodes and Edges assembled with FQNs
// as node IDs. Sort is NOT called here; callers are responsible for sorting.
//
// Modes:
//   - DepsNone (default / ""): remove vendor and builtin nodes and their edges.
//   - DepsBoundary: collapse vendor FQNs into per-package "dependency-boundary"
//     nodes; builtins are dropped. Package name is resolved via opts.Packages
//     when non-nil; falls back to the top-level namespace segment.
//   - DepsFull: keep everything as-is.
func applyDeps(g *Graph, idx *symbols.Index, opts Options) *Graph {
	nodes := g.Nodes
	edges := g.Edges

	mode := opts.Deps
	if mode == "" {
		mode = DepsNone
	}

	switch mode {
	case DepsFull:
		// Keep everything as-is.

	case DepsNone:
		// Remove vendor and builtin nodes and any edge that references them.
		kept := map[string]struct{}{}
		var filteredNodes []Node
		for _, n := range nodes {
			c := classify(idx, n.ID)
			if c == "vendor" || c == "builtin" {
				continue
			}
			filteredNodes = append(filteredNodes, n)
			kept[n.ID] = struct{}{}
		}
		var filteredEdges []Edge
		for _, e := range edges {
			if _, okFrom := kept[e.From]; !okFrom {
				continue
			}
			if _, okTo := kept[e.To]; !okTo {
				continue
			}
			filteredEdges = append(filteredEdges, e)
		}
		g.Nodes = filteredNodes
		g.Edges = filteredEdges

	case DepsBoundary:
		// Collapse vendor FQNs into per-package "dependency-boundary" nodes.
		// Builtins are dropped (same as DepsNone).
		//
		// For each vendor FQN:
		//   1. Resolve pkg, version via opts.Packages when non-nil.
		//   2. Boundary node ID = pkg (or top-level namespace as fallback).
		//   3. Redirect all edges that touched the vendor FQN to the boundary node.
		//   4. Drop self-edges and de-dup.
		//
		// Meta on each boundary node:
		//   package       = package name (or top-level namespace)
		//   version       = version string (when available)
		//   edgeCount     = number of distinct edges from first-party nodes to this boundary
		//   distinctSymbols = number of distinct vendor FQNs collapsed into it

		// Map vendor FQN → boundary node ID.
		vendorToBoundary := map[string]string{}
		// boundaryPkg carries the resolved pkg/version for each boundary ID.
		type boundaryInfo struct {
			pkg     string
			version string
		}
		boundaryMeta := map[string]boundaryInfo{}

		// First pass: classify every node and build the vendor→boundary mapping.
		firstParty := map[string]struct{}{} // nodes that are NOT vendor/builtin
		for _, n := range nodes {
			c := classify(idx, n.ID)
			switch c {
			case "vendor":
				var pkg, version string
				if opts.Packages != nil {
					if p, v, ok := opts.Packages(n.ID); ok {
						pkg = p
						version = v
					}
				}
				if pkg == "" {
					pkg = topLevelNamespace(n.ID)
				}
				vendorToBoundary[n.ID] = pkg
				if _, exists := boundaryMeta[pkg]; !exists {
					boundaryMeta[pkg] = boundaryInfo{pkg: pkg, version: version}
				}
			case "builtin":
				// dropped — do nothing
			default:
				firstParty[n.ID] = struct{}{}
			}
		}

		// Build collapsed node list: first-party nodes + one boundary node per package.
		// Track per-boundary: distinctSymbols (vendor FQNs mapped to it) and
		// edgeCount (distinct edges from first-party to this boundary).
		boundaryDistinctSymbols := map[string]map[string]struct{}{} // boundaryID → set of vendor FQNs
		boundaryEdgeSet := map[string]map[string]struct{}{}          // boundaryID → set of first-party sources

		// Remap edges.
		type edgeKey2 struct{ from, to, kind string }
		remappedEdgeSet := map[edgeKey2]struct{}{}

		for _, e := range edges {
			fromC := classify(idx, e.From)
			toC := classify(idx, e.To)

			newFrom := e.From
			newTo := e.To

			if fromC == "builtin" || toC == "builtin" {
				continue
			}
			if fromC == "vendor" {
				newFrom = vendorToBoundary[e.From]
			}
			if toC == "vendor" {
				newTo = vendorToBoundary[e.To]
			}
			if newFrom == "" || newTo == "" {
				continue
			}
			// Drop self-edges.
			if newFrom == newTo {
				continue
			}
			k := edgeKey2{newFrom, newTo, e.Kind}
			if _, exists := remappedEdgeSet[k]; exists {
				continue
			}
			remappedEdgeSet[k] = struct{}{}

			// Count first-party → boundary edges for metadata.
			if toC == "vendor" {
				boundaryID := newTo
				if _, ok := boundaryDistinctSymbols[boundaryID]; !ok {
					boundaryDistinctSymbols[boundaryID] = map[string]struct{}{}
				}
				boundaryDistinctSymbols[boundaryID][e.To] = struct{}{}
				if _, fpNode := firstParty[e.From]; fpNode {
					if _, ok := boundaryEdgeSet[boundaryID]; !ok {
						boundaryEdgeSet[boundaryID] = map[string]struct{}{}
					}
					boundaryEdgeSet[boundaryID][e.From] = struct{}{}
				}
			}
			if fromC == "vendor" {
				boundaryID := newFrom
				if _, ok := boundaryDistinctSymbols[boundaryID]; !ok {
					boundaryDistinctSymbols[boundaryID] = map[string]struct{}{}
				}
				boundaryDistinctSymbols[boundaryID][e.From] = struct{}{}
			}
		}

		// Build final node list.
		var finalNodes []Node
		for _, n := range nodes {
			c := classify(idx, n.ID)
			if c == "vendor" || c == "builtin" {
				continue
			}
			finalNodes = append(finalNodes, n)
		}
		// Add boundary nodes.
		seenBoundary := map[string]struct{}{}
		for _, vendorFQN := range sortedKeys(vendorToBoundary) {
			boundaryID := vendorToBoundary[vendorFQN]
			if _, seen := seenBoundary[boundaryID]; seen {
				continue
			}
			seenBoundary[boundaryID] = struct{}{}

			info := boundaryMeta[boundaryID]
			meta := map[string]any{
				"package":         info.pkg,
				"edgeCount":       len(boundaryEdgeSet[boundaryID]),
				"distinctSymbols": len(boundaryDistinctSymbols[boundaryID]),
			}
			if info.version != "" {
				meta["version"] = info.version
			}
			finalNodes = append(finalNodes, Node{
				ID:    boundaryID,
				Kind:  "dependency-boundary",
				Label: boundaryID,
				Meta:  meta,
			})
		}

		var finalEdges []Edge
		for k := range remappedEdgeSet {
			finalEdges = append(finalEdges, Edge{From: k.from, To: k.to, Kind: k.kind})
		}

		g.Nodes = finalNodes
		g.Edges = finalEdges
	}

	return g
}

// sortedKeys returns the keys of a map[string]string in ascending order.
// Used to iterate deterministically when building boundary nodes.
func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
