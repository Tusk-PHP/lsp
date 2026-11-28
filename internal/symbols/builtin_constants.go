package symbols

import (
	"slices"
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

func lookupBuiltinConstant(fqn string) (builtinConstant, bool) {
	i, ok := slices.BinarySearchFunc(generatedBuiltinConstants, fqn, func(c builtinConstant, fqn string) int {
		return strings.Compare(c.Name, fqn)
	})
	if !ok {
		return builtinConstant{}, false
	}
	return generatedBuiltinConstants[i], true
}
