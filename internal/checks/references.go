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

type references struct {
	classes   []classReference
	calls     []*ast.Name
	constants []*ast.Name
}

func collectReferences(tree *ast.File) *references {
	refs := &references{}
	named := map[*ast.ConstFetch]bool{}
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
		case *ast.StaticCall:
			class(n.Class)
		case *ast.ClassConstFetch:
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
	slices.SortStableFunc(refs.classes, func(a, b classReference) int { return cmp.Compare(a.name.Start, b.name.Start) })
	return refs
}
