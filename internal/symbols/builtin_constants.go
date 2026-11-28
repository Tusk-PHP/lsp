package symbols

import (
	"slices"
	"sort"
	"strings"
)

type builtinConstant struct {
	Name      string
	Type      string
	Value     string
	Extension string
	Since     string
	PECL      bool
}

func (c builtinConstant) availability() BuiltinAvailability {
	if a, ok := builtinAvailabilityByName[c.Name]; ok {
		return a
	}
	a := BuiltinAvailability{Since: c.Since}
	if c.PECL {
		a.Extension = c.Extension
	}
	return a
}

func (c builtinConstant) symbol() *Symbol {
	return &Symbol{Name: shortName(c.Name), FQN: c.Name, Kind: KindConstant, Source: SourceBuiltin, URI: builtinRegistryURI, Type: c.Type, Value: c.Value}
}

func compareBuiltinConstantNames(a, b string) int {
	if c := compareFold(a, b); c != 0 {
		return c
	}
	return strings.Compare(a, b)
}

func builtinConstantNamed(fqn string) []builtinConstant {
	i, ok := slices.BinarySearchFunc(generatedBuiltinConstants, fqn, func(c builtinConstant, fqn string) int {
		return compareBuiltinConstantNames(c.Name, fqn)
	})
	if !ok {
		return nil
	}
	return generatedBuiltinConstants[i : i+1]
}

func builtinConstantsWithPrefix(prefix string) []builtinConstant {
	lp := strings.ToLower(prefix)
	start := sort.Search(len(generatedBuiltinConstants), func(i int) bool {
		return compareFold(generatedBuiltinConstants[i].Name, lp) >= 0
	})
	end := start
	for end < len(generatedBuiltinConstants) && hasFoldPrefix(generatedBuiltinConstants[end].Name, lp) {
		end++
	}
	return generatedBuiltinConstants[start:end]
}

func (idx *Index) loadBuiltinConstants(constants []builtinConstant) {
	if len(constants) == 0 {
		return
	}
	idx.mu.RLock()
	missing := slices.ContainsFunc(constants, idx.builtinConstantMissingLocked)
	idx.mu.RUnlock()
	if !missing {
		return
	}
	idx.mu.Lock()
	defer idx.mu.Unlock()
	for _, c := range constants {
		if idx.builtinConstantMissingLocked(c) {
			idx.storeBuiltinLocked(c.symbol())
		}
	}
}

func (idx *Index) builtinConstantMissingLocked(c builtinConstant) bool {
	return idx.builtinProfile != nil && idx.builtinLocked(c.Name) == nil && availableIn(c.availability(), *idx.builtinProfile)
}
