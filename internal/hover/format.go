package hover

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// maxSignatureWidth is the longest signature kept on one line in a hover card.
const maxSignatureWidth = 72

// qualifiedName matches the namespace part of a qualified class name in a type.
var qualifiedName = regexp.MustCompile(`\\?(?:[\p{L}_][\p{L}\p{N}_]*\\)+`)

// phpFence wraps code in a PHP code block. Editors highlight the php language
// with grammars that treat everything before an opening tag as plain text, so
// the code block needs one to get colours.
func phpFence(code string) string {
	return "```php\n<?php\n" + code + "\n```\n"
}

// shortTypes removes namespaces from a rendered type, so signatures stay
// readable: App\Models\User|null becomes User|null.
func shortTypes(rendered string) string {
	return qualifiedName.ReplaceAllString(rendered, "")
}

func (p *Provider) formatHoverDeclaration(sym *symbols.Symbol) string {
	var sb strings.Builder

	switch sym.Kind {
	case symbols.KindClass:
		if sym.IsFinal {
			sb.WriteString("final ")
		}
		if sym.IsAbstract {
			sb.WriteString("abstract ")
		}
		if sym.IsReadonly {
			sb.WriteString("readonly ")
		}
		sb.WriteString("class " + sym.Name)
		if sym.Extends != "" {
			sb.WriteString(" extends " + shortName(sym.Extends))
		}
		if len(sym.Implements) > 0 {
			sb.WriteString(" implements " + joinShortNames(sym.Implements))
		}
	case symbols.KindInterface:
		sb.WriteString("interface " + sym.Name)
		if len(sym.Implements) > 0 {
			sb.WriteString(" extends " + joinShortNames(sym.Implements))
		}
	case symbols.KindMethod:
		if sym.IsVirtual {
			sb.WriteString("(magic) ")
		}
		vis := sym.Visibility
		if vis == "" {
			vis = "public"
		}
		if sym.IsAbstract {
			sb.WriteString("abstract ")
		}
		if sym.IsFinal {
			sb.WriteString("final ")
		}
		sb.WriteString(vis)
		if sym.IsStatic {
			sb.WriteString(" static")
		}
		sb.WriteString(" function " + sym.Name)
		sb.WriteString(fmtSignature(sb.Len(), sym.Params, sym.ReturnType))
	case symbols.KindFunction:
		sb.WriteString("function " + sym.Name)
		sb.WriteString(fmtSignature(sb.Len(), sym.Params, sym.ReturnType))
	case symbols.KindProperty:
		if sym.IsVirtual {
			sb.WriteString("(magic) ")
		}
		vis := sym.Visibility
		if vis == "" {
			vis = "public"
		}
		t := sym.Type
		if t == "" {
			t = "mixed"
		} else {
			t = shortTypes(types.RenderTypeExpr(t))
		}
		sb.WriteString(vis)
		// PHP 8.4 asymmetric visibility: e.g. public private(set) string $x
		if sym.SetVisibility != "" {
			sb.WriteString(fmt.Sprintf(" %s(set)", sym.SetVisibility))
		}
		if sym.IsStatic {
			sb.WriteString(" static")
		}
		if sym.IsReadonly {
			sb.WriteString(" readonly")
		}
		propName := sym.Name
		if !strings.HasPrefix(propName, "$") {
			propName = "$" + propName
		}
		sb.WriteString(fmt.Sprintf(" %s %s", t, propName))
		// PHP 8.4 property hooks: e.g. { get; set; }
		if len(sym.Hooks) > 0 {
			hookNames := make([]string, len(sym.Hooks))
			for i, h := range sym.Hooks {
				hookNames[i] = h.Kind + ";"
			}
			sb.WriteString(" { " + strings.Join(hookNames, " ") + " }")
		}
	case symbols.KindEnum:
		sb.WriteString("enum " + sym.Name)
		if sym.BackedType != "" {
			sb.WriteString(": " + sym.BackedType)
		}
		if len(sym.Implements) > 0 {
			sb.WriteString(" implements " + joinShortNames(sym.Implements))
		}
	case symbols.KindEnumCase:
		sb.WriteString("case " + sym.Name)
		if sym.Value != "" {
			sb.WriteString(" = " + sym.Value)
		}
	case symbols.KindConstant:
		sb.WriteString("const " + sym.Name)
		if sym.Value != "" {
			sb.WriteString(" = " + sym.Value)
		}
	case symbols.KindTrait:
		sb.WriteString("trait " + sym.Name)
	}

	return sb.String()
}

func (p *Provider) formatHoverContext(sym *symbols.Symbol) string {
	var sb strings.Builder

	switch sym.Kind {
	case symbols.KindMethod:
		if sym.ParentFQN != "" {
			p.appendMethodOrigin(&sb, sym)
		}
	case symbols.KindClass:
		if impls := p.index.GetImplementors(sym.FQN); len(impls) > 0 {
			sb.WriteString("\n**Implemented by:** ")
			names := make([]string, len(impls))
			for i, impl := range impls {
				names[i] = "`" + impl.FQN + "`"
			}
			sb.WriteString(strings.Join(names, ", ") + "\n")
		}
	case symbols.KindInterface:
		if impls := p.index.GetImplementors(sym.FQN); len(impls) > 0 {
			sb.WriteString("\n**Implementations:** ")
			names := make([]string, len(impls))
			for i, impl := range impls {
				names[i] = "`" + impl.FQN + "`"
			}
			sb.WriteString(strings.Join(names, ", ") + "\n")
		}
	}

	return sb.String()
}

// formatCoverage renders a method or function's line coverage. It only reads
// what's already loaded, and renders nothing when coverage is unknown: a missing
// line must never read as untested code.
func (p *Provider) formatCoverage(sym *symbols.Symbol) string {
	if p.coverage == nil || (sym.Kind != symbols.KindMethod && sym.Kind != symbols.KindFunction) {
		return ""
	}
	p.coverage.RefreshAsync()

	m, stale := p.coverage.Method(symbols.URIToPath(sym.URI), sym.Name, sym.Range.Start.Line+1)
	if m == nil {
		return ""
	}
	line := "\nCoverage: " + coverageBar(m.Covered, m.Executable) + fmt.Sprintf(" · %d/%d lines", m.Covered, m.Executable)
	if m.CRAP > 0 {
		line += " · CRAP " + strconv.FormatFloat(m.CRAP, 'f', -1, 64)
	}
	if stale {
		line += " (file changed since the report, may be out of date)"
	}
	return line + "\n"
}

// coverageBar draws coverage as a coloured dot, a bar and a percentage, using
// only text because hover cards are markdown. The bar is inline code so it's
// monospaced. Partial coverage never rounds to an empty or a full bar, or to 0%
// or 100%. The colour bands are the defaults of PHPUnit's HTML report.
func coverageBar(covered, total int) string {
	const width = 10
	filled := (covered*width + total/2) / total
	percent := covered * 100 / total
	if covered > 0 && covered < total {
		filled = max(1, min(filled, width-1))
		percent = max(1, min(percent, 99))
	}

	dot := "🟢"
	switch {
	case percent < 50:
		dot = "🔴"
	case percent < 90:
		dot = "🟡"
	}
	return fmt.Sprintf("%s `%s%s` %d%%", dot, strings.Repeat("█", filled), strings.Repeat("░", width-filled), percent)
}

func (p *Provider) formatDocBlockDetails(doc *parser.DocBlock) string {
	if doc == nil {
		return ""
	}

	var sb strings.Builder

	if doc.Deprecated {
		msg := doc.DeprecatedMsg
		if msg == "" {
			msg = "This symbol is deprecated."
		}
		sb.WriteString(fmt.Sprintf("\n**Deprecated:** %s\n", msg))
	}
	if len(doc.Params) > 0 {
		sb.WriteString("\n**Params**\n")
		for _, param := range doc.Params {
			line := "- "
			if param.Name != "" {
				line += "`" + param.Name + "` "
			}
			if param.Type != "" {
				line += "`" + types.RenderTypeExpr(param.Type) + "`"
			}
			if param.Description != "" {
				line += " — " + param.Description
			}
			sb.WriteString(line + "\n")
		}
	}
	if doc.Return.Type != "" {
		ret := fmt.Sprintf("\n**Returns** `%s`", types.RenderTypeExpr(doc.Return.Type))
		if doc.Return.Description != "" {
			ret += " — " + doc.Return.Description
		}
		sb.WriteString(ret + "\n")
	}
	if len(doc.Throws) > 0 {
		sb.WriteString("\n**Throws**\n")
		for _, th := range doc.Throws {
			line := "- `" + types.RenderTypeExpr(th.Type) + "`"
			if th.Description != "" {
				line += " — " + th.Description
			}
			sb.WriteString(line + "\n")
		}
	}
	for _, tagName := range []string{"template", "mixin", "see", "property", "property-read", "property-write", "method"} {
		if vals, ok := doc.Tags[tagName]; ok && len(vals) > 0 {
			label := "@" + tagName
			for _, v := range vals {
				sb.WriteString(fmt.Sprintf("\n`%s %s`\n", label, v))
			}
		}
	}

	return sb.String()
}

func shortName(fqn string) string {
	if i := strings.LastIndex(fqn, "\\"); i >= 0 {
		return fqn[i+1:]
	}
	return fqn
}

// replaceReturnType replaces the return type in a hover markdown with a more
// specific generic type. Handles both inline ): Type and **Returns** `Type`.
func replaceReturnType(content, newType string) string {
	// Try **Returns** `Type` pattern (docblock-derived return types)
	marker := "**Returns** `"
	if idx := strings.Index(content, marker); idx >= 0 {
		start := idx + len(marker)
		end := strings.Index(content[start:], "`")
		if end >= 0 {
			return content[:start] + newType + content[start+end:]
		}
	}
	// Try ): Type pattern (inline PHP return type)
	if idx := strings.Index(content, "): "); idx >= 0 {
		endOfType := idx + 3
		end := strings.IndexAny(content[endOfType:], "\n`")
		if end >= 0 {
			old := content[endOfType : endOfType+end]
			return content[:endOfType] + newType + content[endOfType+len(old):]
		}
	}
	return content
}

func joinShortNames(fqns []string) string {
	names := make([]string, len(fqns))
	for i, fqn := range fqns {
		names[i] = shortName(fqn)
	}
	return strings.Join(names, ", ")
}

// fmtSignature renders a parameter list and return type, moving parameters to
// their own lines when the signature wouldn't fit on one. prefixLen is the width
// of what comes before the opening parenthesis.
func fmtSignature(prefixLen int, params []symbols.ParamInfo, returnType string) string {
	parts := fmtParams(params)
	ret := ""
	if returnType != "" {
		ret = ": " + shortTypes(types.RenderTypeExpr(returnType))
	}
	oneLine := "(" + strings.Join(parts, ", ") + ")" + ret
	if len(parts) == 0 || prefixLen+len(oneLine) <= maxSignatureWidth {
		return oneLine
	}
	return "(\n    " + strings.Join(parts, ",\n    ") + "\n)" + ret
}

func fmtParams(params []symbols.ParamInfo) []string {
	var parts []string
	for _, p := range params {
		s := ""
		if p.Type != "" {
			s += shortTypes(types.RenderTypeExpr(p.Type)) + " "
		}
		if p.IsVariadic {
			s += "..."
		}
		if p.IsReference {
			s += "&"
		}
		s += p.Name
		if p.DefaultValue != "" {
			s += " = " + p.DefaultValue
		}
		parts = append(parts, s)
	}
	return parts
}
