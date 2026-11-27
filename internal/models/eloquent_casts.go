package models

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/resolve"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

const (
	castsProperty = "$casts"
	castsMethod   = "casts"
	castDoc       = "Cast: "
)

// injectCasts gives each attribute model's $casts and casts() name the type
// Eloquent casts it to. casts() is merged over $casts, and a later entry
// over an earlier one, as HasAttributes does; members the model already has
// (an accessor, a @property, an IDE helper entry) are left as they are, and
// a property an earlier analysis made from the same cast takes the type the
// cast has now. It returns the classes those types were read from.
func injectCasts(index *symbols.Index, model *symbols.Symbol, source modelSource, decl *ast.ClassDecl, methods map[string]*ast.Method) (dependsOn []string) {
	items := source.castItems(decl, methods)
	if len(items) == 0 {
		return nil
	}
	r := resolve.NewResolver(index)
	lines := ast.NewLines(source.src)
	seen := make(map[string]bool, len(items))
	for i := len(items) - 1; i >= 0; i-- {
		item := items[i]
		name, ok := source.stringValue(item.Key)
		if !ok || name == "" || seen[name] {
			continue
		}
		seen[name] = true
		phpType, classes, ok := source.castType(r, item.Value)
		dependsOn = append(dependsOn, classes...)
		if !ok {
			continue
		}
		propName := "$" + name
		fqn := model.FQN + "::" + propName
		if existing := index.Lookup(fqn); existing != nil {
			if isCastProperty(existing) {
				index.SetPropertyType(fqn, phpType)
			}
			continue
		}
		index.AddVirtualMember(model.FQN, &symbols.Symbol{
			Name:       propName,
			FQN:        fqn,
			Kind:       symbols.KindProperty,
			URI:        model.URI,
			Visibility: "public",
			Type:       phpType,
			IsVirtual:  true,
			DocComment: castDoc + source.text(item.Value),
			Range:      spanRange(lines, item.Key.Bounds()),
		})
	}
	return dependsOn
}

func isCastProperty(sym *symbols.Symbol) bool {
	return sym.IsVirtual && strings.HasPrefix(sym.DocComment, castDoc)
}

func (m modelSource) castItems(decl *ast.ClassDecl, methods map[string]*ast.Method) []*ast.ArrayItem {
	if decl == nil {
		return nil
	}
	var items []*ast.ArrayItem
	for _, member := range decl.Members {
		prop, ok := member.(*ast.Property)
		if !ok {
			continue
		}
		for _, p := range prop.Items {
			if array, isArray := p.Default.(*ast.Array); isArray && p.Var != nil && m.text(p.Var) == castsProperty {
				items = append(items, array.Items...)
			}
		}
	}
	if method := methods[castsMethod]; method != nil && method.Body != nil {
		ast.Walk(method.Body, func(n ast.Node) bool {
			switch n := n.(type) {
			case *ast.Closure, *ast.ArrowFunction:
				return false
			case *ast.Return:
				if array, ok := n.Value.(*ast.Array); ok {
					items = append(items, array.Items...)
				}
				return false
			}
			return true
		})
	}
	return items
}

func (m modelSource) castType(r *resolve.Resolver, value ast.Expr) (phpType string, dependsOn []string, ok bool) {
	if cast, isString := m.stringValue(value); isString {
		if builtin, isBuiltin := frameworklaravel.BuiltinCastType(cast); isBuiltin {
			return builtin, nil, true
		}
		class, arguments, _ := strings.Cut(cast, ":")
		class = strings.TrimPrefix(class, "\\")
		if r.Index.Lookup(class) == nil {
			return "", []string{class}, false
		}
		phpType, dependsOn = frameworklaravel.ClassCastType(r, class, splitCastArguments(arguments))
		return phpType, dependsOn, true
	}
	class, args, isClass := m.classCast(value)
	if !isClass {
		return "", nil, false
	}
	phpType, dependsOn = frameworklaravel.ClassCastType(r, class, args)
	return phpType, dependsOn, true
}

func (m modelSource) classCast(value ast.Expr) (class string, args []string, ok bool) {
	switch v := value.(type) {
	case *ast.ClassConstFetch:
		class = m.classConst(v)
		return class, nil, class != ""
	case *ast.StaticCall:
		named, isName := v.Class.(*ast.ConstFetch)
		if !isName || named.Name == nil || v.Args == nil {
			return "", nil, false
		}
		for _, arg := range v.Args.Args {
			args = append(args, m.classOrString(arg.Value))
		}
		return m.resolve(m.text(named.Name)), args, true
	case *ast.Binary:
		parts := m.concatenated(v)
		fetch, isFetch := parts[0].(*ast.ClassConstFetch)
		if !isFetch || m.classConst(fetch) == "" {
			return "", nil, false
		}
		var rest strings.Builder
		for _, part := range parts[1:] {
			piece := m.classOrString(part)
			if piece == "" {
				return "", nil, false
			}
			rest.WriteString(piece)
		}
		arguments, hasArguments := strings.CutPrefix(rest.String(), ":")
		if !hasArguments {
			return "", nil, false
		}
		return m.classConst(fetch), splitCastArguments(arguments), true
	}
	return "", nil, false
}

func (m modelSource) concatenated(e ast.Expr) []ast.Expr {
	if b, ok := e.(*ast.Binary); ok && b.Op == token.Dot {
		return append(m.concatenated(b.Left), m.concatenated(b.Right)...)
	}
	return []ast.Expr{e}
}

func (m modelSource) classOrString(e ast.Expr) string {
	if fetch, ok := e.(*ast.ClassConstFetch); ok {
		return m.classConst(fetch)
	}
	s, _ := m.stringValue(e)
	return s
}

func splitCastArguments(arguments string) []string {
	if arguments == "" {
		return nil
	}
	args := strings.Split(arguments, ",")
	for i, arg := range args {
		args[i] = strings.TrimPrefix(strings.TrimSpace(arg), "\\")
	}
	return args
}

func (m modelSource) stringValue(e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != token.ConstantString {
		return "", false
	}
	return parser.StringLiteral(m.text(lit))
}

func spanRange(lines ast.Lines, span ast.Span) textpos.Range {
	startLine, startCol := lines.Position(span.Start)
	endLine, endCol := lines.Position(span.End)
	return textpos.Range{
		Start: textpos.Position{Line: startLine, Character: startCol},
		End:   textpos.Position{Line: endLine, Character: endCol},
	}
}
