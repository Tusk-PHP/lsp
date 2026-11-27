package laravel

import (
	"path/filepath"
	"slices"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/lexer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

type routeFile struct {
	uri      string
	path     string
	src      string
	lines    ast.Lines
	tree     *ast.File
	includes []string
	reads    map[routeGroup]*routeFileRead
}

type routeInclude struct {
	path  string
	group routeGroup
}

type routeFileRead struct {
	routes   []RouteName
	includes []routeInclude
}

func (f *routeFile) read(g routeGroup, rootPath string) *routeFileRead {
	if read, ok := f.reads[g]; ok {
		return read
	}
	read := &routeFileRead{}
	r := routeReader{file: f, out: read, rootPath: rootPath}
	for _, stmt := range f.tree.Stmts {
		r.node(stmt, g)
	}
	if f.reads == nil {
		f.reads = make(map[routeGroup]*routeFileRead)
	}
	f.reads[g] = read
	return read
}

func routeFilePath(uri string) string {
	return filepath.Clean(symbols.URIToPath(uri))
}

func parseRouteFile(uri, source, rootPath string) *routeFile {
	f := &routeFile{
		uri:   uri,
		path:  routeFilePath(uri),
		src:   source,
		lines: ast.NewLines(source),
		tree:  phpparser.Parse(source, token.Latest),
	}
	for _, stmt := range f.tree.Stmts {
		ast.Walk(stmt, func(n ast.Node) bool {
			for _, e := range f.includedExprs(n) {
				if path, ok := f.includePath(e, rootPath); ok && !slices.Contains(f.includes, path) {
					f.includes = append(f.includes, path)
				}
			}
			return true
		})
	}
	return f
}

func (f *routeFile) includedExprs(n ast.Node) []ast.Expr {
	switch n := n.(type) {
	case *ast.Include:
		return []ast.Expr{n.Operand}
	case *ast.MethodCall:
		return f.groupArgs(n.Name, n.Args)
	case *ast.StaticCall:
		return f.groupArgs(n.Name, n.Args)
	}
	return nil
}

func (f *routeFile) groupArgs(name ast.Expr, args *ast.ArgList) []ast.Expr {
	id, ok := name.(*ast.IdentifierExpr)
	if !ok || args == nil || id.Span.Text(f.src) != "group" {
		return nil
	}
	values := make([]ast.Expr, 0, len(args.Args))
	for _, a := range args.Args {
		values = append(values, a.Value)
	}
	return values
}

func (f *routeFile) includePath(e ast.Expr, rootPath string) (string, bool) {
	path, ok := f.pathValue(e, rootPath)
	if !ok || !filepath.IsAbs(path) || filepath.Ext(path) != ".php" {
		return "", false
	}
	return filepath.Clean(path), true
}

func (f *routeFile) pathValue(e ast.Expr, rootPath string) (string, bool) {
	switch e := e.(type) {
	case *ast.Literal:
		if e.Kind != token.ConstantString {
			return "", false
		}
		return lexer.Unquote(e.Span.Text(f.src))
	case *ast.MagicConst:
		if e.Kind != token.DirConst {
			return "", false
		}
		return filepath.Dir(f.path), true
	case *ast.Binary:
		if e.Op != token.Dot {
			return "", false
		}
		left, ok := f.pathValue(e.Left, rootPath)
		if !ok {
			return "", false
		}
		right, ok := f.pathValue(e.Right, rootPath)
		return left + right, ok
	case *ast.Call:
		callee, ok := e.Callee.(*ast.ConstFetch)
		if !ok || callee.Name == nil || rootPath == "" || !strings.EqualFold(strings.TrimPrefix(callee.Name.Span.Text(f.src), `\`), "base_path") || e.Args == nil {
			return "", false
		}
		switch len(e.Args.Args) {
		case 0:
			return rootPath, true
		case 1:
			if e.Args.Args[0].Name != nil || e.Args.Args[0].Unpack {
				return "", false
			}
			sub, ok := f.pathValue(e.Args.Args[0].Value, rootPath)
			return filepath.Join(rootPath, sub), ok
		}
	}
	return "", false
}

type routeSession struct {
	rootPath string
	files    map[string]*routeFile
	reading  map[string]bool
	used     map[string]map[routeGroup]bool
	routes   []RouteName
}

func readRouteFiles(files map[string]*routeFile, rootPath string) []RouteName {
	paths := make([]string, 0, len(files))
	included := make(map[string]bool)
	for path, f := range files {
		paths = append(paths, path)
		for _, target := range f.includes {
			if target != path {
				included[target] = true
			}
		}
	}
	slices.Sort(paths)

	s := routeSession{rootPath: rootPath, files: files, reading: make(map[string]bool), used: make(map[string]map[routeGroup]bool)}
	for _, path := range paths {
		if !included[path] {
			s.read(path, routeGroup{})
		}
	}
	for _, path := range paths {
		if len(s.used[path]) == 0 {
			s.read(path, routeGroup{})
		}
	}
	for path, f := range files {
		for g := range f.reads {
			if !s.used[path][g] {
				delete(f.reads, g)
			}
		}
	}
	return s.routes
}

func (s *routeSession) read(path string, g routeGroup) {
	f := s.files[path]
	if f == nil || s.reading[path] {
		return
	}
	s.reading[path] = true
	if s.used[path] == nil {
		s.used[path] = make(map[routeGroup]bool)
	}
	s.used[path][g] = true
	read := f.read(g, s.rootPath)
	s.routes = append(s.routes, read.routes...)
	for _, include := range read.includes {
		s.read(include.path, include.group)
	}
	delete(s.reading, path)
}

func (r *routeReader) include(e ast.Expr, g routeGroup) bool {
	path, ok := r.file.includePath(e, r.rootPath)
	if !ok {
		return false
	}
	r.out.includes = append(r.out.includes, routeInclude{path, g})
	return true
}

func parseRouteDefinitions(uri, source string) []RouteName {
	f := parseRouteFile(uri, source, "")
	return readRouteFiles(map[string]*routeFile{f.path: f}, "")
}
