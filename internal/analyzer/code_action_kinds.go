package analyzer

import "strings"

func codeActionKindAllowed(only []string, kind string) bool {
	if len(only) == 0 {
		return true
	}
	for _, requested := range only {
		if kind == requested || strings.HasPrefix(kind, requested+".") {
			return true
		}
	}
	return false
}
