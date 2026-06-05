package graph

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// buildReferencesTestIndex creates a minimal symbol index for reference-graph tests.
//
// It indexes several PHP class definitions in the App namespace so that FQNs
// resolve correctly without relying on filesystem fixtures.
//
//	App\A  extends App\B implements App\I
//	         property: public App\C $c
//	         method m(App\D $d): App\E
//	         method make(): void  [body contains new F()]
//	App\B  (plain class)
//	App\I  (interface)
//	App\C  (plain class)
//	App\D  (plain class)
//	App\E  (plain class)
//	App\F  (plain class) — target of new
//	App\T  (trait used by A)
//	App\VendorClass — indexed as SourceVendor
func buildReferencesTestIndex(t *testing.T) *symbols.Index {
	t.Helper()
	idx := symbols.NewIndex()

	// Core project classes and the class-under-test (A).
	idx.IndexFileWithSource("file:///t/main.php", `<?php
namespace App;

use App\Dep\D;
use App\Dep\E;

trait T {}

interface I {}

class B {}

class C {}

class F {}

class A extends B implements I {
    use T;
    public C $c;
    public function m(D $d): E {}
    public function make(): void { $x = new F(); }
    public function typed(int $n, string $s): void {}
}
`, symbols.SourceProject)

	// D and E live in a sub-namespace to exercise use-import resolution.
	idx.IndexFileWithSource("file:///t/dep.php", `<?php
namespace App\Dep;

class D {}
class E {}
`, symbols.SourceProject)

	// Vendor class — should be excluded under DepsNone.
	idx.IndexFileWithSource("file:///t/vendor/pkg/VendorClass.php", `<?php
namespace Vendor\Pkg;
class VendorClass {}
`, symbols.SourceVendor)

	// Project class that references the vendor class via property.
	idx.IndexFileWithSource("file:///t/uses_vendor.php", `<?php
namespace App;

use Vendor\Pkg\VendorClass;

class UsesVendor {
    public VendorClass $dep;
}
`, symbols.SourceProject)

	return idx
}

// findEdge returns true if an edge matching (from, to, kind) exists in g.
func findEdge(g *Graph, from, to, kind string) bool {
	for _, e := range g.Edges {
		if e.From == from && e.To == to && e.Kind == kind {
			return true
		}
	}
	return false
}

// findNode returns true if a node with id exists in g.
func findNode(g *Graph, id string) bool {
	for _, n := range g.Nodes {
		if n.ID == id {
			return true
		}
	}
	return false
}

// TestBuildReferences_NilSafe verifies that a nil index returns an empty graph
// without panicking.
func TestBuildReferences_NilSafe(t *testing.T) {
	g := BuildReferences(nil, Options{})
	if g == nil {
		t.Fatal("BuildReferences(nil) must not return nil")
	}
	if g.Kind != "references" {
		t.Errorf("Kind = %q, want %q", g.Kind, "references")
	}
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Errorf("empty graph from nil idx: got %d nodes, %d edges", len(g.Nodes), len(g.Edges))
	}
}

// TestBuildReferences_Kind verifies the graph Kind field.
func TestBuildReferences_Kind(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})
	if g.Kind != "references" {
		t.Errorf("Kind = %q, want %q", g.Kind, "references")
	}
	if g.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", g.SchemaVersion, SchemaVersion)
	}
}

// TestBuildReferences_ExtendsEdge verifies that class inheritance is captured.
func TestBuildReferences_ExtendsEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	if !findEdge(g, `App\A`, `App\B`, "extends") {
		t.Errorf("expected extends edge App\\A → App\\B, not found in %v", g.Edges)
	}
}

// TestBuildReferences_ImplementsEdge verifies that interface implementation is captured.
func TestBuildReferences_ImplementsEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	if !findEdge(g, `App\A`, `App\I`, "implements") {
		t.Errorf("expected implements edge App\\A → App\\I, not found in %v", g.Edges)
	}
}

// TestBuildReferences_PropertyEdge verifies that property type references are captured.
func TestBuildReferences_PropertyEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	if !findEdge(g, `App\A`, `App\C`, "property") {
		t.Errorf("expected property edge App\\A → App\\C, not found in %v", g.Edges)
	}
}

// TestBuildReferences_ParamEdge verifies that method parameter type references are captured.
func TestBuildReferences_ParamEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	// D is imported via 'use App\Dep\D' so it resolves to App\Dep\D.
	if !findEdge(g, `App\A`, `App\Dep\D`, "param") {
		t.Errorf("expected param edge App\\A → App\\Dep\\D, not found in %v", g.Edges)
	}
}

// TestBuildReferences_ReturnsEdge verifies that method return type references are captured.
func TestBuildReferences_ReturnsEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	if !findEdge(g, `App\A`, `App\Dep\E`, "returns") {
		t.Errorf("expected returns edge App\\A → App\\Dep\\E, not found in %v", g.Edges)
	}
}

// TestBuildReferences_UsesEdge verifies that trait inclusion is captured.
func TestBuildReferences_UsesEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	if !findEdge(g, `App\A`, `App\T`, "uses") {
		t.Errorf("expected uses edge App\\A → App\\T, not found in %v", g.Edges)
	}
}

// TestBuildReferences_NewEdge verifies that 'new ClassName' is captured as a
// "new" edge attributed to the enclosing class.
func TestBuildReferences_NewEdge(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	if !findEdge(g, `App\A`, `App\F`, "new") {
		t.Errorf("expected new edge App\\A → App\\F, not found in %v", g.Edges)
	}
}

// TestBuildReferences_BuiltinsNotTargets verifies that PHP built-in types
// (int, string, void, bool) never appear as edge targets.
func TestBuildReferences_BuiltinsNotTargets(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	builtins := []string{"int", "string", "void", "bool", "float", "null", "array",
		"mixed", "object", "callable", "iterable", "never", "false", "true",
		"self", "static", "parent", "resource"}

	for _, b := range builtins {
		if findNode(g, b) {
			t.Errorf("builtin type %q should not appear as a graph node", b)
		}
		for _, e := range g.Edges {
			if e.To == b {
				t.Errorf("builtin type %q should not appear as an edge target (from=%q, kind=%q)", b, e.From, e.Kind)
			}
		}
	}
}

// TestBuildReferences_DepsNoneDropsVendor verifies that DepsNone removes vendor
// nodes and any edges that reference them.
func TestBuildReferences_DepsNoneDropsVendor(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsNone})

	// Vendor node must not appear.
	if findNode(g, `Vendor\Pkg\VendorClass`) {
		t.Error("DepsNone: vendor node Vendor\\Pkg\\VendorClass should have been dropped")
	}

	// No edge may reference the vendor class.
	for _, e := range g.Edges {
		if e.From == `Vendor\Pkg\VendorClass` || e.To == `Vendor\Pkg\VendorClass` {
			t.Errorf("DepsNone: edge referencing vendor class survived: %+v", e)
		}
	}
}

// TestBuildReferences_DepsFull_VendorKept verifies that DepsFull retains vendor
// nodes and their edges.
func TestBuildReferences_DepsFull_VendorKept(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	// The vendor node itself should be present.
	if !findNode(g, `Vendor\Pkg\VendorClass`) {
		t.Error("DepsFull: vendor node Vendor\\Pkg\\VendorClass should be present")
	}

	// And the property edge from App\UsesVendor to it.
	if !findEdge(g, `App\UsesVendor`, `Vendor\Pkg\VendorClass`, "property") {
		t.Errorf("DepsFull: expected property edge App\\UsesVendor → Vendor\\Pkg\\VendorClass, not found in %v", g.Edges)
	}
}

// TestBuildReferences_NodeDedup verifies that each FQN appears at most once in
// the node list.
func TestBuildReferences_NodeDedup(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	seen := map[string]int{}
	for _, n := range g.Nodes {
		seen[n.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("node %q appears %d times, want exactly 1", id, count)
		}
	}
}

// TestBuildReferences_EdgeDedup verifies that (from, to, kind) tuples are unique.
func TestBuildReferences_EdgeDedup(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	type key struct{ from, to, kind string }
	seen := map[key]int{}
	for _, e := range g.Edges {
		k := key{e.From, e.To, e.Kind}
		seen[k]++
	}
	for k, count := range seen {
		if count > 1 {
			t.Errorf("edge (%q,%q,%q) appears %d times, want exactly 1", k.from, k.to, k.kind, count)
		}
	}
}

// TestBuildReferences_Sorted verifies that BuildReferences returns a sorted graph.
func TestBuildReferences_Sorted(t *testing.T) {
	idx := buildReferencesTestIndex(t)
	g := BuildReferences(idx, Options{Deps: DepsFull})

	for i := 1; i < len(g.Nodes); i++ {
		if g.Nodes[i].ID < g.Nodes[i-1].ID {
			t.Errorf("nodes not sorted at index %d: %q < %q", i, g.Nodes[i].ID, g.Nodes[i-1].ID)
		}
	}
	for i := 1; i < len(g.Edges); i++ {
		a, b := g.Edges[i-1], g.Edges[i]
		// (from, to, kind) ascending
		less := a.From < b.From ||
			(a.From == b.From && a.To < b.To) ||
			(a.From == b.From && a.To == b.To && a.Kind < b.Kind)
		if !less && !(a.From == b.From && a.To == b.To && a.Kind == b.Kind) {
			t.Errorf("edges not sorted at index %d: (%q,%q,%q) after (%q,%q,%q)",
				i, b.From, b.To, b.Kind, a.From, a.To, a.Kind)
		}
	}
}
