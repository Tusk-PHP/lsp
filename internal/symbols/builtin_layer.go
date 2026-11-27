package symbols

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/stubs"
)

const builtinRegistryURI = "builtin"

func isBuiltinURI(uri string) bool {
	return uri == builtinRegistryURI || strings.HasPrefix(uri, "builtin://")
}

func (p BuiltinProfile) Equal(other BuiltinProfile) bool {
	a, b := normalizeBuiltinProfile(p), normalizeBuiltinProfile(other)
	return a.PHPVersion == b.PHPVersion && slices.Equal(a.Extensions, b.Extensions)
}

func (idx *Index) ReplaceBuiltins(profile BuiltinProfile) error {
	profile = normalizeBuiltinProfile(profile)
	entries, err := stubs.BuiltinPHPForProfile(stubs.Profile{PHPVersion: profile.PHPVersion, Extensions: profile.Extensions})
	if err != nil {
		return err
	}
	uris := make([]string, len(entries))
	decls := make([][]*Symbol, len(entries))
	for i, entry := range entries {
		uris[i] = "builtin://" + entry.Path
		if file := parser.ParseFile(entry.Content); file != nil {
			decls[i] = fileDeclarations(uris[i], file)
		}
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for uri := range idx.fileSymbols {
		if isBuiltinURI(uri) {
			idx.removeFileSymbols(uri)
		}
	}
	idx.registerBuiltinRegistryLocked(profile)
	for i, entry := range entries {
		if decls[i] == nil {
			continue
		}
		idx.fileSource[uris[i]] = indexedText{text: entry.Content, from: SourceBuiltin}
		idx.addDeclarationsLocked(uris[i], decls[i], SourceBuiltin)
	}
	return nil
}

func (idx *Index) declaredOverBuiltinLocked(sym *Symbol) bool {
	if sym.Source != SourceBuiltin {
		return false
	}
	holder := sym.FQN
	if sym.Kind.IsMember() {
		holder = sym.ParentFQN
	}
	prev := idx.symbols[holder]
	return prev != nil && prev.Source != SourceBuiltin && prev.Source != SourceGenerated
}

func (idx *Index) builtinLocked(fqn string) *Symbol {
	if sym := idx.symbols[fqn]; sym != nil && sym.Source == SourceBuiltin {
		return sym
	}
	if defs := idx.shadowed[fqn]; len(defs) > 0 && defs[0].Source == SourceBuiltin {
		return defs[0]
	}
	return nil
}
