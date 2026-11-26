package sourcectx

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func ClassNamesEndingIn(src string, lines ast.Lines, tree *ast.File, short string) map[uint32]string {
	c := classNames{src: src, short: short, namespaces: NamespacesOf(src, lines, tree), names: map[uint32]string{}, classPositions: map[*ast.ConstFetch]bool{}}
	ast.Walk(tree, c.visit)
	return c.names
}

type classNames struct {
	src            string
	short          string
	namespaces     *Namespaces
	names          map[uint32]string
	classPositions map[*ast.ConstFetch]bool
	declaration    *ast.Name
}

func (c *classNames) visit(n ast.Node) bool {
	switch n := n.(type) {
	case *ast.Namespace:
		c.declaration = n.Name
	case *ast.Use:
		c.imports(n)
		return false
	case *ast.New:
		c.inClassPosition(n.Class)
	case *ast.ClassConstFetch:
		c.inClassPosition(n.Class)
	case *ast.StaticCall:
		c.inClassPosition(n.Class)
	case *ast.StaticPropertyFetch:
		c.inClassPosition(n.Class)
	case *ast.InstanceOf:
		c.inClassPosition(n.Class)
	case *ast.ConstFetch:
		return c.classPositions[n]
	case *ast.ClassDecl:
		c.declared(n.Name)
	case *ast.InterfaceDecl:
		c.declared(n.Name)
	case *ast.TraitDecl:
		c.declared(n.Name)
	case *ast.EnumDecl:
		c.declared(n.Name)
	case *ast.Name:
		if n != c.declaration {
			c.record(n.Span, c.namespaces.At(n.Start).ResolveClass(c.src, n))
		}
	}
	return true
}

func (c *classNames) inClassPosition(class ast.Expr) {
	if fetch, ok := class.(*ast.ConstFetch); ok {
		c.classPositions[fetch] = true
	}
}

func (c *classNames) declared(name *ast.Identifier) {
	if name != nil {
		c.record(name.Span, parser.ResolveImportedName(name.Text(c.src), c.namespaces.At(name.Start).Name, nil, "class"))
	}
}

func (c *classNames) imports(use *ast.Use) {
	prefix := ""
	if use.Prefix != nil {
		prefix = use.Prefix.Text(c.src) + "\\"
	}
	for _, item := range use.Items {
		if item.Name == nil {
			continue
		}
		fqn := ""
		if item.Kind == token.Use {
			fqn = strings.TrimPrefix(prefix+item.Name.Text(c.src), "\\")
		}
		c.record(item.Name.Span, fqn)
	}
}

func (c *classNames) record(span ast.Span, fqn string) {
	text := span.Text(c.src)
	if len(text) < len(c.short) {
		return
	}
	last := len(text) - len(c.short)
	if !strings.EqualFold(text[last:], c.short) || last > 0 && text[last-1] != '\\' {
		return
	}
	c.names[span.Start+uint32(last)] = fqn
}
