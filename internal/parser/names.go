package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
)

func ResolveImportedName(name, namespace string, uses []UseNode, kind string) string {
	if name == "" {
		return ""
	}
	if fqn, ok := strings.CutPrefix(name, "\\"); ok {
		return fqn
	}
	first, rest, qualified := strings.Cut(name, "\\")
	if qualified && strings.EqualFold(first, "namespace") {
		return inNamespace(namespace, rest)
	}
	for _, u := range uses {
		if u.Kind != kind || !strings.EqualFold(u.Alias, first) {
			continue
		}
		if qualified {
			return u.FullName + "\\" + rest
		}
		return u.FullName
	}
	return inNamespace(namespace, name)
}

func inNamespace(namespace, name string) string {
	if namespace == "" {
		return name
	}
	return namespace + "\\" + name
}

func (f *FileNode) ResolveClassName(name string) string {
	return ResolveImportedName(name, f.Namespace, f.Uses, "class")
}

func DeclaredClasses(src string, tree *ast.File) map[string]*ast.ClassDecl {
	classes := map[string]*ast.ClassDecl{}
	namespace := ""
	var visit func(stmts []ast.Stmt)
	visit = func(stmts []ast.Stmt) {
		for _, stmt := range stmts {
			switch n := stmt.(type) {
			case *ast.Namespace:
				namespace = ""
				if n.Name != nil {
					namespace = n.Name.Bounds().Text(src)
				}
				visit(n.Stmts)
			case *ast.ClassDecl:
				if n.Name != nil {
					classes[ResolveImportedName(n.Name.Bounds().Text(src), namespace, nil, "class")] = n
				}
			}
		}
	}
	visit(tree.Stmts)
	return classes
}
