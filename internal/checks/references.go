package checks

import (
	"cmp"
	"slices"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type classReference struct {
	name      *ast.Name
	attribute bool
}

type memberReference struct {
	owner    ast.Expr
	name     *ast.IdentifierExpr
	static   bool
	call     bool
	presence bool
}

type references struct {
	classes   []classReference
	calls     []*ast.Name
	constants []*ast.Name
	members   []memberReference
}

func collectReferences(tree *ast.File) *references {
	refs := &references{}
	named := map[*ast.ConstFetch]bool{}
	present := map[ast.Expr]bool{}
	member := func(n ast.Expr, owner, name ast.Expr, static, call bool) {
		if id, ok := name.(*ast.IdentifierExpr); ok {
			refs.members = append(refs.members, memberReference{owner: owner, name: id, static: static, call: call, presence: present[n]})
		}
	}
	class := func(e ast.Expr) {
		if c, ok := e.(*ast.ConstFetch); ok {
			named[c] = true
			if c.Name != nil {
				refs.classes = append(refs.classes, classReference{name: c.Name})
			}
		}
	}
	classes := func(names ...*ast.Name) {
		for _, n := range names {
			if n != nil {
				refs.classes = append(refs.classes, classReference{name: n})
			}
		}
	}
	ast.Walk(tree, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.Isset:
			for _, v := range n.Vars {
				markPresence(present, v)
			}
		case *ast.Unset:
			for _, v := range n.Vars {
				markPresence(present, v)
			}
		case *ast.Empty:
			markPresence(present, n.Operand)
		case *ast.Binary:
			if n.Op == token.Coalesce {
				present[n.Left] = true
			}
		case *ast.Assign:
			if n.Op == token.CoalesceEqual {
				present[n.Target] = true
			}
		case *ast.PropertyFetch:
			member(n, n.Object, n.Name, false, false)
		case *ast.MethodCall:
			member(n, n.Object, n.Name, false, true)
		case *ast.ClassConstFetch:
			member(n, n.Class, n.Name, true, false)
			class(n.Class)
		case *ast.StaticCall:
			member(n, n.Class, n.Name, true, true)
			class(n.Class)
		case *ast.Call:
			if c, ok := n.Callee.(*ast.ConstFetch); ok {
				named[c] = true
				if c.Name != nil && c.Name.Kind != token.Static {
					refs.calls = append(refs.calls, c.Name)
				}
			}
		case *ast.ConstFetch:
			if !named[n] && n.Name != nil {
				refs.constants = append(refs.constants, n.Name)
			}
		case *ast.New:
			class(n.Class)
		case *ast.InstanceOf:
			class(n.Class)
		case *ast.StaticPropertyFetch:
			class(n.Class)
		case *ast.Catch:
			classes(n.Types...)
		case *ast.ClassDecl:
			classes(n.Extends)
			classes(n.Implements...)
		case *ast.InterfaceDecl:
			classes(n.Extends...)
		case *ast.EnumDecl:
			classes(n.Implements...)
		case *ast.Attribute:
			if n.Name != nil {
				refs.classes = append(refs.classes, classReference{name: n.Name, attribute: true})
			}
		}
		return true
	})
	slices.SortStableFunc(refs.members, func(a, b memberReference) int { return cmp.Compare(a.name.Start, b.name.Start) })
	slices.SortStableFunc(refs.classes, func(a, b classReference) int { return cmp.Compare(a.name.Start, b.name.Start) })
	return refs
}

func markPresence(present map[ast.Expr]bool, e ast.Expr) {
	for e != nil {
		present[e] = true
		switch n := e.(type) {
		case *ast.PropertyFetch:
			e = n.Object
		case *ast.MethodCall:
			e = n.Object
		case *ast.ArrayDimFetch:
			e = n.Array
		default:
			return
		}
	}
}
