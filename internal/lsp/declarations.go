package lsp

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

type declarationKind uint8

const (
	declaredClass declarationKind = iota
	declaredInterface
	declaredTrait
	declaredEnum
	declaredEnumCase
	declaredMethod
	declaredFunction
	declaredProperty
	declaredParameter
)

type declaration struct {
	kind   declarationKind
	name   ast.Span
	node   ast.Span
	static bool
}

func (d declaration) isClassLike() bool {
	return d.kind <= declaredEnum
}

func (d declaration) isCallable() bool {
	return d.kind == declaredMethod || d.kind == declaredFunction
}

func declarationsOf(src string, file *ast.File) []declaration {
	var out []declaration
	add := func(kind declarationKind, name *ast.Identifier, node ast.Span) {
		if name != nil {
			out = append(out, declaration{kind: kind, name: name.Span, node: node})
		}
	}
	ast.Walk(file, func(n ast.Node) bool {
		switch n := n.(type) {
		case *ast.ClassDecl:
			add(declaredClass, n.Name, n.Span)
		case *ast.InterfaceDecl:
			add(declaredInterface, n.Name, n.Span)
		case *ast.TraitDecl:
			add(declaredTrait, n.Name, n.Span)
		case *ast.EnumDecl:
			add(declaredEnum, n.Name, n.Span)
		case *ast.EnumCase:
			add(declaredEnumCase, n.Name, n.Span)
		case *ast.FunctionDecl:
			add(declaredFunction, n.Name, n.Span)
			for _, param := range n.Params {
				out = appendVariable(out, declaredParameter, param.Var)
			}
		case *ast.Method:
			if n.Name == nil {
				return true
			}
			out = append(out, declaration{kind: declaredMethod, name: n.Name.Span, node: n.Span, static: hasModifier(n.Modifiers, token.Static)})
			if strings.EqualFold(n.Name.Text(src), "__construct") {
				for _, param := range n.Params {
					if len(param.Modifiers) > 0 {
						out = appendVariable(out, declaredProperty, param.Var)
					}
				}
			}
		case *ast.Property:
			for _, item := range n.Items {
				out = appendVariable(out, declaredProperty, item.Var)
			}
		}
		return true
	})
	return out
}

func appendVariable(out []declaration, kind declarationKind, v *ast.Variable) []declaration {
	if v == nil {
		return out
	}
	return append(out, declaration{kind: kind, name: v.Span, node: v.Span})
}

func hasModifier(mods []*ast.Modifier, kind token.Kind) bool {
	for _, m := range mods {
		if m.Kind == kind {
			return true
		}
	}
	return false
}
