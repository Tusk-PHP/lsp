package checks

import (
	"cmp"
	"slices"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type callKind int

const (
	callFunction callKind = iota
	callStatic
	callMethod
	callConstructor
)

type callee struct {
	sym      *symbols.Symbol
	forwards bool
}

func (c callee) checkable() bool {
	return c.sym.Source != symbols.SourceBuiltin && !hasVariadicParam(c.sym.Params) && !c.forwards
}

func allCheckable(callees []callee) bool {
	for _, c := range callees {
		if !c.checkable() {
			return false
		}
	}
	return true
}

type resolvedCall struct {
	kind    callKind
	callees []callee
	name    ast.Node
	args    *ast.ArgList
}

type callSite struct {
	kind   callKind
	name   ast.Node
	args   *ast.ArgList
	member memberReference
}

func eachResolvedCall(syntax *fileSyntax, file *parser.FileNode, source string, index *symbols.Index, typeResolver TypeResolver, wants func(*ast.ArgList) bool, visit func(resolvedCall)) {
	refs := syntax.references()
	sites := make([]callSite, 0, len(refs.calls)+len(refs.constructions)+len(refs.members))
	for _, c := range refs.calls {
		sites = append(sites, callSite{kind: callFunction, name: c.name, args: c.args})
	}
	for _, c := range refs.constructions {
		sites = append(sites, callSite{kind: callConstructor, name: c.name, args: c.args})
	}
	for _, m := range refs.members {
		if !m.call() {
			continue
		}
		kind := callMethod
		if m.static {
			kind = callStatic
		}
		sites = append(sites, callSite{kind: kind, name: m.name, args: m.args, member: m})
	}
	slices.SortFunc(sites, func(a, b callSite) int { return cmp.Compare(a.name.Bounds().Start, b.name.Bounds().Start) })
	for _, site := range sites {
		if !syntax.closed(site.args) || (wants != nil && !wants(site.args)) {
			continue
		}
		if callees := site.callees(syntax, file, source, index, typeResolver); len(callees) > 0 {
			visit(resolvedCall{kind: site.kind, callees: callees, name: site.name, args: site.args})
		}
	}
}

func (s callSite) callees(syntax *fileSyntax, file *parser.FileNode, source string, index *symbols.Index, typeResolver TypeResolver) []callee {
	name := syntax.text(s.name)
	switch s.kind {
	case callFunction:
		fqns, _ := resolve.FunctionNames(name, file)
		return functionCallees(fqns, index)
	case callConstructor:
		classFQN, ok := classNamed(name, syntax.line(s.name), file, index)
		if !ok {
			return nil
		}
		return methodCallees(classFQN, "__construct", index)
	}
	recv, ok := syntax.receiverOf(s.member)
	if !ok {
		return nil
	}
	if s.kind == callStatic {
		if c, isName := recv.node.(*ast.ConstFetch); !isName || c.Name == nil || c.Name.Kind == token.Static {
			return nil
		}
		classFQN, ok := recv.className(file, index)
		if !ok {
			return nil
		}
		return methodCallees(classFQN, name, index)
	}
	if syntax.onThis(recv.node) && syntax.inClosure(s.name) {
		return nil
	}
	owners := recv.classLikes(file, index, typeResolver)
	if len(owners) != 1 {
		return nil
	}
	callees := methodCallees(owners[0], name, index)
	for _, c := range callees {
		if c.sym.IsVirtual {
			return nil
		}
	}
	return callees
}

func (f *fileSyntax) closed(args *ast.ArgList) bool {
	if args.End <= args.Start || f.src[args.End-1] != ')' {
		return false
	}
	return len(args.Args) == 0 || args.Args[len(args.Args)-1].End < args.End
}

func functionCallees(fqns []string, index *symbols.Index) []callee {
	for _, fqn := range fqns {
		var callees []callee
		for _, def := range index.Definitions(fqn) {
			if def.Kind == symbols.KindFunction {
				callees = append(callees, callee{sym: def})
			}
		}
		if len(callees) > 0 {
			return callees
		}
	}
	return nil
}

func methodCallees(classFQN, method string, index *symbols.Index) []callee {
	var callees []callee
	for _, def := range index.Definitions(classFQN) {
		if !def.IsClassLike() {
			continue
		}
		c, ok := definitionMethod(index.DefinitionMembers(def), method)
		if !ok {
			return nil
		}
		callees = append(callees, c)
	}
	return callees
}

func definitionMethod(members []*symbols.Symbol, name string) (callee, bool) {
	var c callee
	for _, m := range members {
		if m.Kind != symbols.KindMethod {
			continue
		}
		if c.sym == nil && m.Name == name {
			c.sym = m
		}
		if m.Name == "__call" || m.Name == "__callStatic" {
			c.forwards = true
		}
	}
	return c, c.sym != nil
}
