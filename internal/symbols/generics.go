package symbols

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// docTemplates reads a method's or function's @template tags. Most docblocks
// have none, so they're only parsed when the tag is there.
func docTemplates(doc string, resolve func(string) string) []TemplateParam {
	if !strings.Contains(doc, "@template") && !strings.Contains(doc, "-template") {
		return nil
	}
	return templateParams(parser.ParseDocBlock(doc), resolve)
}

// templateParams turns a docblock's @template tags into template parameters,
// with their bounds and defaults resolved like any other type in the file.
func templateParams(doc *parser.DocBlock, resolve func(string) string) []TemplateParam {
	if doc == nil || len(doc.Templates) == 0 {
		return nil
	}
	names := make(map[string]bool, len(doc.Templates))
	for _, tmpl := range doc.Templates {
		names[tmpl.Name] = true
	}
	out := make([]TemplateParam, 0, len(doc.Templates))
	for _, tmpl := range doc.Templates {
		if tmpl.Name == "" {
			continue
		}
		out = append(out, TemplateParam{
			Name:    tmpl.Name,
			Bound:   resolveDocTypeString(tmpl.Bound, resolve, names),
			Default: resolveDocTypeString(tmpl.Default, resolve, names),
		})
	}
	return out
}

// supertypes reads @extends, @implements and @use (and their @template- and
// prefixed forms) into the parents they give type arguments to.
func supertypes(doc *parser.DocBlock, resolve func(string) string) []Supertype {
	if doc == nil || len(doc.Extends)+len(doc.Implements)+len(doc.Uses) == 0 {
		return nil
	}
	var out []Supertype
	for _, group := range [][]string{doc.Extends, doc.Implements, doc.Uses} {
		for _, raw := range group {
			expr := types.ParseTypeExpr(raw)
			if expr.Kind != types.TypeKindGeneric || expr.Name == "" {
				continue
			}
			// The parsed name loses its leading backslash; resolve what was written.
			name := strings.TrimSpace(raw[:strings.IndexByte(raw, '<')])
			st := Supertype{FQN: resolve(name), Args: make([]string, len(expr.Params))}
			for i, p := range expr.Params {
				st.Args[i] = p.String()
			}
			out = append(out, st)
		}
	}
	return out
}

// resolveDocTypeString resolves a PHPDoc type for storage. A plain class name
// is resolved with the file's imports and namespace; anything more (generics,
// unions, shapes) keeps the form its imports were qualified to, as the names
// in it may be templates. Template names stay as they are.
func resolveDocTypeString(t string, resolve func(string) string, templates map[string]bool) string {
	t = strings.TrimSpace(t)
	if t == "" || templates[t] || isPseudoTypeName(t) {
		return t
	}
	if strings.ContainsAny(t, "<>{}()|&,: '\"") {
		return t
	}
	return resolve(t)
}

// isPseudoTypeName reports whether a PHPDoc type is a keyword such as
// array-key, positive-int or list, never a class.
func isPseudoTypeName(t string) bool {
	if phpBuiltinTypes[strings.ToLower(t)] {
		return true
	}
	switch strings.ToLower(t) {
	case "list", "scalar", "numeric", "integer", "boolean", "double", "class-string", "$this":
		return true
	}
	return strings.IndexByte(t, '-') > 0 && strings.IndexByte(t, '\\') < 0
}

// traitUseSupertypes reads `@use Trait<A>` from the docblocks above a class's
// trait `use` statements.
func traitUseSupertypes(doc string, uses []parser.UseNode, resolve func(string) string) []Supertype {
	if doc == "" || !strings.Contains(doc, "use") {
		return nil
	}
	return supertypes(parser.ParseDocBlock(qualifyDocTypes(doc, uses)), resolve)
}
