package parser

import "strings"

func ResolveImportedName(name, namespace string, uses []UseNode, kind string) string {
	if name == "" {
		return ""
	}
	if fqn, ok := strings.CutPrefix(name, "\\"); ok {
		return fqn
	}
	first, rest, qualified := strings.Cut(name, "\\")
	for _, u := range uses {
		if u.Kind != kind || !strings.EqualFold(u.Alias, first) {
			continue
		}
		if qualified {
			return u.FullName + "\\" + rest
		}
		return u.FullName
	}
	if namespace != "" {
		return namespace + "\\" + name
	}
	return name
}

func (f *FileNode) ResolveClassName(name string) string {
	return ResolveImportedName(name, f.Namespace, f.Uses, "class")
}
