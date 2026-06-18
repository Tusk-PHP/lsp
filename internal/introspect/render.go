package introspect

import (
	"fmt"
	"strings"
)

const (
	headerLine = "═══ Tusk PHP LSP · Parsed-State Dump ══════════════════════════════"
	footerLine = "═══════════════════════════════════════════════════════════════════"
	divider    = "────────────────────────────────────────────────────────────────────"
)

// Render converts a Dump to a human-readable text string, matching the
// structure shown in the design's worked example.
// Compact verbosity renders header + symbol + inferred members + diagnostics.
// Full verbosity adds declared members, reference edges, and container.
// A nil Dump returns an empty string.
// When the file declares multiple top-level symbols each symbol gets its own
// block separated by a divider line; files with no symbols still render the
// header and diagnostics.
func Render(d *Dump, v Verbosity) string {
	if d == nil {
		return ""
	}

	var b strings.Builder

	b.WriteString(headerLine)
	b.WriteByte('\n')

	// Header section
	writeHeader(&b, d)

	b.WriteString(divider)
	b.WriteByte('\n')

	// Render each top-level symbol with a divider between them.
	for i, sec := range d.Symbols {
		if i > 0 {
			b.WriteString(divider)
			b.WriteByte('\n')
		}

		// SYMBOL section
		writeSymbolSection(&b, sec)
		b.WriteByte('\n')

		// DECLARED MEMBERS — Full only
		if v == Full {
			writeDeclaredMembers(&b, sec.DeclaredMembers)
			b.WriteByte('\n')
		}

		// INFERRED MEMBERS — always
		writeInferredMembers(&b, sec.InferredMembers)
		b.WriteByte('\n')

		// REFERENCE EDGES — Full only
		if v == Full {
			writeReferenceEdges(&b, sec.ReferenceEdges)
			b.WriteByte('\n')
		}

		// CONTAINER — Full only
		if v == Full {
			writeContainerSection(&b, sec.ContainerBindings, sec.InjectedDeps)
			b.WriteByte('\n')
		}
	}

	// DIAGNOSTICS — always (file-level, after all symbols)
	writeDiagnostics(&b, d.Diagnostics)

	b.WriteString(footerLine)
	b.WriteByte('\n')

	return b.String()
}

func writeHeader(b *strings.Builder, d *Dump) {
	filePath := d.FilePath
	if filePath == "" {
		filePath = d.URI
	}
	fmt.Fprintf(b, "File      %s\n", filePath)

	bufState := d.BufferState
	if bufState == "" {
		bufState = "unknown"
	}
	if bufState == "live-buffer" {
		fmt.Fprintf(b, "Buffer    UNSAVED (live buffer)\n")
	} else {
		fmt.Fprintf(b, "Buffer    %s\n", bufState)
	}

	serverParts := []string{}
	if d.ServerVersion != "" {
		serverParts = append(serverParts, d.ServerVersion)
	}
	if d.Profile != "" {
		serverParts = append(serverParts, d.Profile)
	}
	if d.Framework != "" {
		serverParts = append(serverParts, "framework "+d.Framework)
	}
	if len(serverParts) > 0 {
		fmt.Fprintf(b, "Server    %s\n", strings.Join(serverParts, " · "))
	}

	if d.DumpSource != "" {
		fmt.Fprintf(b, "Source    %s\n", d.DumpSource)
	}
}

func writeSymbolSection(b *strings.Builder, sec *SymbolSection) {
	b.WriteString("SYMBOL\n")
	fmt.Fprintf(b, "  %s %s", sec.Kind, sec.FQN)
	if sec.Line > 0 {
		fmt.Fprintf(b, "  (line %d)", sec.Line+1)
	}
	b.WriteByte('\n')

	if sec.Extends != nil {
		fmt.Fprintf(b, "    extends  %s  → %s\n", sec.Extends.Name, resolveAnnotation(sec.Extends))
	}

	for i := range sec.Implements {
		impl := &sec.Implements[i]
		fmt.Fprintf(b, "    implements  %s  → %s\n", impl.Name, resolveAnnotation(impl))
	}

	if len(sec.Traits) > 0 {
		parts := make([]string, 0, len(sec.Traits))
		for i := range sec.Traits {
			tr := &sec.Traits[i]
			parts = append(parts, tr.Name+" → "+resolveAnnotation(tr))
		}
		fmt.Fprintf(b, "    traits   %s\n", strings.Join(parts, " · "))
	}
}

func resolveAnnotation(ref *SymbolRef) string {
	if ref == nil {
		return "unresolved"
	}
	if !ref.Resolved {
		return "unresolved"
	}
	if ref.Source != "" {
		return "resolved (" + ref.Source + ")"
	}
	return "resolved"
}

func writeDeclaredMembers(b *strings.Builder, members []MemberEntry) {
	b.WriteString("DECLARED MEMBERS\n")
	if len(members) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, m := range members {
		writeMemberLine(b, m)
	}
}

func writeInferredMembers(b *strings.Builder, members []MemberEntry) {
	b.WriteString("INFERRED MEMBERS  (virtual — what hover/completion will offer)\n")
	if len(members) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, m := range members {
		writeMemberLine(b, m)
		if m.FidelityNote != "" {
			// Indent the fidelity note below the member line.
			for _, line := range strings.Split(m.FidelityNote, "\n") {
				fmt.Fprintf(b, "     %s\n", line)
			}
		}
	}
}

func writeMemberLine(b *strings.Builder, m MemberEntry) {
	switch m.Kind {
	case "method":
		params := strings.Join(m.Params, ", ")
		if m.ReturnType != "" {
			fmt.Fprintf(b, "  %s  %s(%s) : %s", m.Kind, m.Name, params, m.ReturnType)
		} else {
			fmt.Fprintf(b, "  %s  %s(%s)", m.Kind, m.Name, params)
		}
	case "prop":
		if m.Type != "" {
			fmt.Fprintf(b, "  %s    %s : %s", m.Kind, m.Name, m.Type)
		} else {
			fmt.Fprintf(b, "  %s    %s", m.Kind, m.Name)
		}
	default:
		if m.Type != "" {
			fmt.Fprintf(b, "  %s   %s : %s", m.Kind, m.Name, m.Type)
		} else {
			fmt.Fprintf(b, "  %s   %s", m.Kind, m.Name)
		}
	}
	if m.Line > 0 {
		fmt.Fprintf(b, "  (line %d)", m.Line+1)
	}
	b.WriteByte('\n')
}

func writeReferenceEdges(b *strings.Builder, edges []ReferenceEdge) {
	b.WriteString("REFERENCE EDGES  (structural Tier-1, as stored)\n")
	if len(edges) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, e := range edges {
		if e.Unresolved {
			fmt.Fprintf(b, "  %-8s → %s  ⚠ UNRESOLVED — bare short name\n", string(e.Kind), e.RawType)
		} else {
			fmt.Fprintf(b, "  %-8s → %s  %s\n", string(e.Kind), e.ResolvedFQN, e.Source)
		}
	}
}

func writeContainerSection(b *strings.Builder, bindings []ContainerEntry, deps []InjectedDep) {
	b.WriteString("CONTAINER\n")
	if len(bindings) == 0 {
		b.WriteString("  No bindings resolve to this class.\n")
	} else {
		for _, binding := range bindings {
			singleton := ""
			if binding.Singleton {
				singleton = " (singleton)"
			}
			if binding.Abstract != binding.Concrete {
				fmt.Fprintf(b, "  bind  %s → %s%s\n", binding.Abstract, binding.Concrete, singleton)
			} else {
				fmt.Fprintf(b, "  bind  %s%s\n", binding.Abstract, singleton)
			}
		}
	}
	if len(deps) > 0 {
		b.WriteString("  constructor injection:\n")
		for _, d := range deps {
			if d.ResolvedConcrete != "" && d.ResolvedConcrete != d.TypeHint {
				fmt.Fprintf(b, "    %s %s → %s\n", d.ParamName, d.TypeHint, d.ResolvedConcrete)
			} else {
				fmt.Fprintf(b, "    %s %s\n", d.ParamName, d.TypeHint)
			}
		}
	}
}

func writeDiagnostics(b *strings.Builder, diags []DiagnosticEntry) {
	b.WriteString("DIAGNOSTICS  (native checks, this file)\n")
	if len(diags) == 0 {
		b.WriteString("  (none)\n")
		return
	}
	for _, diag := range diags {
		fmt.Fprintf(b, "  %-5s line %-4d %-20s %s\n",
			diag.Severity,
			diag.Line+1,
			diag.Code,
			diag.Message,
		)
	}
}
