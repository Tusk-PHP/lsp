package symbols

import "sort"

type builtinConstant struct {
	Name     string
	Type     string
	Value    string
	Warnable bool
	BuiltinAvailability
}

func lookupWarnableConstant(name string) (BuiltinAvailability, bool) {
	i := sort.Search(len(generatedBuiltinConstants), func(i int) bool { return generatedBuiltinConstants[i].Name >= name })
	if i == len(generatedBuiltinConstants) || generatedBuiltinConstants[i].Name != name || !generatedBuiltinConstants[i].Warnable {
		return BuiltinAvailability{}, false
	}
	return generatedBuiltinConstants[i].BuiltinAvailability, true
}
