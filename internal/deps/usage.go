// Package deps provides briefing-grade dependency-usage analysis for Composer
// projects. It computes, for each installed package, the fraction of its
// exposed public class surface that is statically referenced by first-party
// code in the symbol index.
//
// # Approximation caveat
//
// Results are briefing-grade, not exhaustive. Two fundamental limits apply:
//
//  1. The LSP symbol index is populated on-demand (vendor symbols are indexed
//     lazily as files are opened or referenced). ExposedClasses counts only
//     the symbols that happen to be present in the index at call time; it may
//     under-count a package's true public surface. This is the L31 approximation
//     documented in CURRENT_ISSUES.md.
//
//  2. No code is executed. Dynamic usage patterns — service containers, runtime
//     class-string references, facade aliases, auto-discovered bundles — are
//     invisible to static analysis. Use the unuseddeps package for framework-aware
//     unused-candidate detection.
//
// Callers should communicate these limits to users (e.g. "≈ 4% of
// monolog/monolog's surface touched — briefing grade").
package deps

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/graph"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// PackageUsage holds the briefing-grade usage statistics for a single
// installed Composer package.
type PackageUsage struct {
	// Package is the Composer package name, e.g. "monolog/monolog".
	Package string `json:"package"`

	// Version is the installed version string, e.g. "3.7.0".
	Version string `json:"version"`

	// TouchedClasses is the numerator: the number of distinct vendor class FQNs
	// from this package that were reached by at least one first-party reference
	// edge in the dependency-boundary reference graph.
	TouchedClasses int `json:"touchedClasses"`

	// ExposedClasses is the denominator: the number of distinct KindClass symbols
	// found under this package's PSR-4 prefixes in the symbol index. Due to
	// lazy/on-demand vendor indexing, this may under-count the true surface.
	ExposedClasses int `json:"exposedClasses"`

	// Percent is TouchedClasses/ExposedClasses expressed as a value in [0, 100].
	// It is 0 when ExposedClasses is 0 (division-by-zero guard).
	Percent float64 `json:"percent"`
}

// Usage computes a briefing-grade dependency-usage report for every installed
// Composer package that declares at least one PSR-4 autoload prefix.
//
// The returned slice is sorted by Package name ascending. Packages with zero
// touched classes are included — they are the primary signal for potential
// unused dependencies.
//
// A nil idx returns an empty slice without panicking. rootPath must be the
// project root (the directory containing composer.json).
//
// See the package-level documentation for accuracy caveats.
func Usage(idx *symbols.Index, rootPath string) []PackageUsage {
	if idx == nil {
		return []PackageUsage{}
	}

	// ── Numerator pass ────────────────────────────────────────────────────────
	//
	// Build a dependency-boundary reference graph. Each vendor package is
	// collapsed into a single "dependency-boundary" node whose Meta carries:
	//   "package"         – composer package name (string)
	//   "version"         – installed version (string, may be absent)
	//   "distinctSymbols" – number of distinct vendor FQNs collapsed into it (int)
	//   "edgeCount"       – number of distinct first-party source nodes (int)
	//
	// A boundary node is "touched" when it appears as the To target of at least
	// one edge (i.e. some first-party symbol references something in the package).
	pkgResolver := composer.NewPackageResolver(rootPath)
	g := graph.BuildReferences(idx, graph.Options{
		Deps:     graph.DepsBoundary,
		Packages: pkgResolver,
	})

	// Build the set of boundary node IDs that are edge targets.
	reachedTo := make(map[string]struct{}, len(g.Edges))
	for _, e := range g.Edges {
		reachedTo[e.To] = struct{}{}
	}

	// Extract TouchedClasses (distinctSymbols) per composer package name.
	// Meta["distinctSymbols"] is stored as a Go int by graph/deps.go (it is the
	// result of len(...), which is int). When the graph is round-tripped through
	// JSON encoding/decoding the value would be float64; we support both forms
	// defensively so callers that deserialise a cached graph are not broken.
	touchedByPkg := make(map[string]int)
	for _, n := range g.Nodes {
		if n.Kind != "dependency-boundary" {
			continue
		}
		pkg, _ := n.Meta["package"].(string)
		if pkg == "" {
			continue
		}
		if _, reached := reachedTo[n.ID]; !reached {
			// Not reached by any edge — TouchedClasses stays 0 for this package.
			continue
		}
		// Defensively accept both int (live graph) and float64 (JSON-decoded).
		var ds int
		switch v := n.Meta["distinctSymbols"].(type) {
		case int:
			ds = v
		case float64:
			ds = int(v)
		}
		touchedByPkg[pkg] += ds
	}

	// ── Denominator pass ──────────────────────────────────────────────────────
	//
	// For each installed package, count distinct KindClass FQNs found under its
	// PSR-4 prefixes in the current index. We dedupe FQNs across prefixes within
	// the same package (a class could match multiple prefixes in theory).
	installedPkgs := composer.InstalledPackages(rootPath)
	if len(installedPkgs) == 0 {
		return []PackageUsage{}
	}

	type pkgMeta struct {
		version        string
		hasPSR4        bool
		exposedClasses int
	}
	metaByPkg := make(map[string]*pkgMeta, len(installedPkgs))

	for _, p := range installedPkgs {
		if len(p.PSR4Prefixes) == 0 {
			continue
		}
		m := &pkgMeta{version: p.Version, hasPSR4: true}
		metaByPkg[p.Name] = m

		// Collect distinct KindClass FQNs across all PSR-4 prefixes for this package.
		seen := make(map[string]struct{})
		for _, prefix := range p.PSR4Prefixes {
			syms, _ := idx.SearchByFQNPrefix(prefix)
			for _, sym := range syms {
				if sym.Kind != symbols.KindClass {
					continue
				}
				if _, already := seen[sym.FQN]; !already {
					seen[sym.FQN] = struct{}{}
					m.exposedClasses++
				}
			}
		}
	}

	// ── Assemble result ───────────────────────────────────────────────────────
	//
	// Emit one PackageUsage per package that has a PSR-4 surface. Packages with
	// no PSR-4 autoload entries (e.g. platform pseudo-packages, pure-PHP-file
	// packages) are omitted because there is no meaningful class surface to
	// compute a percentage over.
	result := make([]PackageUsage, 0, len(metaByPkg))
	for pkg, m := range metaByPkg {
		touched := touchedByPkg[pkg]
		exposed := m.exposedClasses

		// Briefing-grade reconciliation: vendor is indexed by-need, so the
		// exposed-class denominator can under-count and even fall below the
		// touched numerator (a class you reference must exist). Floor the
		// denominator at the touched count so Percent stays within [0, 100]
		// instead of reporting nonsensical >100% figures. See CURRENT_ISSUES
		// L31 for the accuracy caveat.
		if exposed < touched {
			exposed = touched
		}

		var pct float64
		if exposed > 0 {
			pct = float64(touched) / float64(exposed) * 100.0
		}

		result = append(result, PackageUsage{
			Package:        pkg,
			Version:        m.version,
			TouchedClasses: touched,
			ExposedClasses: exposed,
			Percent:        pct,
		})
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Package < result[j].Package
	})

	return result
}
