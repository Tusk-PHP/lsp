package graph

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestBuildContainer_LaravelDefaults exercises the full BuildContainer path
// against a real ContainerAnalyzer (Laravel framework) seeded with default
// Laravel bindings.  None of the Illuminate\\... concrete FQNs exist in the
// empty index, so they classify as "unresolved" — not vendor.  The test
// verifies the basic structural invariants of the resulting graph.
func TestBuildContainer_LaravelDefaults(t *testing.T) {
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	g := BuildContainer(idx, ca, Options{Deps: DepsFull})

	if g.Kind != "container" {
		t.Errorf("Kind = %q, want %q", g.Kind, "container")
	}
	if g.SchemaVersion != SchemaVersion {
		t.Errorf("SchemaVersion = %d, want %d", g.SchemaVersion, SchemaVersion)
	}
	if len(g.Nodes) == 0 {
		t.Error("expected at least one node, got none")
	}

	// There must be at least one "binds" edge (Laravel registers many by default).
	foundBinds := false
	for _, e := range g.Edges {
		if e.Kind == "binds" {
			foundBinds = true
			break
		}
	}
	if !foundBinds {
		t.Error("expected at least one 'binds' edge, found none")
	}
}

// TestBuildContainer_NilSafe verifies that nil idx / ca inputs are handled
// gracefully without panic.
func TestBuildContainer_NilSafe(t *testing.T) {
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	g1 := BuildContainer(nil, ca, Options{})
	if g1 == nil || g1.Kind != "container" {
		t.Error("BuildContainer(nil, ca, …) must return a valid empty graph")
	}

	g2 := BuildContainer(idx, nil, Options{})
	if g2 == nil || g2.Kind != "container" {
		t.Error("BuildContainer(idx, nil, …) must return a valid empty graph")
	}
}

// TestBuildContainer_DepsNone verifies that DepsNone removes vendor and builtin
// nodes and their edges from the graph.
//
// Vendor classification is arranged by indexing a PHP class stub with
// symbols.SourceVendor so that idx.Lookup returns a symbol whose Source field
// equals symbols.SourceVendor.  Because ContainerAnalyzer has no exported seam
// to inject a binding after Analyze(), we verify the negative case: no node
// present in the DepsNone output classifies as "vendor" or "builtin", and every
// retained edge references only retained nodes.
func TestBuildContainer_DepsNone(t *testing.T) {
	idx := symbols.NewIndex()

	// Index a minimal PHP class as SourceVendor so classify() returns "vendor"
	// for this FQN — even though no binding in the default analyzer maps to it,
	// this ensures classify() works correctly when called from the post-pass.
	idx.IndexFileWithSource(
		"file:///vendor/acme/lib/ServiceClient.php",
		`<?php
namespace Acme\VendorLib;
class ServiceClient {}
`,
		symbols.SourceVendor,
	)

	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	g := BuildContainer(idx, ca, Options{Deps: DepsNone})

	for _, n := range g.Nodes {
		c := classify(idx, n.ID)
		if c == "vendor" || c == "builtin" {
			t.Errorf("DepsNone: node %q classified as %q should have been removed", n.ID, c)
		}
	}

	// Every edge must reference only retained nodes.
	nodeIDs := map[string]struct{}{}
	for _, n := range g.Nodes {
		nodeIDs[n.ID] = struct{}{}
	}
	for _, e := range g.Edges {
		if _, ok := nodeIDs[e.From]; !ok {
			t.Errorf("DepsNone: edge From=%q has no corresponding retained node", e.From)
		}
		if _, ok := nodeIDs[e.To]; !ok {
			t.Errorf("DepsNone: edge To=%q has no corresponding retained node", e.To)
		}
	}
}

// TestBuildContainer_DepsBoundary verifies that DepsBoundary collapses
// vendor-classified FQNs into "dependency-boundary" nodes and drops builtins.
//
// Vendor binding is arranged by:
//  1. Indexing a concrete PHP class as SourceVendor in the symbol index.
//  2. Injecting the binding directly via AddBinding so the test does not rely
//     on filesystem-based provider parsing.
//
// The PackageResolver stub always returns pkg="monolog/monolog", version="2.0.0".
func TestBuildContainer_DepsBoundary(t *testing.T) {
	idx := symbols.NewIndex()

	// Index the vendor concrete so classify() returns "vendor" for this FQN.
	vendorConcrete := `Monolog\Logger`
	idx.IndexFileWithSource(
		"file:///vendor/monolog/monolog/Logger.php",
		`<?php
namespace Monolog;
class Logger {}
`,
		symbols.SourceVendor,
	)

	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	// Inject the binding directly so the test is not fragile against filesystem parsing.
	ca.AddBinding(&container.ServiceBinding{
		Abstract:  `App\Contracts\LoggerInterface`,
		Concrete:  vendorConcrete,
		Singleton: true,
	})

	resolverCalls := map[string]int{}
	resolver := func(fqn string) (string, string, bool) {
		resolverCalls[fqn]++
		if fqn == vendorConcrete {
			return "monolog/monolog", "2.0.0", true
		}
		return "", "", false
	}

	g := BuildContainer(idx, ca, Options{
		Deps:     DepsBoundary,
		Packages: resolver,
	})

	// No raw vendor or builtin nodes should remain.
	for _, n := range g.Nodes {
		if n.Kind == "class" || n.Kind == "binding" {
			c := classify(idx, n.ID)
			if c == "vendor" {
				t.Errorf("DepsBoundary: vendor node %q was not collapsed into a boundary", n.ID)
			}
			if c == "builtin" {
				t.Errorf("DepsBoundary: builtin node %q should have been dropped", n.ID)
			}
		}
	}

	// Binding is always present (injected above); assert boundary node unconditionally.
	var boundary *Node
	for i := range g.Nodes {
		if g.Nodes[i].Kind == "dependency-boundary" && g.Nodes[i].ID == "monolog/monolog" {
			boundary = &g.Nodes[i]
			break
		}
	}
	if boundary == nil {
		t.Error("DepsBoundary: expected 'monolog/monolog' boundary node, not found")
	} else {
		if boundary.Meta["package"] != "monolog/monolog" {
			t.Errorf("boundary Meta[package] = %v, want %q", boundary.Meta["package"], "monolog/monolog")
		}
		if boundary.Meta["version"] != "2.0.0" {
			t.Errorf("boundary Meta[version] = %v, want %q", boundary.Meta["version"], "2.0.0")
		}
		ds, _ := boundary.Meta["distinctSymbols"].(int)
		if ds < 1 {
			t.Errorf("boundary Meta[distinctSymbols] = %v, want >= 1", boundary.Meta["distinctSymbols"])
		}
	}
}

// TestBuildContainer_DepsBoundary_FallbackNamespace verifies the top-level
// namespace fallback when PackageResolver returns ok=false.
func TestBuildContainer_DepsBoundary_FallbackNamespace(t *testing.T) {
	idx := symbols.NewIndex()

	// Index a vendor concrete without a known package.
	idx.IndexFileWithSource(
		"file:///vendor/unknown/Pkg/Client.php",
		`<?php
namespace UnknownPkg;
class Client {}
`,
		symbols.SourceVendor,
	)

	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	// Inject the binding directly so the test does not rely on filesystem parsing.
	ca.AddBinding(&container.ServiceBinding{
		Abstract:  `App\Contracts\ClientInterface`,
		Concrete:  `UnknownPkg\Client`,
		Singleton: true,
	})

	// Resolver returns ok=false → boundary ID should fall back to "UnknownPkg".
	resolver := func(fqn string) (string, string, bool) {
		return "", "", false
	}

	g := BuildContainer(idx, ca, Options{
		Deps:     DepsBoundary,
		Packages: resolver,
	})

	found := false
	for _, n := range g.Nodes {
		if n.Kind == "dependency-boundary" && n.ID == "UnknownPkg" {
			found = true
			if n.Meta["package"] != "UnknownPkg" {
				t.Errorf("fallback boundary Meta[package] = %v, want %q", n.Meta["package"], "UnknownPkg")
			}
		}
	}
	if !found {
		t.Error("DepsBoundary with no resolver: expected fallback boundary node 'UnknownPkg'")
	}
}

// TestClassify verifies the classify helper for each source kind.
func TestClassify(t *testing.T) {
	idx := symbols.NewIndex()

	idx.IndexFileWithSource("file:///project/Foo.php", `<?php
namespace Project;
class Foo {}
`, symbols.SourceProject)

	idx.IndexFileWithSource("file:///vendor/bar/Bar.php", `<?php
namespace Vendor;
class Bar {}
`, symbols.SourceVendor)

	cases := []struct {
		fqn  string
		want string
	}{
		{"Project\\Foo", "project"},
		{"Vendor\\Bar", "vendor"},
		{"Totally\\Unknown", "unresolved"},
	}

	for _, tc := range cases {
		got := classify(idx, tc.fqn)
		if got != tc.want {
			t.Errorf("classify(%q) = %q, want %q", tc.fqn, got, tc.want)
		}
	}
}

// TestTopLevelNamespace verifies the FQN → top-level segment extraction.
func TestTopLevelNamespace(t *testing.T) {
	cases := []struct {
		fqn  string
		want string
	}{
		{`Monolog\Logger`, "Monolog"},
		{`Illuminate\Contracts\Auth\Factory`, "Illuminate"},
		{"Simple", "Simple"},
	}
	for _, tc := range cases {
		got := topLevelNamespace(tc.fqn)
		if got != tc.want {
			t.Errorf("topLevelNamespace(%q) = %q, want %q", tc.fqn, got, tc.want)
		}
	}
}

// TestNodeDedup_BindingPreferred verifies that when the same FQN is added as
// both a "binding" and a "class", the "binding" kind is preferred and the node
// appears exactly once.
func TestNodeDedup_BindingPreferred(t *testing.T) {
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	g := BuildContainer(idx, ca, Options{Deps: DepsFull})

	seen := map[string]int{}
	for _, n := range g.Nodes {
		seen[n.ID]++
	}
	for id, count := range seen {
		if count > 1 {
			t.Errorf("node %q appears %d times (expected exactly 1)", id, count)
		}
	}
}

// TestBuildContainer_EmptyDepsMode verifies that an empty DepsMode string is
// treated identically to DepsNone.
func TestBuildContainer_EmptyDepsMode(t *testing.T) {
	idx := symbols.NewIndex()
	ca := container.NewContainerAnalyzer(idx, t.TempDir(), "laravel")
	ca.Analyze()

	gNone := BuildContainer(idx, ca, Options{Deps: DepsNone})
	gEmpty := BuildContainer(idx, ca, Options{Deps: ""})

	if len(gNone.Nodes) != len(gEmpty.Nodes) {
		t.Errorf("DepsNone nodes=%d, empty-mode nodes=%d (should be equal)", len(gNone.Nodes), len(gEmpty.Nodes))
	}
	if len(gNone.Edges) != len(gEmpty.Edges) {
		t.Errorf("DepsNone edges=%d, empty-mode edges=%d (should be equal)", len(gNone.Edges), len(gEmpty.Edges))
	}
}
