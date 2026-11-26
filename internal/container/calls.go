package container

import (
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel"
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
	configBinding    = "config"
	configRepository = `Illuminate\Config\Repository`
)

type Abstract struct {
	Name  string
	Class bool
}

func NewEngine(index *symbols.Index, ca *ContainerAnalyzer) *resolve.Engine {
	e := resolve.NewEngine(index)
	if ca != nil {
		e.ContainerCall = ca.CallType
	}
	return e
}

func (ca *ContainerAnalyzer) CallType(r *resolve.Resolver, src string, file *parser.FileNode, call ast.Expr) (resolve.ResolvedType, bool) {
	if cfg, ok := laravel.ParseConfigCall(src, call); ok {
		return ca.configType(cfg)
	}
	args := ResolvingArgs(src, call)
	if args == nil || len(args.Args) == 0 {
		return resolve.ResolvedType{}, false
	}
	abstract, ok := AbstractOf(r, src, file, args.Args[0].Value)
	if !ok {
		return resolve.ResolvedType{}, false
	}
	if binding := ca.ResolveDependency(abstract.Name); binding != nil {
		if binding.Concrete == binding.Abstract && !ca.isClass(binding.Concrete) {
			return resolve.ResolvedType{}, true
		}
		return resolve.ResolvedType{FQN: binding.Concrete}, true
	}
	if ca.isClass(abstract.Name) {
		return resolve.ResolvedType{FQN: abstract.Name}, true
	}
	return resolve.ResolvedType{}, false
}

func ResolvingArgs(src string, call ast.Expr) *ast.ArgList {
	switch n := call.(type) {
	case *ast.Call:
		if slices.Contains(ResolvingFunctions, sourcectx.FunctionName(src, n)) {
			return n.Args
		}
	case *ast.MethodCall:
		if slices.Contains(ResolvingMethods, sourcectx.MethodName(src, n)) {
			return n.Args
		}
	}
	return nil
}

func (ca *ContainerAnalyzer) isClass(name string) bool {
	sym := ca.index.Lookup(name)
	return sym != nil && sym.IsClassLike()
}

func (ca *ContainerAnalyzer) configType(cfg laravel.ConfigCall) (resolve.ResolvedType, bool) {
	if !cfg.Repository {
		return resolve.ResolvedType{}, true
	}
	if binding := ca.ResolveDependency(configBinding); binding != nil {
		return resolve.ResolvedType{FQN: binding.Concrete}, true
	}
	return resolve.ResolvedType{FQN: configRepository}, true
}

func AbstractOf(r *resolve.Resolver, src string, file *parser.FileNode, e ast.Expr) (Abstract, bool) {
	if value, ok := sourcectx.StringValue(src, e); ok {
		return Abstract{Name: value}, value != ""
	}
	fetch, ok := e.(*ast.ClassConstFetch)
	if !ok || !strings.EqualFold(fetch.Name.Bounds().Text(src), "class") {
		return Abstract{}, false
	}
	class, ok := fetch.Class.(*ast.ConstFetch)
	if !ok {
		return Abstract{}, false
	}
	return Abstract{Name: r.ResolveClassNameLoose(class.Bounds().Text(src), file), Class: true}, true
}
