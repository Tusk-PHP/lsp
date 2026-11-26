package container

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	sourcectx "github.com/Tusk-PHP/lsp/internal/source"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var (
	ResolvingFunctions = []string{"app", "resolve"}
	ResolvingMethods   = []string{"get", "make"}
)

const (
	configHelper     = "config"
	configBinding    = "config"
	configRepository = `Illuminate\Config\Repository`
)

func NewEngine(index *symbols.Index, ca *ContainerAnalyzer) *resolve.Engine {
	e := resolve.NewEngine(index)
	if ca != nil {
		e.ContainerCall = ca.CallType
	}
	return e
}

func (ca *ContainerAnalyzer) CallType(r *resolve.Resolver, src string, file *parser.FileNode, call ast.Expr) (resolve.ResolvedType, bool) {
	var args *ast.ArgList
	switch n := call.(type) {
	case *ast.Call:
		name := sourcectx.FunctionName(src, n)
		if name == configHelper {
			return ca.configType(n.Args)
		}
		if !slices.Contains(ResolvingFunctions, name) {
			return resolve.ResolvedType{}, false
		}
		args = n.Args
	case *ast.MethodCall:
		if !slices.Contains(ResolvingMethods, sourcectx.MethodName(src, n)) {
			return resolve.ResolvedType{}, false
		}
		args = n.Args
	default:
		return resolve.ResolvedType{}, false
	}
	if len(args.Args) == 0 {
		return resolve.ResolvedType{}, false
	}
	abstract, ok := abstractOf(r, src, file, args.Args[0].Value)
	if !ok {
		return resolve.ResolvedType{}, false
	}
	if binding := ca.ResolveDependency(abstract); binding != nil {
		if binding.Concrete == binding.Abstract && !ca.isClass(binding.Concrete) {
			return resolve.ResolvedType{}, true
		}
		return resolve.ResolvedType{FQN: binding.Concrete}, true
	}
	if ca.isClass(abstract) {
		return resolve.ResolvedType{FQN: abstract}, true
	}
	return resolve.ResolvedType{}, false
}

func (ca *ContainerAnalyzer) isClass(name string) bool {
	sym := ca.index.Lookup(name)
	return sym != nil && sym.IsClassLike()
}

func (ca *ContainerAnalyzer) configType(args *ast.ArgList) (resolve.ResolvedType, bool) {
	if len(args.Args) > 0 {
		return resolve.ResolvedType{}, true
	}
	if binding := ca.ResolveDependency(configBinding); binding != nil {
		return resolve.ResolvedType{FQN: binding.Concrete}, true
	}
	return resolve.ResolvedType{FQN: configRepository}, true
}

func abstractOf(r *resolve.Resolver, src string, file *parser.FileNode, e ast.Expr) (string, bool) {
	if value, ok := sourcectx.StringValue(src, e); ok {
		return value, value != ""
	}
	fetch, ok := e.(*ast.ClassConstFetch)
	if !ok || !strings.EqualFold(fetch.Name.Bounds().Text(src), "class") {
		return "", false
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok {
		return "", false
	}
	return r.ResolveClassNameLoose(class.Bounds().Text(src), file), true
}
