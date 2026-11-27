package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/types"
)

func (c *importCollector) markDocComment(comment string) {
	c.markInlineDocTags(comment)
	var tag string
	for line := range docLines(comment) {
		if c.pending == 0 {
			return
		}
		if strings.HasPrefix(line, "@") {
			c.markDocTag(tag)
			tag = line[1:]
			continue
		}
		if tag != "" {
			tag += " " + line
		}
	}
	c.markDocTag(tag)
}

func (c *importCollector) markInlineDocTags(comment string) {
	for rest := comment; c.pending > 0; {
		start := strings.Index(rest, "{@")
		if start < 0 {
			return
		}
		rest = rest[start+2:]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return
		}
		switch name, value := splitDocTag(rest[:end]); name {
		case "see", "link":
			c.markDocTarget(value)
		}
		rest = rest[end+1:]
	}
}

func (c *importCollector) markDocTag(tag string) {
	if tag == "" || c.pending == 0 {
		return
	}
	name, value := splitDocTag(tag)
	base, prio := docTagName(name)
	switch base {
	case "param", "var", "param-out", "param-closure-this", "property", "property-read", "property-write":
		c.markDocType(docVariableType(value))
	case "type":
		if prio == 0 {
			c.markDocType(docVariableType(value))
			return
		}
		c.markDocType(parseDocTypeAlias(value).Type)
	case "return", "throws", "self-out", "mixin", "extends", "implements", "use", "require-extends", "require-implements":
		typ, _ := types.ExtractDocTypeString(value)
		c.markDocType(typ)
	case "assert", "assert-if-true", "assert-if-false":
		c.markDocType(parseDocAssert(value, AssertAlways).Type)
	case "template":
		tmpl := parseDocTemplate(value)
		c.markDocType(tmpl.Bound)
		c.markDocType(tmpl.Default)
	case "import-type":
		c.markDocType(parseDocImportedTypeAlias(value).From)
	case "method":
		c.markDocMethod(value)
	case "see", "link", "covers", "coversDefaultClass", "uses":
		c.markDocTarget(value)
	default:
		if isDocAnnotation(name) {
			tokens, _ := tokenizeFrom(tag, false)
			c.collect(tokens, nil)
		}
	}
}

func docVariableType(value string) string {
	if strings.HasPrefix(value, "$") {
		end := strings.IndexAny(value, " \t")
		if end < 0 {
			return ""
		}
		value = value[end+1:]
	}
	typ, _ := types.ExtractDocTypeString(value)
	return typ
}

func (c *importCollector) markDocMethod(value string) {
	method := parseDocMethod(value)
	c.markDocType(method.ReturnType)
	for _, param := range method.Params {
		c.markDocType(param.Type)
		if param.Default != "" {
			tokens, _ := tokenizeFrom(param.Default, false)
			c.collect(tokens, nil)
		}
	}
}

func (c *importCollector) markDocTarget(value string) {
	target := value
	if end := strings.IndexAny(value, " \t"); end >= 0 {
		target = value[:end]
	}
	if target == "" || strings.Contains(target, "://") {
		return
	}
	tokens, _ := tokenizeFrom(target, false)
	c.collect(tokens, nil)
}

func isDocAnnotation(name string) bool {
	return name != "" && ('A' <= name[0] && name[0] <= 'Z' || strings.IndexByte(name, '\\') > 0)
}

func (c *importCollector) markDocType(raw string) {
	if raw == "" {
		return
	}
	if isPlainDocName(raw) {
		c.markDocTypeName(raw)
		return
	}
	c.markTypeExpr(types.ParseTypeExpr(raw))
}

func isPlainDocName(raw string) bool {
	for i := 0; i < len(raw); i++ {
		if !IsIdentByte(raw[i]) && raw[i] != '\\' {
			return false
		}
	}
	return true
}

func (c *importCollector) markTypeExpr(expr types.TypeExpr) {
	c.markDocTypeName(expr.Name)
	if expr.Element != nil {
		c.markTypeExpr(*expr.Element)
	}
	if expr.ReturnType != nil {
		c.markTypeExpr(*expr.ReturnType)
	}
	if cond := expr.Condition; cond != nil {
		c.markTypeExpr(cond.Target)
		c.markTypeExpr(cond.IfTrue)
		c.markTypeExpr(cond.IfFalse)
	}
	for _, group := range [...][]types.TypeExpr{expr.Params, expr.Types, expr.ParamTypes} {
		for _, inner := range group {
			c.markTypeExpr(inner)
		}
	}
	for _, field := range expr.ShapeFields {
		c.markTypeExpr(field.Type)
	}
	for _, tmpl := range expr.Templates {
		if tmpl.Bound != nil {
			c.markTypeExpr(*tmpl.Bound)
		}
	}
}

func (c *importCollector) markDocTypeName(name string) {
	end := 0
	for end < len(name) && (IsIdentByte(name[end]) || name[end] == '\\') {
		end++
	}
	if end < len(name) && strings.IndexByte(" .:&", name[end]) < 0 {
		return
	}
	name = name[:end]
	if name == "" || name[0] == '\\' {
		return
	}
	if first, _, qualified := strings.Cut(name, "\\"); qualified {
		name = first
	}
	c.mark(name, classImport)
}
