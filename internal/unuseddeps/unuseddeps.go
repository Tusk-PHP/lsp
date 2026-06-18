// Package unuseddeps reports composer require packages that have no static
// reference from first-party code — candidates for removal. Results are always
// framed as "candidate — review", never a hard verdict, because PHP has many
// non-static usage patterns (service providers, config strings, facades, bundles).
package unuseddeps

import (
	"sort"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/graph"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Candidate is a composer package that has no detected static reference from
// first-party code.
type Candidate struct {
	Package string `json:"package"`
	Reason  string `json:"reason"`
}

// Report is the result of an unused-dependency analysis.
type Report struct {
	Framework  string      `json:"framework"`
	Checked    int         `json:"checked"`
	Candidates []Candidate `json:"candidates"`
}

// Analyze inspects idx for first-party→vendor edges and returns the set of
// direct composer require and require-dev packages that appear to have no
// static reference.
//
// Logic:
//  1. Load the universe of direct (non-platform) requires from composer.json,
//     unioning in require-dev packages (deduplicated).
//  2. Build a dependency-boundary reference graph; every boundary node whose
//     Meta["package"] is reached by at least one first-party edge is "referenced".
//  3. Exclude packages registered via framework auto-discovery mechanisms that
//     are invisible to static analysis:
//     - Laravel: extra.laravel.providers / extra.laravel.aliases
//     - Symfony: config/bundles.php bundle class registrations
//  4. Any required package that is neither referenced nor excluded is a candidate.
//
// A nil idx is handled gracefully (returns an empty report with Checked=0).
func Analyze(idx *symbols.Index, rootPath, framework string) Report {
	rep := Report{
		Framework:  framework,
		Candidates: []Candidate{},
	}

	// Union require + require-dev into a single deduplicated, sorted universe.
	required := unionRequires(
		composer.DirectRequires(rootPath),
		composer.DirectDevRequires(rootPath),
	)
	rep.Checked = len(required)
	if len(required) == 0 || idx == nil {
		return rep
	}

	// Build reference graph with dependency-boundary mode so vendor FQNs
	// collapse into nodes keyed by composer package name.
	pkgResolver := composer.NewPackageResolver(rootPath)
	g := graph.BuildReferences(idx, graph.Options{
		Deps:     graph.DepsBoundary,
		Packages: pkgResolver,
	})

	// Collect the set of package names that are reachable from first-party code:
	// a dependency-boundary node is "referenced" when it is the To target of at
	// least one edge.
	reachedTo := make(map[string]struct{})
	for _, e := range g.Edges {
		reachedTo[e.To] = struct{}{}
	}

	referenced := make(map[string]struct{})
	for _, n := range g.Nodes {
		if n.Kind != "dependency-boundary" {
			continue
		}
		pkg, ok := n.Meta["package"].(string)
		if !ok || pkg == "" {
			continue
		}
		if _, reached := reachedTo[n.ID]; reached {
			referenced[pkg] = struct{}{}
		}
	}

	// Packages excluded via framework auto-discovery mechanisms that are
	// invisible to static analysis.  Each call is unconditional and nil-safe —
	// on projects that don't use the corresponding framework the helpers return
	// nil and contribute nothing to the exclusion set.
	excluded := make(map[string]struct{})
	for _, pkg := range composer.LaravelAutoDiscoveredPackages(rootPath) {
		excluded[pkg] = struct{}{}
	}
	for _, pkg := range composer.SymfonyBundlePackages(rootPath) {
		excluded[pkg] = struct{}{}
	}

	// Emit candidates: required but not referenced and not excluded.
	for _, pkg := range required {
		if _, ref := referenced[pkg]; ref {
			continue
		}
		if _, ex := excluded[pkg]; ex {
			continue
		}
		rep.Candidates = append(rep.Candidates, Candidate{
			Package: pkg,
			Reason:  "no static reference found — review",
		})
	}

	// Candidates are already in sorted order because unionRequires is sorted,
	// but sort explicitly to be safe.
	sort.Slice(rep.Candidates, func(i, j int) bool {
		return rep.Candidates[i].Package < rep.Candidates[j].Package
	})

	return rep
}

// unionRequires merges two sorted, deduplication-ready package slices (e.g.
// DirectRequires and DirectDevRequires) into a single sorted, deduplicated
// slice. Either argument may be nil.
func unionRequires(a, b []string) []string {
	if len(b) == 0 {
		return a
	}
	if len(a) == 0 {
		return b
	}

	seen := make(map[string]struct{}, len(a)+len(b))
	for _, p := range a {
		seen[p] = struct{}{}
	}
	for _, p := range b {
		seen[p] = struct{}{}
	}

	merged := make([]string, 0, len(seen))
	for p := range seen {
		merged = append(merged, p)
	}
	sort.Strings(merged)
	return merged
}
