package parser

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/types"
)

func (c *importCollector) markDocComment(comment string) {
	c.markInlineDocTags(comment)
	annotation := false
	for line := range docLines(comment) {
		if c.pending == 0 {
			return
		}
		if !line.isTag {
			if annotation {
				c.markDocCode(line.text)
			}
			continue
		}
		annotation = isDocAnnotation(line.tag.Name)
		c.markDocTag(line.tag)
	}
}

func (c *importCollector) markDocCode(code string) {
	tokens, _ := tokenizeFrom(code, false)
	c.collect(tokens, nil)
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

func (c *importCollector) markDocTag(tag DocTag) {
	name, value := tag.Name, tag.Value
	base, prio := docTagName(name)
	switch docTagSpecOf(base).layout {
	case docLayoutProse:
		if isDocAnnotation(name) {
			c.markDocCode(name)
			c.markDocCode(value)
		}
	case docLayoutMethod:
		c.markDocMethod(value)
	case docLayoutReference:
		c.markDocTarget(value)
	default:
		forEachDocTagType(base, prio, value, func(_ int, typ string) { c.markDocType(typ) })
	}
}

func (c *importCollector) markDocMethod(value string) {
	method := parseDocMethod(value)
	c.markDocType(method.ReturnType)
	for _, param := range method.Params {
		c.markDocType(param.Type)
		if param.Default != "" {
			c.markDocCode(param.Default)
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
	c.markDocCode(target)
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
