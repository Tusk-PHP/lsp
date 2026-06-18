// Package graph defines the versioned DTO used to represent dependency/container
// graphs, plus JSON serialisation with deterministic (sorted) output.
package graph

import (
	"fmt"
	"strings"
)

// sanitizeMermaidID converts a node ID (often a PHP FQN containing backslashes,
// colons, and other characters that are not valid in Mermaid node identifiers)
// into a stable, id-safe string. The mapping is built from the sorted node list
// so that it is deterministic: n0, n1, n2, ... in sorted order.
func buildMermaidIDMap(g *Graph) map[string]string {
	m := make(map[string]string, len(g.Nodes))
	for i, n := range g.Nodes {
		m[n.ID] = fmt.Sprintf("n%d", i)
	}
	return m
}

// escapeMermaidLabel escapes double-quote characters in a Mermaid label string.
func escapeMermaidLabel(s string) string {
	return strings.ReplaceAll(s, `"`, "#quot;")
}

// mermaidNodeLine returns one Mermaid node definition line for the given node,
// using the pre-computed sanitized id and the node kind to pick the shape.
//
//   - class            => id["label"]
//   - binding          => id(["label"])
//   - dependency-boundary => id[["label"]]
//   - model            => id[("label")]
//   - package          => id(["label"])  (stadium/rounded shape)
//   - anything else    => id["label"]  (same as class)
func mermaidNodeLine(mid, label, kind string) string {
	escaped := escapeMermaidLabel(label)
	switch kind {
	case "binding":
		return fmt.Sprintf(`    %s(["%s"])`, mid, escaped)
	case "dependency-boundary":
		return fmt.Sprintf(`    %s[["%s"]]`, mid, escaped)
	case "model":
		return fmt.Sprintf(`    %s[("%s")]`, mid, escaped)
	case "package":
		return fmt.Sprintf(`    %s(["%s"])`, mid, escaped)
	default: // "class" and unknown/empty
		return fmt.Sprintf(`    %s["%s"]`, mid, escaped)
	}
}

// RenderMermaid renders g as a Mermaid flowchart (graph LR). Node shapes encode
// the node kind; edges are labelled with the edge kind. Output is deterministic:
// Sort is called on a copy before rendering. Nil or empty graphs return the
// header skeleton without panicking.
func RenderMermaid(g *Graph) string {
	if g == nil {
		return "graph LR\n"
	}

	// Work on a shallow copy so we do not mutate the caller's graph.
	work := &Graph{
		SchemaVersion: g.SchemaVersion,
		Kind:          g.Kind,
		Nodes:         append([]Node(nil), g.Nodes...),
		Edges:         append([]Edge(nil), g.Edges...),
	}
	work.Sort()

	idMap := buildMermaidIDMap(work)

	var b strings.Builder
	b.WriteString("graph LR\n")

	for _, n := range work.Nodes {
		mid := idMap[n.ID]
		label := n.Label
		if label == "" {
			label = n.ID
		}
		b.WriteString(mermaidNodeLine(mid, label, n.Kind))
		b.WriteByte('\n')
	}

	for _, e := range work.Edges {
		fromID, fromOK := idMap[e.From]
		toID, toOK := idMap[e.To]
		if !fromOK || !toOK {
			// Defensive: skip edges whose endpoints are not in the node set.
			continue
		}
		fmt.Fprintf(&b, "    %s -->|%s| %s\n", fromID, e.Kind, toID)
	}

	return b.String()
}

// escapeDOTString escapes a string for use inside a DOT double-quoted attribute
// value. Both backslashes and double-quotes must be escaped because DOT's string
// syntax treats both as special.
func escapeDOTString(s string) string {
	// Escape backslashes first, then double-quotes.
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

// dotShape returns the Graphviz shape name for a given node kind.
//
//   - class               => box
//   - binding             => ellipse
//   - dependency-boundary => box3d
//   - model               => cylinder
//   - package             => component
//   - anything else       => box
func dotShape(kind string) string {
	switch kind {
	case "binding":
		return "ellipse"
	case "dependency-boundary":
		return "box3d"
	case "model":
		return "cylinder"
	case "package":
		return "component"
	default: // "class" and unknown/empty
		return "box"
	}
}

// RenderDOT renders g as a Graphviz DOT digraph. Node shapes encode the node
// kind; edges carry a label with the edge kind. Output is deterministic: Sort is
// called on a copy before rendering. Nil or empty graphs return the header
// skeleton without panicking.
func RenderDOT(g *Graph) string {
	if g == nil {
		return "digraph {\n}\n"
	}

	// Work on a shallow copy so we do not mutate the caller's graph.
	work := &Graph{
		SchemaVersion: g.SchemaVersion,
		Kind:          g.Kind,
		Nodes:         append([]Node(nil), g.Nodes...),
		Edges:         append([]Edge(nil), g.Edges...),
	}
	work.Sort()

	// Build a set of known node IDs for defensive edge filtering.
	nodeSet := make(map[string]struct{}, len(work.Nodes))
	for _, n := range work.Nodes {
		nodeSet[n.ID] = struct{}{}
	}

	var b strings.Builder
	// Name the digraph after the graph kind (e.g. "container", "references").
	// "graph"/"digraph" are DOT keywords, so an empty kind yields an anonymous
	// digraph rather than an invalid name.
	if work.Kind != "" {
		b.WriteString("digraph " + work.Kind + " {\n")
	} else {
		b.WriteString("digraph {\n")
	}

	for _, n := range work.Nodes {
		label := n.Label
		if label == "" {
			label = n.ID
		}
		fmt.Fprintf(&b, "    \"%s\" [label=\"%s\", shape=%s];\n",
			escapeDOTString(n.ID),
			escapeDOTString(label),
			dotShape(n.Kind),
		)
	}

	for _, e := range work.Edges {
		if _, fromOK := nodeSet[e.From]; !fromOK {
			continue
		}
		if _, toOK := nodeSet[e.To]; !toOK {
			continue
		}
		fmt.Fprintf(&b, "    \"%s\" -> \"%s\" [label=\"%s\"];\n",
			escapeDOTString(e.From),
			escapeDOTString(e.To),
			escapeDOTString(e.Kind),
		)
	}

	b.WriteString("}\n")
	return b.String()
}
