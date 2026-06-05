package graph

import (
	"bytes"
	"strings"
	"testing"
)

// buildTestGraph returns a Graph whose nodes and edges are intentionally added
// in non-sorted order so that Sort / EncodeJSON ordering can be verified.
func buildTestGraph() *Graph {
	return &Graph{
		SchemaVersion: SchemaVersion,
		Kind:          "container",
		Nodes: []Node{
			{ID: "C\\Concrete", Kind: "class", Label: "Concrete"},
			{ID: "A\\Abstract", Kind: "binding", Label: "Abstract"},
			{ID: "B\\Middle", Kind: "class", Label: "Middle"},
		},
		Edges: []Edge{
			{From: "C\\Concrete", To: "B\\Middle", Kind: "injects"},
			{From: "A\\Abstract", To: "C\\Concrete", Kind: "binds"},
			{From: "A\\Abstract", To: "B\\Middle", Kind: "binds"},
		},
	}
}

func TestSort_NodeOrder(t *testing.T) {
	g := buildTestGraph()
	g.Sort()

	wantIDs := []string{"A\\Abstract", "B\\Middle", "C\\Concrete"}
	for i, want := range wantIDs {
		if g.Nodes[i].ID != want {
			t.Errorf("Nodes[%d].ID = %q, want %q", i, g.Nodes[i].ID, want)
		}
	}
}

func TestSort_EdgeOrder(t *testing.T) {
	g := buildTestGraph()
	g.Sort()

	// Edges sorted by (From, To, Kind):
	// ("A\\Abstract","B\\Middle","binds")
	// ("A\\Abstract","C\\Concrete","binds")
	// ("C\\Concrete","B\\Middle","injects")
	type edge struct{ from, to, kind string }
	want := []edge{
		{"A\\Abstract", "B\\Middle", "binds"},
		{"A\\Abstract", "C\\Concrete", "binds"},
		{"C\\Concrete", "B\\Middle", "injects"},
	}
	for i, w := range want {
		e := g.Edges[i]
		if e.From != w.from || e.To != w.to || e.Kind != w.kind {
			t.Errorf("Edges[%d] = (%q,%q,%q), want (%q,%q,%q)",
				i, e.From, e.To, e.Kind, w.from, w.to, w.kind)
		}
	}
}

func TestSort_Determinism(t *testing.T) {
	// Build two identical graphs, sort both, compare node/edge order.
	g1 := buildTestGraph()
	g2 := buildTestGraph()
	g1.Sort()
	g2.Sort()

	for i := range g1.Nodes {
		if g1.Nodes[i].ID != g2.Nodes[i].ID {
			t.Errorf("non-deterministic node order at index %d: %q vs %q",
				i, g1.Nodes[i].ID, g2.Nodes[i].ID)
		}
	}
	for i := range g1.Edges {
		if g1.Edges[i].From != g2.Edges[i].From ||
			g1.Edges[i].To != g2.Edges[i].To ||
			g1.Edges[i].Kind != g2.Edges[i].Kind {
			t.Errorf("non-deterministic edge order at index %d", i)
		}
	}
}

func TestSort_EmptyGraph(t *testing.T) {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "container"}
	// Must not panic on empty slices.
	g.Sort()
	if len(g.Nodes) != 0 || len(g.Edges) != 0 {
		t.Error("Sort on empty graph produced unexpected elements")
	}
}

func TestEncodeJSON_SchemaVersion(t *testing.T) {
	g := buildTestGraph()
	var buf bytes.Buffer
	if err := g.EncodeJSON(&buf); err != nil {
		t.Fatalf("EncodeJSON returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"schemaVersion": 1`) {
		t.Errorf("JSON output missing schemaVersion:1, got:\n%s", out)
	}
}

func TestEncodeJSON_EmptyGraphArrays(t *testing.T) {
	g := &Graph{SchemaVersion: SchemaVersion, Kind: "container"}
	var buf bytes.Buffer
	if err := g.EncodeJSON(&buf); err != nil {
		t.Fatalf("EncodeJSON returned error: %v", err)
	}
	out := buf.String()
	if strings.Contains(out, "null") {
		t.Errorf("empty graph must not serialise null fields, got:\n%s", out)
	}
	if !strings.Contains(out, `"nodes": []`) || !strings.Contains(out, `"edges": []`) {
		t.Errorf("empty graph must serialise nodes/edges as [], got:\n%s", out)
	}
}

func TestEncodeJSON_NodesSorted(t *testing.T) {
	g := buildTestGraph()
	var buf bytes.Buffer
	if err := g.EncodeJSON(&buf); err != nil {
		t.Fatalf("EncodeJSON returned error: %v", err)
	}
	out := buf.String()

	// A\Abstract must appear before B\Middle and C\Concrete.
	posA := strings.Index(out, "A\\\\Abstract")
	posB := strings.Index(out, "B\\\\Middle")
	posC := strings.Index(out, "C\\\\Concrete")
	if posA < 0 || posB < 0 || posC < 0 {
		t.Fatalf("one or more node IDs missing in output:\n%s", out)
	}
	if !(posA < posB && posB < posC) {
		t.Errorf("nodes not in sorted order in JSON output (posA=%d, posB=%d, posC=%d)",
			posA, posB, posC)
	}
}

func TestEncodeJSON_KindPresent(t *testing.T) {
	g := buildTestGraph()
	var buf bytes.Buffer
	if err := g.EncodeJSON(&buf); err != nil {
		t.Fatalf("EncodeJSON returned error: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, `"kind": "container"`) {
		t.Errorf("JSON output missing graph kind, got:\n%s", out)
	}
}

func TestEncodeJSON_Determinism(t *testing.T) {
	g1 := buildTestGraph()
	g2 := buildTestGraph()

	var b1, b2 bytes.Buffer
	if err := g1.EncodeJSON(&b1); err != nil {
		t.Fatal(err)
	}
	if err := g2.EncodeJSON(&b2); err != nil {
		t.Fatal(err)
	}
	if b1.String() != b2.String() {
		t.Error("EncodeJSON produced different output for identical graphs")
	}
}
