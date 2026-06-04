package graph

import (
	"strings"
	"testing"
)

// buildRenderTestGraph constructs a small Graph with all three node kinds and
// both edge kinds, using PHP FQNs that contain backslashes and colons to
// exercise sanitization/escaping in the renderers.
func buildRenderTestGraph() *Graph {
	return &Graph{
		SchemaVersion: SchemaVersion,
		Kind:          "container",
		Nodes: []Node{
			{
				ID:    `App\Service`,
				Kind:  "class",
				Label: `App\Service`,
			},
			{
				ID:    `Illuminate\Contracts\Cache\Factory`,
				Kind:  "binding",
				Label: `Illuminate\Contracts\Cache\Factory`,
			},
			{
				ID:    `vendor/illuminate`,
				Kind:  "dependency-boundary",
				Label: `vendor/illuminate`,
			},
		},
		Edges: []Edge{
			{From: `Illuminate\Contracts\Cache\Factory`, To: `App\Service`, Kind: "binds"},
			{From: `App\Service`, To: `vendor/illuminate`, Kind: "injects"},
		},
	}
}

// TestRenderMermaid_Header checks that the output begins with "graph LR".
func TestRenderMermaid_Header(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderMermaid(g)
	if !strings.HasPrefix(out, "graph LR\n") {
		t.Errorf("RenderMermaid output does not start with 'graph LR\\n', got:\n%s", out)
	}
}

// TestRenderMermaid_NodeLabels checks that every node label appears in the output.
func TestRenderMermaid_NodeLabels(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderMermaid(g)

	for _, n := range g.Nodes {
		label := n.Label
		if label == "" {
			label = n.ID
		}
		// Labels may have " escaped to #quot; inside Mermaid, but the raw
		// backslash-containing FQN text should be present.
		if !strings.Contains(out, label) {
			t.Errorf("RenderMermaid: label %q not found in output:\n%s", label, out)
		}
	}
}

// TestRenderMermaid_EdgeKinds checks that each edge kind appears as a Mermaid
// edge label (-->|kind|).
func TestRenderMermaid_EdgeKinds(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderMermaid(g)

	for _, e := range g.Edges {
		needle := "-->|" + e.Kind + "|"
		if !strings.Contains(out, needle) {
			t.Errorf("RenderMermaid: edge kind marker %q not found in output:\n%s", needle, out)
		}
	}
}

// TestRenderMermaid_Shapes checks that all three shape syntaxes are present.
func TestRenderMermaid_Shapes(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderMermaid(g)

	// class  => ["..."]
	if !strings.Contains(out, `["`) {
		t.Errorf("RenderMermaid: class shape '[\"' not found in output:\n%s", out)
	}
	// binding => (["..."])
	if !strings.Contains(out, `(["`) {
		t.Errorf("RenderMermaid: binding shape '([\"' not found in output:\n%s", out)
	}
	// dependency-boundary => [["..."]]
	if !strings.Contains(out, `[["`) {
		t.Errorf("RenderMermaid: dependency-boundary shape '[[\"' not found in output:\n%s", out)
	}
}

// TestRenderMermaid_Determinism checks that rendering the same graph twice
// produces byte-identical output.
func TestRenderMermaid_Determinism(t *testing.T) {
	g1 := buildRenderTestGraph()
	g2 := buildRenderTestGraph()
	out1 := RenderMermaid(g1)
	out2 := RenderMermaid(g2)
	if out1 != out2 {
		t.Errorf("RenderMermaid: non-deterministic output.\nFirst:\n%s\nSecond:\n%s", out1, out2)
	}
}

// TestRenderMermaid_NilGraph checks that a nil graph returns a header-only
// string without panicking.
func TestRenderMermaid_NilGraph(t *testing.T) {
	out := RenderMermaid(nil)
	if out != "graph LR\n" {
		t.Errorf("RenderMermaid(nil) = %q, want %q", out, "graph LR\n")
	}
}

// TestRenderMermaid_EmptyGraph checks that an empty (non-nil) graph returns a
// header-only string without panicking.
func TestRenderMermaid_EmptyGraph(t *testing.T) {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "container"}
	out := RenderMermaid(g)
	if !strings.HasPrefix(out, "graph LR\n") {
		t.Errorf("RenderMermaid empty: expected header prefix, got:\n%s", out)
	}
	// Must contain no node lines beyond the header.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 1 {
		t.Errorf("RenderMermaid empty: expected 1 line (header), got %d:\n%s", len(lines), out)
	}
}

// TestRenderMermaid_FQNSpecialChars checks that FQNs with backslashes and
// colons appear correctly in labels (not breaking Mermaid syntax).
func TestRenderMermaid_FQNSpecialChars(t *testing.T) {
	g := &Graph{
		SchemaVersion: SchemaVersion,
		Kind:          "container",
		Nodes: []Node{
			{ID: `App\Service::method`, Kind: "class", Label: `App\Service::method`},
		},
	}
	out := RenderMermaid(g)
	// The label text must appear inside a quoted form.
	if !strings.Contains(out, `App\Service::method`) {
		t.Errorf("RenderMermaid: FQN with special chars not in output:\n%s", out)
	}
}

// ---------------------------------------------------------------------------
// DOT renderer tests
// ---------------------------------------------------------------------------

// TestRenderDOT_Header checks that the output contains "digraph".
func TestRenderDOT_Header(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderDOT(g)
	if !strings.Contains(out, "digraph") {
		t.Errorf("RenderDOT output does not contain 'digraph', got:\n%s", out)
	}
}

// TestRenderDOT_NodesPresent checks that every node ID and label appears in the
// DOT output (properly escaped).
func TestRenderDOT_NodesPresent(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderDOT(g)

	for _, n := range g.Nodes {
		// Escape as the renderer would to build the expected needle.
		escapedID := escapeDOTString(n.ID)
		if !strings.Contains(out, escapedID) {
			t.Errorf("RenderDOT: node id %q (escaped: %q) not found in output:\n%s",
				n.ID, escapedID, out)
		}
	}
}

// TestRenderDOT_EdgesPresent checks that every edge (from -> to with label)
// appears in the DOT output.
func TestRenderDOT_EdgesPresent(t *testing.T) {
	g := buildRenderTestGraph()
	out := RenderDOT(g)

	for _, e := range g.Edges {
		// The label= attribute should contain the edge kind.
		needle := `label="` + escapeDOTString(e.Kind) + `"`
		if !strings.Contains(out, needle) {
			t.Errorf("RenderDOT: edge label %q not found in output:\n%s", needle, out)
		}
	}
}

// TestRenderDOT_BackslashEscape checks that backslashes in FQNs are doubled in
// the DOT output so they do not break DOT label parsing.
func TestRenderDOT_BackslashEscape(t *testing.T) {
	g := &Graph{
		SchemaVersion: SchemaVersion,
		Kind:          "container",
		Nodes: []Node{
			{ID: `App\Service`, Kind: "class", Label: `App\Service`},
		},
	}
	out := RenderDOT(g)
	// The DOT output must have the escaped form (double backslash).
	if !strings.Contains(out, `App\\Service`) {
		t.Errorf("RenderDOT: backslash not escaped in output:\n%s", out)
	}
	// There must be no bare (unescaped) backslash inside a quoted string.
	// We check that every backslash in the output is preceded by another backslash.
	for i := 0; i < len(out); i++ {
		if out[i] == '\\' {
			if i+1 >= len(out) || out[i+1] != '\\' {
				t.Errorf("RenderDOT: found lone backslash at position %d in output:\n%s", i, out)
				break
			}
			i++ // skip the second backslash of the pair
		}
	}
}

// TestRenderDOT_QuoteEscape checks that double-quotes inside labels are escaped.
func TestRenderDOT_QuoteEscape(t *testing.T) {
	g := &Graph{
		SchemaVersion: SchemaVersion,
		Kind:          "container",
		Nodes: []Node{
			{ID: `App\Foo`, Kind: "class", Label: `Say "hello"`},
		},
	}
	out := RenderDOT(g)
	// The escaped form must appear; the raw form must not break the attribute.
	if !strings.Contains(out, `Say \"hello\"`) {
		t.Errorf("RenderDOT: quotes not escaped in label, output:\n%s", out)
	}
}

// TestRenderDOT_Determinism checks that rendering the same graph twice produces
// byte-identical output.
func TestRenderDOT_Determinism(t *testing.T) {
	g1 := buildRenderTestGraph()
	g2 := buildRenderTestGraph()
	out1 := RenderDOT(g1)
	out2 := RenderDOT(g2)
	if out1 != out2 {
		t.Errorf("RenderDOT: non-deterministic output.\nFirst:\n%s\nSecond:\n%s", out1, out2)
	}
}

// TestRenderDOT_NilGraph checks that a nil graph returns a header-only string
// without panicking.
func TestRenderDOT_NilGraph(t *testing.T) {
	out := RenderDOT(nil)
	if out != "digraph container {\n}\n" {
		t.Errorf("RenderDOT(nil) = %q, want %q", out, "digraph container {\n}\n")
	}
}

// TestRenderDOT_EmptyGraph checks that an empty (non-nil) graph returns the
// header skeleton without panicking.
func TestRenderDOT_EmptyGraph(t *testing.T) {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "container"}
	out := RenderDOT(g)
	if !strings.Contains(out, "digraph") {
		t.Errorf("RenderDOT empty: expected 'digraph' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "}\n") {
		t.Errorf("RenderDOT empty: expected closing brace, got:\n%s", out)
	}
}
