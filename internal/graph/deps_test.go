package graph

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// buildDepsTestGraph creates a simple hand-built Graph with project, vendor, and
// builtin nodes, plus edges among them, for use in applyDeps tests.
//
// Nodes:
//   - "App\\Service"        (project)
//   - "Monolog\\Logger"     (vendor)
//   - "Monolog\\Handler"    (vendor, second vendor FQN in same package)
//   - "Psr\\Log\\LoggerInterface" (vendor, different package)
//   - "stdClass"            (builtin)
//
// Edges:
//   - App\Service → Monolog\Logger     (injects)
//   - App\Service → Monolog\Handler    (injects)
//   - App\Service → Psr\Log\LoggerInterface (injects)
//   - App\Service → stdClass           (injects)
//   - Monolog\Logger → stdClass        (injects)
func buildDepsTestGraph(idx *symbols.Index) *Graph {
	idx.IndexFileWithSource(
		"file:///project/Service.php",
		`<?php
namespace App;
class Service {}
class Controller {}
`,
		symbols.SourceProject,
	)
	idx.IndexFileWithSource(
		"file:///vendor/monolog/Logger.php",
		`<?php
namespace Monolog;
class Logger {}
`,
		symbols.SourceVendor,
	)
	idx.IndexFileWithSource(
		"file:///vendor/monolog/Handler.php",
		`<?php
namespace Monolog;
class Handler {}
`,
		symbols.SourceVendor,
	)
	idx.IndexFileWithSource(
		"file:///vendor/psr/log/LoggerInterface.php",
		`<?php
namespace Psr\Log;
interface LoggerInterface {}
`,
		symbols.SourceVendor,
	)
	idx.IndexFileWithSource(
		"file:///builtin/stdClass.php",
		`<?php
class stdClass {}
`,
		symbols.SourceBuiltin,
	)

	g := &Graph{SchemaVersion: SchemaVersion, Kind: "test"}
	g.Nodes = []Node{
		{ID: `App\Service`, Kind: "class", Label: `App\Service`},
		{ID: `App\Controller`, Kind: "class", Label: `App\Controller`},
		{ID: `Monolog\Logger`, Kind: "class", Label: `Monolog\Logger`},
		{ID: `Monolog\Handler`, Kind: "class", Label: `Monolog\Handler`},
		{ID: `Psr\Log\LoggerInterface`, Kind: "class", Label: `Psr\Log\LoggerInterface`},
		{ID: "stdClass", Kind: "class", Label: "stdClass"},
	}
	// Use distinct from-nodes for two Monolog targets so they don't collapse
	// to the same remapped edge key — ensuring distinctSymbols counts correctly.
	g.Edges = []Edge{
		{From: `App\Service`, To: `Monolog\Logger`, Kind: "injects"},
		{From: `App\Controller`, To: `Monolog\Handler`, Kind: "injects"},
		{From: `App\Service`, To: `Psr\Log\LoggerInterface`, Kind: "injects"},
		{From: `App\Service`, To: "stdClass", Kind: "injects"},
		{From: `Monolog\Logger`, To: "stdClass", Kind: "injects"},
	}
	return g
}

// TestApplyDeps_DepsFull verifies that DepsFull keeps all nodes and edges.
func TestApplyDeps_DepsFull(t *testing.T) {
	idx := symbols.NewIndex()
	g := buildDepsTestGraph(idx)

	result := applyDeps(g, idx, Options{Deps: DepsFull})

	if len(result.Nodes) != 6 {
		t.Errorf("DepsFull: want 6 nodes, got %d", len(result.Nodes))
	}
	if len(result.Edges) != 5 {
		t.Errorf("DepsFull: want 5 edges, got %d", len(result.Edges))
	}
}

// TestApplyDeps_DepsNone verifies that DepsNone drops vendor and builtin nodes
// and their edges, keeping only project (and unresolved) nodes.
func TestApplyDeps_DepsNone(t *testing.T) {
	idx := symbols.NewIndex()
	g := buildDepsTestGraph(idx)

	result := applyDeps(g, idx, Options{Deps: DepsNone})

	// Only App\Service and App\Controller (project) should remain.
	if len(result.Nodes) != 2 {
		t.Errorf("DepsNone: want 2 nodes, got %d: %v", len(result.Nodes), result.Nodes)
	}
	retained := map[string]struct{}{}
	for _, n := range result.Nodes {
		retained[n.ID] = struct{}{}
	}
	if _, ok := retained[`App\Service`]; !ok {
		t.Error("DepsNone: App\\Service missing from retained nodes")
	}
	if _, ok := retained[`App\Controller`]; !ok {
		t.Error("DepsNone: App\\Controller missing from retained nodes")
	}

	// No edges should remain (all targets were vendor or builtin).
	if len(result.Edges) != 0 {
		t.Errorf("DepsNone: want 0 edges, got %d: %v", len(result.Edges), result.Edges)
	}
}

// TestApplyDeps_DepsNone_Empty treats "" as DepsNone.
func TestApplyDeps_DepsNone_Empty(t *testing.T) {
	idx := symbols.NewIndex()
	g := buildDepsTestGraph(idx)

	resultEmpty := applyDeps(g, idx, Options{Deps: ""})

	// Rebuild to get a fresh graph for comparison.
	idx2 := symbols.NewIndex()
	g2 := buildDepsTestGraph(idx2)
	resultNone := applyDeps(g2, idx2, Options{Deps: DepsNone})

	if len(resultEmpty.Nodes) != len(resultNone.Nodes) {
		t.Errorf("empty DepsMode nodes=%d, DepsNone nodes=%d (should be equal)",
			len(resultEmpty.Nodes), len(resultNone.Nodes))
	}
	if len(resultEmpty.Edges) != len(resultNone.Edges) {
		t.Errorf("empty DepsMode edges=%d, DepsNone edges=%d (should be equal)",
			len(resultEmpty.Edges), len(resultNone.Edges))
	}
}

// TestApplyDeps_DepsBoundary verifies that DepsBoundary collapses vendor FQNs
// into "dependency-boundary" nodes, drops builtins, and produces correct Meta.
func TestApplyDeps_DepsBoundary(t *testing.T) {
	idx := symbols.NewIndex()
	g := buildDepsTestGraph(idx)

	// Stub PackageResolver: Monolog\Logger and Monolog\Handler → "monolog/monolog" 2.0.0,
	// Psr\Log\LoggerInterface → "psr/log" 1.0.0.
	resolver := func(fqn string) (string, string, bool) {
		switch fqn {
		case `Monolog\Logger`, `Monolog\Handler`:
			return "monolog/monolog", "2.0.0", true
		case `Psr\Log\LoggerInterface`:
			return "psr/log", "1.0.0", true
		}
		return "", "", false
	}

	result := applyDeps(g, idx, Options{Deps: DepsBoundary, Packages: resolver})

	// Verify no raw vendor/builtin nodes remain.
	for _, n := range result.Nodes {
		if n.Kind == "class" {
			c := classify(idx, n.ID)
			if c == "vendor" {
				t.Errorf("DepsBoundary: vendor node %q not collapsed", n.ID)
			}
			if c == "builtin" {
				t.Errorf("DepsBoundary: builtin node %q not dropped", n.ID)
			}
		}
	}

	// Find the boundary nodes.
	boundaryByID := map[string]Node{}
	for _, n := range result.Nodes {
		if n.Kind == "dependency-boundary" {
			boundaryByID[n.ID] = n
		}
	}

	// Expect two boundary nodes: "monolog/monolog" and "psr/log".
	if len(boundaryByID) != 2 {
		t.Errorf("DepsBoundary: want 2 boundary nodes, got %d: %v", len(boundaryByID), boundaryByID)
	}

	// Verify monolog/monolog boundary.
	if mono, ok := boundaryByID["monolog/monolog"]; ok {
		if mono.Meta["package"] != "monolog/monolog" {
			t.Errorf("monolog/monolog Meta[package] = %v, want %q", mono.Meta["package"], "monolog/monolog")
		}
		if mono.Meta["version"] != "2.0.0" {
			t.Errorf("monolog/monolog Meta[version] = %v, want %q", mono.Meta["version"], "2.0.0")
		}
		// distinctSymbols: Monolog\Logger and Monolog\Handler both collapsed here.
		ds, _ := mono.Meta["distinctSymbols"].(int)
		if ds != 2 {
			t.Errorf("monolog/monolog Meta[distinctSymbols] = %v, want 2", mono.Meta["distinctSymbols"])
		}
		// edgeCount: App\Service and App\Controller both connect to it (2 distinct sources).
		ec, _ := mono.Meta["edgeCount"].(int)
		if ec != 2 {
			t.Errorf("monolog/monolog Meta[edgeCount] = %v, want 2", mono.Meta["edgeCount"])
		}
	} else {
		t.Error("DepsBoundary: boundary node 'monolog/monolog' not found")
	}

	// Verify psr/log boundary.
	if psr, ok := boundaryByID["psr/log"]; ok {
		if psr.Meta["package"] != "psr/log" {
			t.Errorf("psr/log Meta[package] = %v, want %q", psr.Meta["package"], "psr/log")
		}
		if psr.Meta["version"] != "1.0.0" {
			t.Errorf("psr/log Meta[version] = %v, want %q", psr.Meta["version"], "1.0.0")
		}
		ds, _ := psr.Meta["distinctSymbols"].(int)
		if ds != 1 {
			t.Errorf("psr/log Meta[distinctSymbols] = %v, want 1", psr.Meta["distinctSymbols"])
		}
	} else {
		t.Error("DepsBoundary: boundary node 'psr/log' not found")
	}

	// stdClass (builtin) must not appear as a boundary or project node.
	for _, n := range result.Nodes {
		if n.ID == "stdClass" {
			t.Errorf("DepsBoundary: stdClass builtin node should have been dropped, got %+v", n)
		}
	}
}

// TestApplyDeps_DepsBoundary_FallbackNamespace verifies that when PackageResolver
// returns ok=false the boundary ID falls back to the top-level namespace segment.
func TestApplyDeps_DepsBoundary_FallbackNamespace(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource(
		"file:///project/Controller.php",
		`<?php
namespace App;
class Controller {}
`,
		symbols.SourceProject,
	)
	idx.IndexFileWithSource(
		"file:///vendor/acme/Client.php",
		`<?php
namespace Acme\Client;
class HttpClient {}
`,
		symbols.SourceVendor,
	)

	g := &Graph{SchemaVersion: SchemaVersion, Kind: "test"}
	g.Nodes = []Node{
		{ID: `App\Controller`, Kind: "class", Label: `App\Controller`},
		{ID: `Acme\Client\HttpClient`, Kind: "class", Label: `Acme\Client\HttpClient`},
	}
	g.Edges = []Edge{
		{From: `App\Controller`, To: `Acme\Client\HttpClient`, Kind: "injects"},
	}

	// Resolver always returns ok=false.
	resolver := func(fqn string) (string, string, bool) {
		return "", "", false
	}

	result := applyDeps(g, idx, Options{Deps: DepsBoundary, Packages: resolver})

	// Boundary node should have ID = "Acme" (top-level namespace).
	found := false
	for _, n := range result.Nodes {
		if n.Kind == "dependency-boundary" && n.ID == "Acme" {
			found = true
			if n.Meta["package"] != "Acme" {
				t.Errorf("fallback boundary Meta[package] = %v, want %q", n.Meta["package"], "Acme")
			}
			// version must not be present (no version resolved).
			if _, hasVersion := n.Meta["version"]; hasVersion {
				t.Errorf("fallback boundary should have no version Meta, got %v", n.Meta["version"])
			}
		}
	}
	if !found {
		t.Error("DepsBoundary fallback: expected boundary node 'Acme', not found")
	}
}

// TestApplyDeps_SelfEdgesDropped verifies that DepsBoundary drops self-edges
// produced when two vendor FQNs collapse into the same boundary node.
func TestApplyDeps_SelfEdgesDropped(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource(
		"file:///vendor/monolog/Logger.php",
		`<?php namespace Monolog; class Logger {}`,
		symbols.SourceVendor,
	)
	idx.IndexFileWithSource(
		"file:///vendor/monolog/Handler.php",
		`<?php namespace Monolog; class Handler {}`,
		symbols.SourceVendor,
	)

	g := &Graph{SchemaVersion: SchemaVersion, Kind: "test"}
	g.Nodes = []Node{
		{ID: `Monolog\Logger`, Kind: "class", Label: `Monolog\Logger`},
		{ID: `Monolog\Handler`, Kind: "class", Label: `Monolog\Handler`},
	}
	// Edge between two vendor FQNs that collapse to the same boundary → self-edge.
	g.Edges = []Edge{
		{From: `Monolog\Logger`, To: `Monolog\Handler`, Kind: "uses"},
	}

	resolver := func(fqn string) (string, string, bool) {
		return "monolog/monolog", "2.0.0", true
	}

	result := applyDeps(g, idx, Options{Deps: DepsBoundary, Packages: resolver})

	// The self-edge (monolog/monolog → monolog/monolog) must be dropped.
	for _, e := range result.Edges {
		if e.From == "monolog/monolog" && e.To == "monolog/monolog" {
			t.Errorf("DepsBoundary: self-edge %q → %q was not dropped", e.From, e.To)
		}
	}
}
