package resolve

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func FunctionNames(name string, file *parser.FileNode) ([]string, bool) {
	if file == nil {
		return nil, false
	}
	fqns := FunctionNamesIn(name, file.Namespace, file.Uses)
	return fqns, fqns != nil
}

// FunctionNamesIn lists the functions a call of name in namespace, with uses
// imported, can reach, in the order PHP tries them: the namespaced one, then
// the global one it falls back to.
func FunctionNamesIn(name, namespace string, uses []parser.UseNode) []string {
	name = strings.TrimSpace(name)
	fullyQualified := strings.HasPrefix(name, "\\")
	name = strings.TrimPrefix(name, "\\")
	if name == "" {
		return nil
	}
	if fullyQualified {
		return []string{name}
	}
	if strings.Contains(name, "\\") {
		return []string{ImportedName(name, namespace, uses, "class")}
	}
	for _, u := range uses {
		if u.Kind == "function" && strings.EqualFold(u.Alias, name) {
			return []string{u.FullName}
		}
	}
	if namespace == "" {
		return []string{name}
	}
	return []string{namespace + "\\" + name, name}
}

// CalledFunctionFQN is the function a call of name runs: the first of
// FunctionNamesIn the index has, else the last, which PHP falls back to and
// which may be defined where the index doesn't reach.
func CalledFunctionFQN(index *symbols.Index, name, namespace string, uses []parser.UseNode) string {
	fqns := FunctionNamesIn(name, namespace, uses)
	for _, fqn := range fqns {
		if fn := index.LookupFold(fqn, isFunction); fn != nil {
			return fn.FQN
		}
	}
	if len(fqns) == 0 {
		return ""
	}
	return fqns[len(fqns)-1]
}

func ImportedName(name, currentNs string, uses []parser.UseNode, useKind string) string {
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "\\") {
		return strings.TrimPrefix(name, "\\")
	}
	parts := strings.SplitN(name, "\\", 2)
	for _, u := range uses {
		if useKind != "" && u.Kind != useKind {
			continue
		}
		if !strings.EqualFold(u.Alias, parts[0]) {
			continue
		}
		if len(parts) > 1 {
			return u.FullName + "\\" + parts[1]
		}
		return u.FullName
	}
	if currentNs != "" {
		return currentNs + "\\" + name
	}
	return name
}

func (r *Resolver) CalledFunction(name string, file *parser.FileNode) *symbols.Symbol {
	fqns, _ := FunctionNames(name, file)
	for _, fqn := range fqns {
		if fn := r.Index.LookupFold(fqn, isFunction); fn != nil {
			return fn
		}
	}
	return nil
}

func (r *Resolver) typedFunction(name string, file *parser.FileNode) *symbols.Symbol {
	fqns, _ := FunctionNames(name, file)
	for _, fqn := range fqns {
		if fn := r.Index.LookupFold(fqn, isFunction); fn != nil {
			return fn
		}
		for i := range file.Functions {
			if strings.EqualFold(file.Functions[i].FullName, fqn) {
				return symbols.FunctionSymbol("", file, &file.Functions[i])
			}
		}
	}
	return nil
}

func isFunction(sym *symbols.Symbol) bool {
	return sym.Kind == symbols.KindFunction
}
