// Package introspect produces a faithful parsed-state dump for a single PHP
// file — declaring what the LSP has actually parsed and resolved for that file,
// including declared symbols, inferred/virtual members, structural reference
// edges, container bindings, and native diagnostics.
//
// The cardinal rule is fidelity: read the ACTUAL stored index/container state
// and report it as-is, warts included. Do NOT re-derive or clean up data.
package introspect

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/checks"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Verbosity controls which sections are included in the rendered output.
type Verbosity int

const (
	// Compact renders header + symbol + inferred members + diagnostics.
	Compact Verbosity = iota
	// Full adds declared members, reference edges, and container sections.
	Full
)

// ParseVerbosity converts a string flag value to a Verbosity constant.
// "full" returns Full; any other value (including empty) returns Compact.
func ParseVerbosity(s string) Verbosity {
	if strings.ToLower(s) == "full" {
		return Full
	}
	return Compact
}

// Input bundles the stored state that Document reads to build the dump.
// All pointer fields are nil-safe: a nil Index or Container produces a
// partial dump rather than a panic.
type Input struct {
	// URI is the file:// URI of the document being dumped.
	URI string
	// Source is the raw PHP source text (may be empty if unavailable).
	Source string
	// Index is the live symbol index; may be nil.
	Index *symbols.Index
	// Container is the DI container analyzer; may be nil.
	Container *container.ContainerAnalyzer
	// Framework is the detected framework name (e.g. "laravel", "symfony", "").
	Framework string
	// ServerVersion is the running server build string (e.g. "tusk-php 0.9.0").
	ServerVersion string
	// Profile describes the active PHP profile (e.g. "PHP 8.2 (ext: pdo, mbstring)").
	Profile string
	// BufferState is either "live-buffer" (index reflects unsaved in-memory edits)
	// or "disk" (index was built from file on disk).
	BufferState string
	// Diagnostics holds the native-check findings for this file (caller-supplied).
	Diagnostics []checks.Finding
	// Verbosity selects which sections to include.
	Verbosity Verbosity
}

// SymbolSection summarises the file's primary class/interface/trait/enum.
type SymbolSection struct {
	// FQN is the fully-qualified name of the primary symbol.
	FQN string
	// Kind is the symbol kind label ("class", "interface", "trait", "enum").
	Kind string
	// Line is 0-based source line of the symbol declaration.
	Line int
	// Extends describes the parent class with its resolution status.
	Extends *SymbolRef
	// Implements lists implemented interfaces with resolution status.
	Implements []SymbolRef
	// Traits lists used traits with resolution status.
	Traits []SymbolRef
}

// SymbolRef is a single reference to another type with its resolution status.
type SymbolRef struct {
	// Name is the name exactly as stored in the index (may be a short name).
	Name string
	// Resolved is true when Lookup found the target in the index.
	Resolved bool
	// Source classifies where the target was found ("project", "vendor", "builtin").
	// Empty when Resolved is false.
	Source string
}

// MemberEntry describes a single declared or virtual class member.
type MemberEntry struct {
	// Name is the member name (e.g. "$items", "scopePending").
	Name string
	// Kind is the member kind label ("prop", "method", "const").
	Kind string
	// Type is the stored type annotation (may be empty or a bare short name).
	Type string
	// ReturnType is set for methods.
	ReturnType string
	// Params lists the parameter signatures for methods.
	Params []string
	// DocComment is stored as-is from the index.
	DocComment string
	// Line is the 0-based source line.
	Line int
	// IsVirtual mirrors symbols.Symbol.IsVirtual.
	IsVirtual bool
	// FidelityNote is non-empty when the stored representation is known to be
	// lossy (e.g. a to-many relation collapsed to "Collection").
	FidelityNote string
}

// RefEdgeKind classifies the structural relationship kind.
type RefEdgeKind string

const (
	RefEdgeParam      RefEdgeKind = "param"
	RefEdgeReturns    RefEdgeKind = "returns"
	RefEdgeProperty   RefEdgeKind = "property"
	RefEdgeExtends    RefEdgeKind = "extends"
	RefEdgeImplements RefEdgeKind = "implements"
)

// ReferenceEdge is a structural Tier-1 edge from the primary symbol to another
// type, as stored in the index.
type ReferenceEdge struct {
	// Kind labels the edge relationship.
	Kind RefEdgeKind
	// RawType is the type exactly as stored in the index (may be a short name).
	RawType string
	// ResolvedFQN is set when Lookup found the target.
	ResolvedFQN string
	// Source classifies the resolved target's origin; empty when unresolved.
	Source string
	// Unresolved is true when the type could not be found in the index.
	Unresolved bool
	// ContextName provides additional context (member name for param/property edges).
	ContextName string
}

// ContainerEntry describes one relevant DI container binding.
type ContainerEntry struct {
	// Abstract is the interface/key bound in the container.
	Abstract string
	// Concrete is the implementation FQN.
	Concrete string
	// Singleton indicates whether the binding is a singleton.
	Singleton bool
}

// InjectedDep mirrors container.InjectedDependency for the dump.
type InjectedDep struct {
	ParamName        string
	TypeHint         string
	ResolvedConcrete string
}

// DiagnosticEntry is a single diagnostic finding formatted for display.
type DiagnosticEntry struct {
	Severity string
	Line     int
	Code     string
	Message  string
}

// Dump holds all sections of a parsed-state dump in structured form.
// Exported fields allow test assertions without string parsing.
type Dump struct {
	// URI is the file:// URI that was dumped.
	URI string
	// FilePath is the filesystem path extracted from URI.
	FilePath string
	// BufferState is "live-buffer" or "disk".
	BufferState string
	// ServerVersion is the server build string.
	ServerVersion string
	// Profile is the active PHP profile string.
	Profile string
	// Framework is the detected framework name.
	Framework string
	// DumpSource describes whether the dump came from the live in-memory index
	// or a disk-built index (for the fidelity annotation in the header).
	DumpSource string

	// PrimarySymbol holds the file's primary class/interface/trait/enum.
	// Nil when no primary symbol was found.
	PrimarySymbol *SymbolSection

	// DeclaredMembers are the non-virtual members from GetClassMembers.
	DeclaredMembers []MemberEntry
	// InferredMembers are the virtual members from GetClassMembers.
	InferredMembers []MemberEntry

	// ReferenceEdges are structural edges (Full verbosity only).
	ReferenceEdges []ReferenceEdge

	// ContainerBindings are DI bindings that relate to the primary symbol.
	ContainerBindings []ContainerEntry
	// InjectedDeps is the constructor injection analysis for the primary symbol.
	InjectedDeps []InjectedDep

	// Diagnostics are the native-check findings for this file.
	Diagnostics []DiagnosticEntry
}

// Document builds a Dump by reading stored state from in.Index and
// in.Container. It never panics on nil inputs and always returns a non-nil
// Dump (possibly with empty sections).
func Document(in Input) *Dump {
	d := &Dump{
		URI:           in.URI,
		FilePath:      symbols.URIToPath(in.URI),
		BufferState:   in.BufferState,
		ServerVersion: in.ServerVersion,
		Profile:       in.Profile,
		Framework:     in.Framework,
	}

	// Determine dump source annotation.
	if in.BufferState == "live-buffer" {
		d.DumpSource = "live in-memory index (executeCommand) — interactive fidelity"
	} else {
		d.DumpSource = "disk-built index — disk fidelity"
	}

	if in.Index == nil {
		// No index available: populate diagnostics only.
		d.Diagnostics = buildDiagnostics(in.Diagnostics)
		return d
	}

	// ── SYMBOL ──────────────────────────────────────────────────────────────
	fileSyms := in.Index.GetFileSymbols(in.URI)
	primary := pickPrimarySymbol(fileSyms)
	if primary != nil {
		d.PrimarySymbol = buildSymbolSection(primary, in.Index)

		// ── MEMBERS ─────────────────────────────────────────────────────────
		members := in.Index.GetClassMembers(primary.FQN)
		sort.Slice(members, func(i, j int) bool {
			if members[i].Range.Start.Line != members[j].Range.Start.Line {
				return members[i].Range.Start.Line < members[j].Range.Start.Line
			}
			return members[i].Name < members[j].Name
		})

		for _, m := range members {
			entry := buildMemberEntry(m)
			if m.IsVirtual {
				d.InferredMembers = append(d.InferredMembers, entry)
			} else {
				d.DeclaredMembers = append(d.DeclaredMembers, entry)
			}
		}

		// ── REFERENCE EDGES (Full only) ──────────────────────────────────────
		if in.Verbosity == Full {
			d.ReferenceEdges = buildReferenceEdges(primary, members, in.Index)
		}

		// ── CONTAINER (Full only) ────────────────────────────────────────────
		if in.Verbosity == Full && in.Container != nil {
			d.ContainerBindings, d.InjectedDeps = buildContainerSection(primary.FQN, in.Container)
		}
	}

	// ── DIAGNOSTICS ──────────────────────────────────────────────────────────
	d.Diagnostics = buildDiagnostics(in.Diagnostics)

	return d
}

// pickPrimarySymbol returns the first class, interface, trait, or enum from
// the file's symbol list, preferring them over functions and members.
func pickPrimarySymbol(fileSyms []*symbols.Symbol) *symbols.Symbol {
	for _, s := range fileSyms {
		switch s.Kind {
		case symbols.KindClass, symbols.KindInterface, symbols.KindTrait, symbols.KindEnum:
			return s
		}
	}
	return nil
}

// buildSymbolSection constructs the SYMBOL section for a primary symbol.
func buildSymbolSection(sym *symbols.Symbol, idx *symbols.Index) *SymbolSection {
	sec := &SymbolSection{
		FQN:  sym.FQN,
		Kind: kindLabel(sym.Kind),
		Line: sym.Range.Start.Line,
	}

	if sym.Extends != "" {
		ref := resolveRef(sym.Extends, idx)
		sec.Extends = &ref
	}

	for _, impl := range sym.Implements {
		sec.Implements = append(sec.Implements, resolveRef(impl, idx))
	}

	for _, tr := range idx.TraitsOf(sym.FQN) {
		sec.Traits = append(sec.Traits, resolveRef(tr, idx))
	}

	return sec
}

// resolveRef looks up a type name in the index and returns a SymbolRef with
// resolution status and source classification.
func resolveRef(name string, idx *symbols.Index) SymbolRef {
	if name == "" {
		return SymbolRef{Name: name, Resolved: false}
	}
	sym := idx.Lookup(name)
	if sym == nil {
		return SymbolRef{Name: name, Resolved: false}
	}
	return SymbolRef{
		Name:     name,
		Resolved: true,
		Source:   sourceLabel(sym.Source),
	}
}

// buildMemberEntry converts a symbols.Symbol into a MemberEntry, adding a
// fidelity note when the stored type is lossy for virtual to-many relations.
func buildMemberEntry(sym *symbols.Symbol) MemberEntry {
	e := MemberEntry{
		Name:       sym.Name,
		Kind:       memberKindLabel(sym.Kind),
		Type:       sym.Type,
		ReturnType: sym.ReturnType,
		DocComment: sym.DocComment,
		Line:       sym.Range.Start.Line,
		IsVirtual:  sym.IsVirtual,
	}
	for _, p := range sym.Params {
		e.Params = append(e.Params, formatParam(p))
	}
	if sym.IsVirtual {
		e.FidelityNote = collectionFidelityNote(sym)
	}
	return e
}

// collectionFidelityNote returns a non-empty fidelity warning when a virtual
// property's type is "Collection" or bare "array", indicating that the to-many
// target FQN has been collapsed and is not retained in the index.
func collectionFidelityNote(sym *symbols.Symbol) string {
	if sym.Kind != symbols.KindProperty {
		return ""
	}
	t := strings.ToLower(strings.TrimSpace(sym.Type))
	// Strip namespace prefix to catch Illuminate\...\Collection too.
	if idx := strings.LastIndex(t, "\\"); idx >= 0 {
		t = t[idx+1:]
	}
	if t == "collection" || t == "array" {
		return "⚠ FIDELITY: to-many target collapsed to `" + sym.Type + "`; related FQN NOT retained — shown as the index actually stores it."
	}
	return ""
}

// buildReferenceEdges derives Tier-1 structural reference edges from the
// primary symbol and its members, resolving each type via Lookup.
func buildReferenceEdges(primary *symbols.Symbol, members []*symbols.Symbol, idx *symbols.Index) []ReferenceEdge {
	seen := make(map[string]bool)
	var edges []ReferenceEdge

	addEdge := func(kind RefEdgeKind, rawType, contextName string) {
		if rawType == "" {
			return
		}
		// Deduplicate on (kind, rawType, contextName).
		key := string(kind) + "|" + rawType + "|" + contextName
		if seen[key] {
			return
		}
		seen[key] = true

		edge := ReferenceEdge{
			Kind:        kind,
			RawType:     rawType,
			ContextName: contextName,
		}
		sym := idx.Lookup(rawType)
		if sym != nil {
			edge.ResolvedFQN = sym.FQN
			edge.Source = sourceLabel(sym.Source)
		} else {
			edge.Unresolved = true
		}
		edges = append(edges, edge)
	}

	// Extends and implements edges from the primary symbol.
	if primary.Extends != "" {
		addEdge(RefEdgeExtends, primary.Extends, "")
	}
	for _, impl := range primary.Implements {
		addEdge(RefEdgeImplements, impl, "")
	}

	// Member-level edges.
	for _, m := range members {
		if m.IsVirtual {
			continue // virtual members are display-only, not structural edges
		}
		switch m.Kind {
		case symbols.KindProperty:
			if m.Type != "" && !symbols.IsPHPBuiltinType(m.Type) {
				addEdge(RefEdgeProperty, m.Type, m.Name)
			}
		case symbols.KindMethod:
			if m.ReturnType != "" && !symbols.IsPHPBuiltinType(m.ReturnType) {
				addEdge(RefEdgeReturns, m.ReturnType, m.Name)
			}
			for _, p := range m.Params {
				if p.Type != "" && !symbols.IsPHPBuiltinType(p.Type) {
					addEdge(RefEdgeParam, p.Type, m.Name+":"+p.Name)
				}
			}
		}
	}

	sort.Slice(edges, func(i, j int) bool {
		a, b := edges[i], edges[j]
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		if a.RawType != b.RawType {
			return a.RawType < b.RawType
		}
		return a.ContextName < b.ContextName
	})

	return edges
}

// buildContainerSection returns DI bindings relevant to the primary FQN and
// its constructor injection analysis.
func buildContainerSection(fqn string, ca *container.ContainerAnalyzer) ([]ContainerEntry, []InjectedDep) {
	var entries []ContainerEntry

	bindings := ca.GetBindings()

	// Collect keys and sort for deterministic output.
	keys := make([]string, 0, len(bindings))
	for k := range bindings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, k := range keys {
		b := bindings[k]
		if b.Abstract == fqn || b.Concrete == fqn {
			entries = append(entries, ContainerEntry{
				Abstract:  b.Abstract,
				Concrete:  b.Concrete,
				Singleton: b.Singleton,
			})
		}
	}

	rawDeps := ca.AnalyzeConstructorInjection(fqn)
	deps := make([]InjectedDep, 0, len(rawDeps))
	for _, d := range rawDeps {
		deps = append(deps, InjectedDep{
			ParamName:        d.ParamName,
			TypeHint:         d.TypeHint,
			ResolvedConcrete: d.ResolvedConcrete,
		})
	}

	return entries, deps
}

// buildDiagnostics converts checks.Finding values into DiagnosticEntry values,
// sorted by line then code for deterministic output.
func buildDiagnostics(findings []checks.Finding) []DiagnosticEntry {
	entries := make([]DiagnosticEntry, 0, len(findings))
	for _, f := range findings {
		entries = append(entries, DiagnosticEntry{
			Severity: severityLabel(f.Severity),
			Line:     f.StartLine,
			Code:     f.Code,
			Message:  f.Message,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Line != entries[j].Line {
			return entries[i].Line < entries[j].Line
		}
		return entries[i].Code < entries[j].Code
	})
	return entries
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func kindLabel(k symbols.SymbolKind) string {
	switch k {
	case symbols.KindClass:
		return "class"
	case symbols.KindInterface:
		return "interface"
	case symbols.KindTrait:
		return "trait"
	case symbols.KindEnum:
		return "enum"
	default:
		return "unknown"
	}
}

func memberKindLabel(k symbols.SymbolKind) string {
	switch k {
	case symbols.KindProperty:
		return "prop"
	case symbols.KindMethod:
		return "method"
	case symbols.KindConstant:
		return "const"
	default:
		return "member"
	}
}

func sourceLabel(src symbols.SymbolSource) string {
	switch src {
	case symbols.SourceProject:
		return "project"
	case symbols.SourceVendor:
		return "vendor"
	case symbols.SourceBuiltin:
		return "builtin"
	default:
		return "unknown"
	}
}

func severityLabel(s checks.Severity) string {
	switch s {
	case checks.SeverityError:
		return "error"
	case checks.SeverityWarning:
		return "warn"
	case checks.SeverityInfo:
		return "info"
	case checks.SeverityHint:
		return "hint"
	default:
		return "unknown"
	}
}

func formatParam(p symbols.ParamInfo) string {
	var b strings.Builder
	if p.IsVariadic {
		b.WriteString("...")
	}
	if p.Type != "" {
		b.WriteString(p.Type)
		b.WriteByte(' ')
	}
	b.WriteString(p.Name)
	if p.DefaultValue != "" {
		b.WriteString(" = ")
		b.WriteString(p.DefaultValue)
	}
	return b.String()
}
