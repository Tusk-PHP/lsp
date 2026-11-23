package blade

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

// Attribute is an attribute a component takes.
type Attribute struct {
	Name    string // as written on the tag, in kebab case: `alert-type`
	Type    string
	Default string
}

// Component is a Blade component: a class, an anonymous view, or both.
type Component struct {
	Name     string // as written after `<x-`: `forms.input`
	ClassFQN string // empty for an anonymous component
	// URI and Line, Col locate the class, or the view of an anonymous one.
	URI       string
	Line, Col int
	ViewURI   string
	// Attributes are the class's constructor parameters, or the view's
	// `@props`.
	Attributes []Attribute
}

// Components finds a project's Blade components by name.
type Components interface {
	// ComponentNames returns the names of the components starting with
	// prefix, as written after `<x-`.
	ComponentNames(prefix string) []string
	// Component returns a component by name.
	Component(name string) (Component, bool)
	// ComponentsKnown reports whether every component written as `<x-name>`
	// can be found, so that one not found doesn't exist.
	ComponentsKnown() bool
}

// Props returns the attributes an anonymous component declares with
// `@props(['type' => 'info', 'message'])`.
func Props(src string) []Attribute {
	for _, n := range Parse(src) {
		if n.Kind != KindDirective || !strings.EqualFold(n.Name, "props") || !n.HasArgs {
			continue
		}
		args := splitArgs(src, n.Args)
		if len(args) == 0 {
			return nil
		}
		arr := trimSpan(src, args[0])
		if arr.End-arr.Start < 2 || src[arr.Start] != '[' || src[arr.End-1] != ']' {
			return nil
		}
		var attrs []Attribute
		for _, item := range splitArgs(src, Span{arr.Start + 1, arr.End - 1}) {
			key, value := item, Span{}
			if k := topLevelArrow(src, item); k >= 0 {
				key, value = Span{item.Start, k}, trimSpan(src, Span{k + 2, item.End})
			}
			name, ok := stringLiteral(src, key)
			if !ok {
				continue
			}
			a := Attribute{Name: kebab(src[name.Start:name.End])}
			if value.End > value.Start {
				a.Default = src[value.Start:value.End]
			}
			attrs = append(attrs, a)
		}
		return attrs
	}
	return nil
}

// topLevelArrow returns the offset of the first `=>` in sp outside strings
// and brackets, or -1.
func topLevelArrow(src string, sp Span) int {
	depth := 0
	for i := sp.Start; i+1 < sp.End; i++ {
		switch c := src[i]; c {
		case '\'', '"':
			for i++; i < sp.End && src[i] != c; i++ {
				if src[i] == '\\' {
					i++
				}
			}
		case '(', '[', '{':
			depth++
		case ')', ']', '}':
			depth--
		case '=':
			if depth == 0 && src[i+1] == '>' {
				return i
			}
		}
	}
	return -1
}

func trimSpan(src string, sp Span) Span {
	for sp.Start < sp.End && isSpaceByte(src[sp.Start]) {
		sp.Start++
	}
	for sp.End > sp.Start && isSpaceByte(src[sp.End-1]) {
		sp.End--
	}
	return sp
}

// kebab turns a camelCase name into the kebab case attributes are written in.
func kebab(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if 'A' <= c && c <= 'Z' {
			if i > 0 {
				b.WriteByte('-')
			}
			c += 'a' - 'A'
		} else if c == '_' {
			c = '-'
		}
		b.WriteByte(c)
	}
	return b.String()
}

// Kebab turns a camelCase name into kebab case, as Blade writes component
// attributes and names.
func Kebab(s string) string { return kebab(s) }

// isXComponent reports whether a component node names a Blade component:
// `<x-name>`, a package's `<x-pkg::name>`, or Flux's `<flux:name>`. Not
// `<x-slot>`, `<x-dynamic-component>` or Livewire's `<livewire:…>`.
func isXComponent(src string, n Node) bool {
	_, ok := componentName(src, n)
	return ok
}

// componentName returns the name a component tag is looked up by:
// `<flux:button>` is `flux::button`.
func componentName(src string, n Node) (string, bool) {
	if n.Kind != KindComponent || n.Name == "" {
		return "", false
	}
	tag := src[n.Span.Start:]
	switch {
	case strings.HasPrefix(tag, "<flux:"):
		return "flux::" + strings.TrimPrefix(n.Name, "flux:"), true
	case !strings.HasPrefix(tag, "<x-"):
		return "", false
	}
	if n.Name == "slot" || n.Name == "dynamic-component" || strings.HasPrefix(n.Name, "slot:") {
		return "", false
	}
	// `pkg::name` is a namespace; a single `:` is something else.
	if strings.Contains(strings.ReplaceAll(n.Name, "::", ""), ":") {
		return "", false
	}
	return n.Name, true
}

// componentAt returns the `<x-name>` component whose name contains offset.
func componentAt(src string, nodes []Node, offset int) (Node, bool) {
	i := sort.Search(len(nodes), func(i int) bool { return nodes[i].Span.Start > offset }) - 1
	if i < 0 {
		return Node{}, false
	}
	n := nodes[i]
	if !isXComponent(src, n) || offset < n.Expr.Start || offset > n.Expr.End {
		return Node{}, false
	}
	return n, true
}

// componentPrefixAt reports whether offset ends a partial component name
// after `<x-` or `<flux:`, returning where the name starts and the namespace
// the tag implies ("flux::" for `<flux:`).
func componentPrefixAt(src string, offset int) (start int, namespace string, ok bool) {
	k := strings.LastIndexByte(src[:offset], '<')
	if k < 0 {
		return 0, "", false
	}
	switch rest := src[k:offset]; {
	case strings.HasPrefix(rest, "<x-"):
		start = k + 3
	case strings.HasPrefix(rest, "<flux:"):
		start, namespace = k+6, "flux::"
	default:
		return 0, "", false
	}
	for i := start; i < offset; i++ {
		if !isTagNameByte(src[i]) {
			return 0, "", false
		}
	}
	return start, namespace, true
}

// attributeContext is an attribute name being typed in a component tag.
type attributeContext struct {
	component Node
	start     int             // where the attribute name starts
	present   map[string]bool // attributes already on the tag
}

// attributeAt reports whether offset is in an attribute name of an
// `<x-name>` tag, outside its values.
func attributeAt(src string, nodes []Node, offset int) (attributeContext, bool) {
	i := sort.Search(len(nodes), func(i int) bool { return nodes[i].Span.Start >= offset }) - 1
	for ; i >= 0; i-- {
		if nodes[i].Kind == KindComponent {
			break
		}
	}
	if i < 0 || !isXComponent(src, nodes[i]) {
		return attributeContext{}, false
	}
	n := nodes[i]
	if offset <= n.Span.End {
		return attributeContext{}, false
	}
	tagEnd := tagClose(src, n.Span.End, len(src))
	if offset >= tagEnd && !(tagEnd == len(src) && offset == len(src)) {
		return attributeContext{}, false
	}
	ctx := attributeContext{component: n, present: map[string]bool{}}
	// Read the attributes up to offset, skipping values.
	k := n.Span.End
	for k < offset {
		c := src[k]
		switch {
		case c == '"' || c == '\'':
			close := strings.IndexByte(src[k+1:], c)
			if close < 0 || k+1+close >= offset {
				return attributeContext{}, false // offset is in a value
			}
			k += close + 2
			continue
		case c == '{' && strings.HasPrefix(src[k:], "{{"):
			close := strings.Index(src[k+2:], "}}")
			if close < 0 || k+2+close+2 > offset {
				return attributeContext{}, false
			}
			k += close + 4
			continue
		case isAttrNameByte(c):
			s := k
			for k < len(src) && isAttrNameByte(src[k]) {
				k++
			}
			if k >= offset {
				ctx.start = s
				if src[s] == ':' {
					ctx.start++
				}
				return ctx, isSpaceByte(src[s-1])
			}
			ctx.present[strings.TrimLeft(src[s:k], ":")] = true
			continue
		}
		k++
	}
	// Right after a space: no name typed yet.
	if isSpaceByte(src[offset-1]) {
		ctx.start = offset
		return ctx, true
	}
	return attributeContext{}, false
}

func (f *Features) completeComponents(src string, nodes []Node, offset int) ([]Completion, bool) {
	if f.Components == nil {
		return nil, false
	}
	if start, ns, ok := componentPrefixAt(src, offset); ok {
		end := offset
		for end < len(src) && isTagNameByte(src[end]) {
			end++
		}
		var items []Completion
		for _, name := range f.Components.ComponentNames(ns + src[start:offset]) {
			label := strings.TrimPrefix(name, ns)
			items = append(items, Completion{Kind: CompleteComponent, Label: label, Detail: "component", Insert: label, Start: start, End: end})
		}
		return items, true
	}
	ctx, ok := attributeAt(src, nodes, offset)
	if !ok {
		return nil, false
	}
	name, _ := componentName(src, ctx.component)
	c, ok := f.Components.Component(name)
	if !ok {
		return nil, true
	}
	prefix := src[ctx.start:offset]
	bound := ctx.start > 0 && src[ctx.start-1] == ':'
	var items []Completion
	for _, a := range c.Attributes {
		if ctx.present[a.Name] || !strings.HasPrefix(a.Name, prefix) {
			continue
		}
		detail := a.Type
		if a.Default != "" {
			detail = strings.TrimSpace(detail + " = " + a.Default)
		}
		insert := a.Name + `="$1"`
		if bound {
			insert = a.Name + `="\$$1"`
		}
		items = append(items, Completion{Kind: CompleteAttribute, Label: a.Name, Detail: detail, Insert: insert, Snippet: true, Start: ctx.start, End: offset})
	}
	return items, true
}

func (f *Features) componentHover(src string, nodes []Node, offset int) (HoverResult, bool) {
	n, ok := componentAt(src, nodes, offset)
	if !ok || f.Components == nil {
		return HoverResult{}, false
	}
	name, _ := componentName(src, n)
	c, ok := f.Components.Component(name)
	if !ok {
		return HoverResult{}, false
	}
	var b strings.Builder
	tag := "x-" + c.Name
	if rest, ok := strings.CutPrefix(c.Name, "flux::"); ok {
		tag = "flux:" + rest
	}
	b.WriteString("Component `" + tag + "`")
	if c.ClassFQN != "" {
		b.WriteString("\n\n`" + c.ClassFQN + "`")
	}
	if c.ViewURI != "" {
		b.WriteString("\n\nView " + relativeViewPath(c.ViewURI))
	}
	if len(c.Attributes) > 0 {
		b.WriteString("\n\nAttributes:\n")
		for _, a := range c.Attributes {
			b.WriteString("\n- `" + a.Name + "`")
			if a.Type != "" {
				b.WriteString(" " + a.Type)
			}
			if a.Default != "" {
				b.WriteString(" = `" + a.Default + "`")
			}
		}
	}
	return HoverResult{Markdown: b.String(), Span: n.Expr}, true
}

func (f *Features) componentDefinition(src string, nodes []Node, offset int) ([]Location, bool) {
	n, ok := componentAt(src, nodes, offset)
	if !ok {
		return nil, false
	}
	if f.Components == nil {
		return nil, true
	}
	name, _ := componentName(src, n)
	c, ok := f.Components.Component(name)
	if !ok {
		return nil, true
	}
	locs := []Location{{URI: c.URI, Line: c.Line, Col: c.Col, EndCol: c.Col}}
	if c.ClassFQN != "" && c.ViewURI != "" && c.ViewURI != c.URI {
		locs = append(locs, Location{URI: c.ViewURI})
	}
	return locs, true
}

func (f *Features) unknownComponents(src string, nodes []Node) []Diagnostic {
	if f.Components == nil || !f.Components.ComponentsKnown() {
		return nil
	}
	var out []Diagnostic
	for _, n := range nodes {
		name, ok := componentName(src, n)
		if !ok || strings.Contains(name, "::") {
			// A package's components aren't reported: its namespace may
			// be registered in ways the server doesn't read.
			continue
		}
		if _, ok := f.Components.Component(name); ok {
			continue
		}
		out = append(out, Diagnostic{Code: rule.UnknownComponent, Problem: Problem{Span: n.Expr, Message: "Component `x-" + n.Name + "` not found"}})
	}
	return out
}
