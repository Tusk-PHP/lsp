package symbols

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// ClassGenerator supplies classes a framework writes at build time, such as
// Magento's factories and proxies, so they resolve before the project has been
// compiled. The index asks for a class only when a lookup misses, and keeps
// what it gets until the class it was generated from changes.
type ClassGenerator interface {
	// Candidate reports, from the name alone, whether fqn could be generated.
	// It runs on every lookup miss, so it must be cheap.
	Candidate(fqn string) bool
	// Generate returns PHP source declaring fqn and the FQN of the class it is
	// generated from, or ok false when fqn isn't one it can build. It may look
	// symbols up in idx; the index holds no lock while it runs.
	Generate(fqn string, idx *Index) (source, from string, ok bool)
}

type classGeneratorHolder struct{ g ClassGenerator }

// generatedScheme prefixes the URIs generated classes are filed under. They
// are not files: GetAllFileURIs leaves them out, and their symbols point at the
// class they were generated from.
const generatedScheme = "tusk-generated:///"

// SetClassGenerator installs the generator for missing classes, or removes it
// when g is nil.
func (idx *Index) SetClassGenerator(g ClassGenerator) {
	if g == nil {
		idx.generator.Store(nil)
		return
	}
	idx.generator.Store(&classGeneratorHolder{g: g})
}

// ensureGenerated builds the class named by fqn (or owning the member it
// names) when it is missing and the generator can supply it. It reports
// whether it added anything.
func (idx *Index) ensureGenerated(fqn string) bool {
	holder := idx.generator.Load()
	if holder == nil {
		return false
	}
	fqn = strings.TrimPrefix(fqn, "\\")
	if i := strings.Index(fqn, "::"); i >= 0 {
		fqn = fqn[:i]
	}
	if fqn == "" || !holder.g.Candidate(fqn) {
		return false
	}
	idx.mu.RLock()
	exists := idx.symbols[fqn] != nil || len(idx.foldIndex[strings.ToLower(fqn)]) > 0
	idx.mu.RUnlock()
	if exists {
		return false
	}

	source, from, ok := holder.g.Generate(fqn, idx)
	if !ok {
		return false
	}
	file := parser.ParseFile(source)
	if file == nil {
		return false
	}

	idx.mu.Lock()
	defer idx.mu.Unlock()
	if idx.symbols[fqn] != nil {
		return true // built meanwhile, or declared for real
	}
	origin := idx.symbols[from]
	if origin == nil {
		return false // the source class went away meanwhile
	}
	uri := generatedScheme + fqn
	idx.removeFileSymbols(uri)
	idx.generatedURIs[uri] = true
	idx.indexParsedLocked(uri, file, SourceGenerated)
	// The generated file doesn't exist on disk: go-to-definition leads to the
	// class it's generated from.
	for _, sym := range idx.fileSymbols[uri] {
		sym.URI = origin.URI
		sym.Range = origin.Range
	}
	key := strings.ToLower(from)
	idx.generatedFrom[key] = appendUnique(idx.generatedFrom[key], uri)
	return true
}

// dropGeneratedFromLocked removes the classes generated from fqn.
func (idx *Index) dropGeneratedFromLocked(fqn string) {
	key := strings.ToLower(fqn)
	uris, ok := idx.generatedFrom[key]
	if !ok {
		return
	}
	delete(idx.generatedFrom, key)
	for _, uri := range uris {
		idx.removeFileSymbols(uri)
	}
}

// dropGeneratedLocked removes the generated class named fqn, if there is one.
func (idx *Index) dropGeneratedLocked(fqn string) {
	if uri := generatedScheme + fqn; idx.generatedURIs[uri] {
		idx.removeFileSymbols(uri)
	}
}
