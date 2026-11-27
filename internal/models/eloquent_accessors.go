package models

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/types"
)

const attributeGetter = "get"

var attributeFactories = map[string]bool{"make": true, attributeGetter: true}

// attributeType is the type of the property an accessor returning Attribute
// makes: TGet of the Attribute<TGet, TSet> its PHPDoc returns, else what
// its PHPDoc returns when that isn't Attribute itself, else the return type
// of the get closure it builds the Attribute with, else mixed.
func (m modelSource) attributeType(method parser.MethodNode, decl *ast.Method) string {
	if method.DocComment != "" {
		if doc := parser.ParseDocBlock(method.DocComment); doc != nil && doc.Return.Type != "" {
			returned := m.qualifiedType(doc.Return.Type)
			switch {
			case returned.Kind == types.TypeKindGeneric && returned.Name == frameworklaravel.EloquentAttribute && len(returned.Params) > 0:
				return returned.Params[0].String()
			case returned.Name != frameworklaravel.EloquentAttribute:
				return returned.String()
			}
		}
	}
	if getter := m.attributeGetter(decl); getter != "" {
		return m.qualifiedType(getter).String()
	}
	return "mixed"
}

// qualifiedType parses a type written in the model's file, with each class
// name in it resolved through the file's imports and namespace.
func (m modelSource) qualifiedType(written string) types.TypeExpr {
	return types.ParseTypeExpr(written).MapNames(func(name string) string {
		if resolve.IsBuiltinTypeName(name) {
			return name
		}
		return m.resolve(name)
	})
}

// attributeGetter is the return type declared by the get closure decl's
// body passes to Attribute::make(), Attribute::get() or new Attribute(),
// as its get: argument or its first.
func (m modelSource) attributeGetter(decl *ast.Method) string {
	if decl == nil || decl.Body == nil {
		return ""
	}
	getter := ""
	ast.Walk(decl.Body, func(n ast.Node) bool {
		if getter != "" {
			return false
		}
		var class ast.Expr
		var args *ast.ArgList
		switch n := n.(type) {
		case *ast.StaticCall:
			if name, ok := n.Name.(*ast.IdentifierExpr); !ok || !attributeFactories[strings.ToLower(m.text(name))] {
				return true
			}
			class, args = n.Class, n.Args
		case *ast.New:
			class, args = n.Class, n.Args
		default:
			return true
		}
		if named, ok := class.(*ast.ConstFetch); !ok || named.Name == nil || m.resolve(m.text(named.Name)) != frameworklaravel.EloquentAttribute || args == nil {
			return true
		}
		getter = m.closureReturnType(m.attributeGetterArg(args))
		return getter == ""
	})
	return getter
}

func (m modelSource) attributeGetterArg(args *ast.ArgList) ast.Expr {
	for i, arg := range args.Args {
		if arg.Name != nil {
			if strings.EqualFold(m.text(arg.Name), attributeGetter) {
				return arg.Value
			}
			continue
		}
		if i == 0 {
			return arg.Value
		}
	}
	return nil
}

func (m modelSource) closureReturnType(e ast.Expr) string {
	switch fn := e.(type) {
	case *ast.Closure:
		if fn.ReturnType != nil {
			return m.text(fn.ReturnType)
		}
	case *ast.ArrowFunction:
		if fn.ReturnType != nil {
			return m.text(fn.ReturnType)
		}
	}
	return ""
}
