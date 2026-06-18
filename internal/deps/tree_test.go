package deps

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer/lockfile"
	"github.com/Tusk-PHP/lsp/internal/graph"
)

// writeComposerLock writes a composer.lock JSON fixture to a temp directory
// and returns the directory path and the loaded *lockfile.Lockfile.
func writeComposerLock(t *testing.T, packages []map[string]any) (*lockfile.Lockfile, string) {
	t.Helper()
	dir := t.TempDir()

	raw := map[string]any{
		"packages":     packages,
		"packages-dev": []any{},
	}
	data, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("marshal composer.lock: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "composer.lock"), data, 0o644); err != nil {
		t.Fatalf("write composer.lock: %v", err)
	}
	lf, err := lockfile.Load(dir)
	if err != nil {
		t.Fatalf("lockfile.Load: %v", err)
	}
	if lf == nil {
		t.Fatal("lockfile.Load returned nil unexpectedly")
	}
	return lf, dir
}

// nodeByID returns the node with the given ID, or nil when absent.
func nodeByID(g *graph.Graph, id string) *graph.Node {
	for i := range g.Nodes {
		if g.Nodes[i].ID == id {
			return &g.Nodes[i]
		}
	}
	return nil
}

// hasEdge reports whether the graph contains an edge From→To with Kind="requires".
func hasEdge(g *graph.Graph, from, to string) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == "requires" {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Basic graph: A → B (+ php platform req), B → C, C → nothing
// ---------------------------------------------------------------------------

func buildBasicLockfile(t *testing.T) *lockfile.Lockfile {
	t.Helper()
	lf, _ := writeComposerLock(t, []map[string]any{
		{
			"name":    "vendor/a",
			"version": "1.0.0",
			"require": map[string]any{
				"vendor/b": "^2.0",
				"php":      ">=7.4",
			},
		},
		{
			"name":    "vendor/b",
			"version": "2.1.0",
			"require": map[string]any{
				"vendor/c": "^1.0",
				"ext-json": "*",
			},
		},
		{
			"name":    "vendor/c",
			"version": "1.5.0",
			"require": map[string]any{},
		},
	})
	return lf
}

// TestBuildTree_BasicNodeKindsAndLabels checks that all three packages are
// emitted with Kind="package" and the expected "name@version" labels.
func TestBuildTree_BasicNodeKindsAndLabels(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	if g.Kind != "deps-tree" {
		t.Errorf("graph Kind = %q, want %q", g.Kind, "deps-tree")
	}
	if g.SchemaVersion != graph.SchemaVersion {
		t.Errorf("graph SchemaVersion = %d, want %d", g.SchemaVersion, graph.SchemaVersion)
	}

	cases := []struct{ id, label string }{
		{"vendor/a", "vendor/a@1.0.0"},
		{"vendor/b", "vendor/b@2.1.0"},
		{"vendor/c", "vendor/c@1.5.0"},
	}
	for _, tc := range cases {
		n := nodeByID(g, tc.id)
		if n == nil {
			t.Errorf("node %q not found", tc.id)
			continue
		}
		if n.Kind != "package" {
			t.Errorf("node %q Kind = %q, want %q", tc.id, n.Kind, "package")
		}
		if n.Label != tc.label {
			t.Errorf("node %q Label = %q, want %q", tc.id, n.Label, tc.label)
		}
	}
}

// TestBuildTree_DirectFlag checks that direct packages have Meta["direct"]=true
// and transitive packages have Meta["direct"]=false.
func TestBuildTree_DirectFlag(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	directNode := nodeByID(g, "vendor/a")
	if directNode == nil {
		t.Fatal("node vendor/a not found")
	}
	if d, _ := directNode.Meta["direct"].(bool); !d {
		t.Errorf("vendor/a Meta[direct] = %v, want true", directNode.Meta["direct"])
	}

	for _, id := range []string{"vendor/b", "vendor/c"} {
		n := nodeByID(g, id)
		if n == nil {
			t.Fatalf("node %q not found", id)
		}
		if d, _ := n.Meta["direct"].(bool); d {
			t.Errorf("%q Meta[direct] = true, want false (transitive)", id)
		}
	}
}

// TestBuildTree_DepthValues checks that Meta["depth"] reflects the BFS depth.
func TestBuildTree_DepthValues(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	wantDepths := map[string]int{
		"vendor/a": 0,
		"vendor/b": 1,
		"vendor/c": 2,
	}
	for id, wantDepth := range wantDepths {
		n := nodeByID(g, id)
		if n == nil {
			t.Fatalf("node %q not found", id)
		}
		d, _ := n.Meta["depth"].(int)
		if d != wantDepth {
			t.Errorf("node %q depth = %d, want %d", id, d, wantDepth)
		}
	}
}

// TestBuildTree_PlatformReqsSkipped checks that php and ext-* requires are not
// emitted as edges or nodes.
func TestBuildTree_PlatformReqsSkipped(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	// "php" and "ext-json" must not appear as nodes.
	for _, bad := range []string{"php", "ext-json"} {
		if n := nodeByID(g, bad); n != nil {
			t.Errorf("platform node %q should not be in the graph", bad)
		}
	}
	// No edges targeting them either.
	for _, e := range g.Edges {
		if e.To == "php" || e.To == "ext-json" {
			t.Errorf("unexpected edge to platform req %q", e.To)
		}
	}
}

// TestBuildTree_Edges checks that the expected requires edges are present.
func TestBuildTree_Edges(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	if !hasEdge(g, "vendor/a", "vendor/b") {
		t.Error("expected edge vendor/a → vendor/b")
	}
	if !hasEdge(g, "vendor/b", "vendor/c") {
		t.Error("expected edge vendor/b → vendor/c")
	}
}

// ---------------------------------------------------------------------------
// Depth bounding
// ---------------------------------------------------------------------------

// TestBuildTree_Depth0 checks that Depth=0 emits only the direct node with no
// child edges or child nodes (the direct node itself is still emitted).
func TestBuildTree_Depth0(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: 0})

	// vendor/a must be present.
	if nodeByID(g, "vendor/a") == nil {
		t.Error("direct node vendor/a missing for Depth=0")
	}
	// Children must not be present (no expansion at depth 0).
	for _, id := range []string{"vendor/b", "vendor/c"} {
		if nodeByID(g, id) != nil {
			t.Errorf("transitive node %q should not appear with Depth=0", id)
		}
	}
	// No edges from vendor/a.
	if len(g.Edges) != 0 {
		t.Errorf("expected 0 edges for Depth=0, got %d: %v", len(g.Edges), g.Edges)
	}
}

// TestBuildTree_Depth1 checks that Depth=1 expands direct packages but does
// not expand their children. The boundary node (vendor/c at depth 2) must NOT
// be emitted because vendor/b at depth 1 is not expanded and therefore its
// children are never visited.
func TestBuildTree_Depth1(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: 1})

	// vendor/a (depth 0) and vendor/b (depth 1) must be present.
	if nodeByID(g, "vendor/a") == nil {
		t.Error("node vendor/a missing for Depth=1")
	}
	if nodeByID(g, "vendor/b") == nil {
		t.Error("node vendor/b missing for Depth=1 (should be emitted as boundary)")
	}
	// vendor/c is at depth 2, beyond Depth=1; it must not appear.
	if nodeByID(g, "vendor/c") != nil {
		t.Error("node vendor/c should not appear with Depth=1")
	}
	// Edge A→B must exist.
	if !hasEdge(g, "vendor/a", "vendor/b") {
		t.Error("edge vendor/a → vendor/b missing for Depth=1")
	}
	// Edge B→C must NOT exist (B is not expanded).
	if hasEdge(g, "vendor/b", "vendor/c") {
		t.Error("edge vendor/b → vendor/c should not exist for Depth=1")
	}
}

// TestBuildTree_Full checks that Full=true expands everything regardless of
// Depth, reaching vendor/c.
func TestBuildTree_Full(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: 0, Full: true})

	for _, id := range []string{"vendor/a", "vendor/b", "vendor/c"} {
		if nodeByID(g, id) == nil {
			t.Errorf("node %q missing with Full=true", id)
		}
	}
	if !hasEdge(g, "vendor/a", "vendor/b") {
		t.Error("edge vendor/a → vendor/b missing with Full=true")
	}
	if !hasEdge(g, "vendor/b", "vendor/c") {
		t.Error("edge vendor/b → vendor/c missing with Full=true")
	}
}

// ---------------------------------------------------------------------------
// Cycle detection
// ---------------------------------------------------------------------------

// buildCycleLockfile builds A ↔ B cycle: A requires B, B requires A.
func buildCycleLockfile(t *testing.T) *lockfile.Lockfile {
	t.Helper()
	lf, _ := writeComposerLock(t, []map[string]any{
		{
			"name":    "vendor/a",
			"version": "1.0.0",
			"require": map[string]any{
				"vendor/b": "^1.0",
			},
		},
		{
			"name":    "vendor/b",
			"version": "1.0.0",
			"require": map[string]any{
				"vendor/a": "^1.0",
			},
		},
	})
	return lf
}

// TestBuildTree_CycleTerminates checks that a cycle between A and B does not
// cause infinite traversal, and that both nodes and the edges are emitted.
func TestBuildTree_CycleTerminates(t *testing.T) {
	lf := buildCycleLockfile(t)
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	if nodeByID(g, "vendor/a") == nil {
		t.Error("node vendor/a missing in cycle graph")
	}
	if nodeByID(g, "vendor/b") == nil {
		t.Error("node vendor/b missing in cycle graph")
	}
	// At least the A→B edge must be present. The B→A back-edge may or may not
	// appear depending on visit order, but the graph must be finite.
	if !hasEdge(g, "vendor/a", "vendor/b") {
		t.Error("edge vendor/a → vendor/b missing in cycle graph")
	}
	// The graph should have at most 2 nodes and at most 2 edges.
	if len(g.Nodes) > 2 {
		t.Errorf("cycle graph has %d nodes, want ≤2", len(g.Nodes))
	}
	if len(g.Edges) > 2 {
		t.Errorf("cycle graph has %d edges, want ≤2", len(g.Edges))
	}
}

// ---------------------------------------------------------------------------
// Missing-package node
// ---------------------------------------------------------------------------

// TestBuildTree_MissingPackageNode checks that a package referenced by a
// lockfile entry but absent from the lockfile itself is still emitted as a
// node with Meta["missing"]=true.
func TestBuildTree_MissingPackageNode(t *testing.T) {
	// Only vendor/a is in the lockfile; vendor/b is required but absent.
	lf, _ := writeComposerLock(t, []map[string]any{
		{
			"name":    "vendor/a",
			"version": "1.0.0",
			"require": map[string]any{
				"vendor/b": "^2.0",
			},
		},
	})
	g := BuildTree(lf, []string{"vendor/a"}, TreeOptions{Depth: -1})

	n := nodeByID(g, "vendor/b")
	if n == nil {
		t.Fatal("missing package vendor/b should still be emitted as a node")
	}
	if missing, _ := n.Meta["missing"].(bool); !missing {
		t.Errorf("vendor/b Meta[missing] = %v, want true", n.Meta["missing"])
	}
	// The edge must still exist.
	if !hasEdge(g, "vendor/a", "vendor/b") {
		t.Error("edge vendor/a → vendor/b missing for missing-package case")
	}
}

// ---------------------------------------------------------------------------
// Nil / empty guard
// ---------------------------------------------------------------------------

// TestBuildTree_NilLockfile checks that a nil lockfile returns an empty graph
// with Kind set correctly, without panicking.
func TestBuildTree_NilLockfile(t *testing.T) {
	g := BuildTree(nil, []string{"vendor/a"}, TreeOptions{Depth: -1})
	if g == nil {
		t.Fatal("BuildTree returned nil for nil lockfile")
	}
	if g.Kind != "deps-tree" {
		t.Errorf("Kind = %q, want %q", g.Kind, "deps-tree")
	}
	if len(g.Nodes) != 0 {
		t.Errorf("expected 0 nodes for nil lockfile, got %d", len(g.Nodes))
	}
}

// TestBuildTree_EmptyDirect checks that an empty direct list returns a graph
// with no nodes or edges.
func TestBuildTree_EmptyDirect(t *testing.T) {
	lf := buildBasicLockfile(t)
	g := BuildTree(lf, []string{}, TreeOptions{Depth: -1})
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("expected empty graph, got %d nodes / %d edges", len(g.Nodes), len(g.Edges))
	}
}

// ---------------------------------------------------------------------------
// Determinism
// ---------------------------------------------------------------------------

// TestBuildTree_Determinism checks that calling BuildTree twice on identical
// inputs produces byte-identical JSON output.
func TestBuildTree_Determinism(t *testing.T) {
	lf := buildBasicLockfile(t)
	opts := TreeOptions{Depth: -1}
	direct := []string{"vendor/a"}

	g1 := BuildTree(lf, direct, opts)
	g2 := BuildTree(lf, direct, opts)

	// Compare node IDs and edge tuples after Sort (Sort is called inside BuildTree).
	if len(g1.Nodes) != len(g2.Nodes) {
		t.Fatalf("node count mismatch: %d vs %d", len(g1.Nodes), len(g2.Nodes))
	}
	for i := range g1.Nodes {
		if g1.Nodes[i].ID != g2.Nodes[i].ID {
			t.Errorf("Nodes[%d].ID: %q vs %q", i, g1.Nodes[i].ID, g2.Nodes[i].ID)
		}
	}
	if len(g1.Edges) != len(g2.Edges) {
		t.Fatalf("edge count mismatch: %d vs %d", len(g1.Edges), len(g2.Edges))
	}
	for i := range g1.Edges {
		e1, e2 := g1.Edges[i], g2.Edges[i]
		if e1.From != e2.From || e1.To != e2.To || e1.Kind != e2.Kind {
			t.Errorf("Edges[%d] mismatch", i)
		}
	}
}

// ---------------------------------------------------------------------------
// isPlatformRequirement unit tests
// ---------------------------------------------------------------------------

func TestIsPlatformRequirement(t *testing.T) {
	cases := []struct {
		name string
		want bool
	}{
		{"php", true},
		{"PHP", true},
		{"ext-json", true},
		{"ext-mbstring", true},
		{"lib-curl", true},
		{"noslash", true},
		{"vendor/package", false},
		{"psr/log", false},
		{"symfony/console", false},
	}
	for _, tc := range cases {
		got := isPlatformRequirement(tc.name)
		if got != tc.want {
			t.Errorf("isPlatformRequirement(%q) = %v, want %v", tc.name, got, tc.want)
		}
	}
}
