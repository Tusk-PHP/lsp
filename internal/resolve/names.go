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
	name = strings.TrimSpace(name)
	fullyQualified := strings.HasPrefix(name, "\\")
	name = strings.TrimPrefix(name, "\\")
	if name == "" {
		return nil, false
	}
	if fullyQualified {
		return []string{name}, true
	}
	if strings.Contains(name, "\\") {
		return []string{parser.ResolveImportedName(name, file.Namespace, file.Uses, "class")}, true
	}
	for _, u := range file.Uses {
		if u.Kind == "function" && strings.EqualFold(u.Alias, name) {
			return []string{u.FullName}, true
		}
	}
	var fqns []string
	if file.Namespace != "" {
		fqns = append(fqns, file.Namespace+"\\"+name)
	}
	fqns = append(fqns, name)
	return fqns, true
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
