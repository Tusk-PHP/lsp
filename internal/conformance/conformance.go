//go:build conformance

package conformance

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Tusk-PHP/lsp/internal/analyzer"
	"github.com/Tusk-PHP/lsp/internal/completion"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/hover"
	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// corpusEntry holds a collected PHP file with its URI and source.
type corpusEntry struct {
	path    string // absolute filesystem path
	uri     string // file:// URI used for indexing
	source  string // raw PHP source
	project *corpusProject
}

type corpusProject struct {
	root    string
	profile symbols.BuiltinProfile
}

func (e corpusEntry) grammars() phpparser.GrammarRange {
	return e.project.profile.Grammars
}

// skippedTestdataDir reports whether dir, under the testdata root, holds code
// installed locally for other tests: a fixture's Composer vendor folder
// (testdata/laravel/vendor) or the field-baseline checkouts
// (testdata/field/_repos). corpus/_repos is walked as a root of its own.
func skippedTestdataDir(root, dir string) bool {
	rel, err := filepath.Rel(root, dir)
	if err != nil {
		return false
	}
	parts := strings.Split(filepath.ToSlash(rel), "/")
	return len(parts) == 2 && (parts[1] == "vendor" || parts[0] == "field" && parts[1] == "_repos" || parts[0] == "corpus" && parts[1] == "_repos")
}

// testdataRoot returns the absolute path to the testdata directory.
func testdataRoot() string {
	_, file, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(file), "..", "..", "testdata")
}

// collectCorpus walks testdata/ and, if present, testdata/corpus/_repos/ for
// every .php file that is not a Blade view. Absence of the _repos directory is
// normal (CI only) and never causes an error. Folders that git ignores and CI's
// conformance jobs don't have are left out (see skippedTestdataDir), so a local
// run reads the same files as CI.
func collectCorpus() ([]corpusEntry, error) {
	roots := []string{testdataRoot()}

	reposDir := filepath.Join(testdataRoot(), "corpus", "_repos")
	if info, err := os.Stat(reposDir); err == nil && info.IsDir() {
		roots = append(roots, reposDir)
	}

	var entries []corpusEntry
	seen := make(map[string]bool)
	projects := make(map[string]*corpusProject)
	projectOf := func(root, path string) *corpusProject {
		dir := root
		if rel, err := filepath.Rel(root, path); err == nil {
			if first, _, nested := strings.Cut(filepath.ToSlash(rel), "/"); nested {
				dir = filepath.Join(root, first)
			}
		}
		if p, ok := projects[dir]; ok {
			return p
		}
		profile, _ := workspace.ResolveBuiltinProfileWithConfig(dir, "", 2*time.Second, nil, nil)
		p := &corpusProject{root: dir, profile: profile}
		projects[dir] = p
		return p
	}

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				// Skip unreadable paths gracefully.
				return nil
			}
			if info.IsDir() {
				if root == testdataRoot() && skippedTestdataDir(root, path) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".php") || workspace.IsBlade(path) {
				return nil
			}
			if seen[path] {
				return nil
			}
			seen[path] = true

			src, readErr := os.ReadFile(path)
			if readErr != nil {
				// Skip unreadable files; they are not a harness failure.
				return nil
			}
			uri := pathToURI(path)
			entries = append(entries, corpusEntry{
				path:    path,
				uri:     uri,
				source:  string(src),
				project: projectOf(root, path),
			})
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("walking %s: %w", root, err)
		}
	}
	return entries, nil
}

// pathToURI converts an absolute file-system path to a file:// URI.
func pathToURI(path string) string {
	path = filepath.ToSlash(path)
	if !strings.HasPrefix(path, "/") {
		// Windows drive letter: /C:/...
		path = "/" + path
	}
	return "file://" + path
}

// buildIndexes constructs one symbols.Index per corpus project, with the
// project's own builtin profile and PHP grammars.
func buildIndexes(entries []corpusEntry) map[*corpusProject]*symbols.Index {
	indexes := make(map[*corpusProject]*symbols.Index)
	for _, e := range entries {
		idx, ok := indexes[e.project]
		if !ok {
			idx = symbols.NewIndexFor(e.project.profile.Grammars)
			idx.RegisterBuiltinsForProfile(e.project.profile)
			indexes[e.project] = idx
		}
		idx.IndexFile(e.uri, e.source)
	}
	return indexes
}

// providers bundles all the LSP providers exercised by the harness.
type providers struct {
	ana  *analyzer.Analyzer
	hov  *hover.Provider
	comp *completion.Provider
}

// newProviders creates the provider set from a shared index.
// A fresh, empty ContainerAnalyzer is used (no service provider analysis).
func newProviders(idx *symbols.Index) *providers {
	ca := container.NewContainerAnalyzer(idx, "", "none")
	a := analyzer.NewAnalyzer(idx, ca)
	h := hover.NewProvider(idx, ca, "none")
	c := completion.NewProvider(idx, ca, "none")
	return &providers{ana: a, hov: h, comp: c}
}

// anchorPoint represents a position in a file where we exercise operations.
type anchorPoint struct {
	line int
	col  int
	kind anchorKind
}

type anchorKind int

const (
	anchorArrow       anchorKind = iota // ->
	anchorNullsafe                      // ?->
	anchorDoubleColon                   // ::
	anchorNew                           // new keyword
	anchorClassName                     // class-name identifier
	anchorMethodName                    // method/function name
	anchorUseImport                     // use import short name
)

// extractAnchors scans the token stream of a parse result and returns
// deterministic anchor positions for every operator and name token of interest.
func extractAnchors(source string) []anchorPoint {
	p := parser.New()
	result := p.Parse(source)
	if result == nil {
		return nil
	}

	lines := strings.Split(source, "\n")
	lineCount := len(lines)

	var anchors []anchorPoint

	for i := 0; i < len(result.Tokens); i++ {
		tok := result.Tokens[i]
		line := tok.Line
		if line < 0 || line >= lineCount {
			continue
		}

		switch tok.Kind {
		case parser.TokenArrow:
			// Position cursor just after ->, on the member name.
			if i+1 < len(result.Tokens) {
				next := result.Tokens[i+1]
				if next.Kind == parser.TokenIdentifier || next.Kind == parser.TokenVariable {
					anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorArrow})
				}
			}

		case parser.TokenQuestion:
			// Nullsafe: ?-> — peek for -> immediately after.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenArrow {
				if i+2 < len(result.Tokens) {
					next := result.Tokens[i+2]
					if next.Kind == parser.TokenIdentifier {
						anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorNullsafe})
					}
				}
			}

		case parser.TokenDoubleColon:
			// ::member.
			if i+1 < len(result.Tokens) {
				next := result.Tokens[i+1]
				if next.Kind == parser.TokenIdentifier || next.Kind == parser.TokenVariable {
					anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorDoubleColon})
				}
			}

		case parser.TokenNew:
			// new ClassName — position on the class name.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenIdentifier {
				next := result.Tokens[i+1]
				anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorNew})
			}

		case parser.TokenUse:
			// use Foo\Bar as Baz; — collect the alias (last segment or explicit alias).
			// Skip to the end of the use path, then record the last identifier.
			j := i + 1
			var lastName *parser.Token
			for j < len(result.Tokens) {
				tt := result.Tokens[j]
				if tt.Kind == parser.TokenSemicolon || tt.Kind == parser.TokenEOF {
					break
				}
				if tt.Kind == parser.TokenIdentifier {
					lastName = &result.Tokens[j]
				}
				j++
			}
			if lastName != nil {
				anchors = append(anchors, anchorPoint{line: lastName.Line, col: lastName.Column, kind: anchorUseImport})
			}

		case parser.TokenClass, parser.TokenInterface, parser.TokenTrait, parser.TokenEnum:
			// Structural declaration: record the name token.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenIdentifier {
				next := result.Tokens[i+1]
				anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorClassName})
			}

		case parser.TokenFunction:
			// Function/method name.
			if i+1 < len(result.Tokens) && result.Tokens[i+1].Kind == parser.TokenIdentifier {
				next := result.Tokens[i+1]
				anchors = append(anchors, anchorPoint{line: next.Line, col: next.Column, kind: anchorMethodName})
			}
		}
	}

	return anchors
}

// operationResults holds the outputs of exercising all operations at one anchor.
// FindReferences is excluded from the per-anchor loop because its implementation
// scans every indexed file from disk (O(corpus) per call), making it too slow to
// run at every anchor point across the full corpus. It is still crash-tested via
// checkFindReferencesSafe separately.
type operationResults struct {
	definition  *protocol.Location
	docSymbols  []protocol.DocumentSymbol
	hover       *protocol.Hover
	completions []protocol.CompletionItem
	sigHelp     *protocol.SignatureHelp
}

// runOperations exercises provider operations at anchor pos, catching panics.
// The kind parameter controls which operations are attempted: hover and completion
// run on use-import and member-access anchors; definition, document symbols, and
// signature help run on all anchors.
func runOperations(uri, source string, pos protocol.Position, kind anchorKind, prov *providers) (res operationResults, panicErr error) {
	defer func() {
		if r := recover(); r != nil {
			panicErr = fmt.Errorf("panic at %s line %d col %d: %v", uri, pos.Line, pos.Character, r)
		}
	}()

	res.definition = prov.ana.FindDefinition(uri, source, pos)
	res.docSymbols = prov.ana.GetDocumentSymbols(uri, source)
	res.sigHelp = prov.ana.GetSignatureHelp(uri, source, pos)

	// Hover and completion are exercised on use-import anchors and on all
	// member-access anchors (->, ?->, ::). The chain-resolver re-entrancy guard
	// (C1, fixed in internal/resolve) prevents the fatal stack overflow that
	// previously occurred when AccessChainClass ↔ ResolveVariableType cycled
	// via ChainResolver on self/mutually-referential assignments.
	if kind == anchorUseImport || kind == anchorArrow || kind == anchorNullsafe || kind == anchorDoubleColon {
		res.hover = prov.hov.GetHover(uri, source, pos)
		res.completions = prov.comp.GetCompletions(uri, source, pos)
	}
	return res, nil
}

// isProjectEntry returns true if the corpus entry comes from the small in-repo
// project testdata (testdata/project/), not the larger laravel/symfony fixtures.
func isProjectEntry(e corpusEntry) bool {
	return strings.Contains(e.path, "/testdata/project/") ||
		strings.Contains(e.path, `\testdata\project\`)
}

// maxAnchorsPerFile limits the number of anchor points exercised per file to
// keep the operation phase fast across a large corpus. Anchors are sampled
// deterministically (every N-th anchor).
const maxAnchorsPerFile = 5

// sampleAnchors returns a deterministic subset of at most maxAnchorsPerFile
// anchors. If there are fewer than maxAnchorsPerFile anchors, all are returned.
func sampleAnchors(anchors []anchorPoint) []anchorPoint {
	if len(anchors) <= maxAnchorsPerFile {
		return anchors
	}
	step := len(anchors) / maxAnchorsPerFile
	if step < 1 {
		step = 1
	}
	out := make([]anchorPoint, 0, maxAnchorsPerFile)
	for i := 0; i < len(anchors) && len(out) < maxAnchorsPerFile; i += step {
		out = append(out, anchors[i])
	}
	return out
}
