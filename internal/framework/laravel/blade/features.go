package blade

import (
	"sort"
	"strconv"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/rule"
)

// Views finds a project's views by name.
type Views interface {
	// ViewURI returns the file of a view.
	ViewURI(name string) (string, bool)
	// ViewNames returns the names of the views starting with prefix.
	ViewNames(prefix string) []string
	// ViewsKnown reports whether every view of the project can be found by
	// name, so that one not found doesn't exist.
	ViewsKnown() bool
	// NamespaceKnown reports the same for a package's view namespace, as
	// in `courier::mail`.
	NamespaceKnown(ns string) bool
}

// Features answers the requests about a template's Blade itself: its
// directives and the views, sections and stacks they name. PHP in the
// template is answered elsewhere, through its virtual PHP.
type Features struct {
	Views      Views
	Components Components
	// Read returns the source of a file, open in the editor or on disk.
	Read func(uri string) string
	// Custom returns what the project registers with Blade, or nil.
	Custom func() *Custom
}

func (f *Features) custom() *Custom {
	if f.Custom == nil {
		return nil
	}
	return f.Custom()
}

// CompletionKind is what a completion item stands for.
type CompletionKind int

const (
	CompleteDirective CompletionKind = iota
	CompleteView
	CompleteSection
	CompleteStack
	CompleteComponent
	CompleteAttribute
)

// Completion is a completion item. It replaces template[Start:End].
type Completion struct {
	Kind       CompletionKind
	Label      string
	Detail     string
	Doc        string
	Insert     string
	Snippet    bool
	Start, End int
}

// Location is a place in a file, on one line, with byte columns.
type Location struct {
	URI       string
	Line, Col int
	EndCol    int
}

// Complete returns completions for the Blade at offset. ok is false when
// offset isn't in a place Blade completes, and PHP completion should answer.
func (f *Features) Complete(uri, src string, offset int) (items []Completion, ok bool) {
	nodes := ParseCustom(src, f.custom())
	if items, ok := f.completeComponents(src, nodes, offset); ok {
		return items, true
	}
	if InPHP(nodes, offset) {
		if ref, ok := PartialRefAt(src, offset); ok {
			return f.completeRef(uri, src, ref), true
		}
		return nil, false
	}
	at, prefix, ok := DirectivePrefixAt(src, nodes, offset)
	if !ok {
		// An unclosed `@include('` isn't a directive yet, so its argument
		// isn't PHP either.
		if ref, ok := PartialRefAt(src, offset); ok {
			return f.completeRef(uri, src, ref), true
		}
		return nil, false
	}
	lower := strings.ToLower(prefix)
	infos := DirectiveInfos()
	infos = append(infos, customInfos(f.custom())...)
	for _, info := range infos {
		if !strings.HasPrefix(strings.ToLower(info.Name), lower) {
			continue
		}
		items = append(items, Completion{
			Kind: CompleteDirective, Label: "@" + info.Name, Doc: info.Doc,
			Insert: "@" + info.Snippet, Snippet: true, Start: at, End: offset,
		})
	}
	return items, true
}

func (f *Features) completeRef(uri, src string, ref Ref) []Completion {
	var items []Completion
	switch ref.Kind {
	case RefView:
		if f.Views == nil {
			return nil
		}
		for _, name := range f.Views.ViewNames(ref.Name) {
			items = append(items, Completion{Kind: CompleteView, Label: name, Detail: "view", Insert: name, Start: ref.Span.Start, End: ref.Span.End})
		}
	case RefSection, RefStack:
		if ref.Declares {
			// What a layout yields is filled by the views that extend it,
			// which can't be found from here.
			return nil
		}
		kind, detail := CompleteSection, "section"
		if ref.Kind == RefStack {
			kind, detail = CompleteStack, "stack"
		}
		seen := map[string]bool{}
		for _, d := range LayoutDeclarations(uri, src, ref.Kind, f.load) {
			if seen[d.Name] || !strings.HasPrefix(d.Name, ref.Name) {
				continue
			}
			seen[d.Name] = true
			items = append(items, Completion{Kind: kind, Label: d.Name, Detail: detail + " in " + baseName(d.URI), Insert: d.Name, Start: ref.Span.Start, End: ref.Span.End})
		}
	}
	return items
}

// HoverResult is the text shown on hover, for the span it applies to.
type HoverResult struct {
	Markdown string
	Span     Span
}

// Hover describes the directive or view name at offset.
func (f *Features) Hover(src string, offset int) (HoverResult, bool) {
	nodes := ParseCustom(src, f.custom())
	if h, ok := f.componentHover(src, nodes, offset); ok {
		return h, true
	}
	if n, ok := DirectiveAt(nodes, offset); ok {
		info, ok := LookupDirectiveInfo(n.Name)
		if c := f.custom(); !ok && c != nil {
			for _, ci := range customInfos(c) {
				if strings.EqualFold(ci.Name, n.Name) {
					info, ok = ci, true
				}
			}
		}
		if !ok {
			return HoverResult{}, false
		}
		return HoverResult{Markdown: "```blade\n@" + info.Name + "\n```\n\n" + info.Doc, Span: nameSpan(n)}, true
	}
	ref, ok := RefAt(Refs(src, nodes), offset)
	if !ok || ref.Kind != RefView || f.Views == nil {
		return HoverResult{}, false
	}
	uri, ok := f.Views.ViewURI(ref.Name)
	if !ok {
		return HoverResult{}, false
	}
	return HoverResult{Markdown: "View `" + ref.Name + "`\n\n" + relativeViewPath(uri), Span: ref.Span}, true
}

// Definition returns where the view, section or stack named at offset is: the
// view's file, or the `@yield` or `@stack` of a layout. ok is false when
// offset names none of them.
func (f *Features) Definition(uri, src string, offset int) ([]Location, bool) {
	nodes := ParseCustom(src, f.custom())
	if locs, ok := f.componentDefinition(src, nodes, offset); ok {
		return locs, true
	}
	if n, ok := DirectiveAt(nodes, offset); ok {
		// A directive the project registers goes to where it's registered.
		c := f.custom()
		if c == nil || isDirective(n.Name) {
			return nil, false
		}
		key := strings.ToLower(n.Name)
		if cond := c.conditionalOf(key); cond != "" {
			key = cond
		}
		if loc, ok := c.Where[key]; ok {
			return []Location{loc}, true
		}
		return nil, true
	}
	ref, ok := RefAt(Refs(src, nodes), offset)
	if !ok {
		return nil, false
	}
	switch ref.Kind {
	case RefView:
		if f.Views == nil {
			return nil, true
		}
		if target, ok := f.Views.ViewURI(ref.Name); ok {
			return []Location{{URI: target}}, true
		}
	case RefSection, RefStack:
		var locs []Location
		for _, d := range LayoutDeclarations(uri, src, ref.Kind, f.load) {
			if d.Name == ref.Name && !(d.URI == uri && ref.Declares) {
				locs = append(locs, Location{URI: d.URI, Line: d.Line, Col: d.Col, EndCol: d.EndCol})
			}
		}
		return locs, true
	}
	return nil, true
}

// Diagnostic is a problem found in a template, with its rule code.
type Diagnostic struct {
	Problem
	Code rule.Code
}

// Diagnose reports the template's unbalanced blocks, and the views and
// components it names that don't exist.
func (f *Features) Diagnose(src string) []Diagnostic {
	nodes := ParseCustom(src, f.custom())
	var out []Diagnostic
	for _, p := range CheckStructureCustom(src, nodes, f.custom()) {
		out = append(out, Diagnostic{Problem: p, Code: rule.UnclosedDirective})
	}
	out = append(out, f.unknownComponents(src, nodes)...)
	if f.Views == nil {
		return out
	}
	known := f.Views.ViewsKnown()
	for _, r := range Refs(src, nodes) {
		if r.Kind != RefView || r.Optional {
			continue
		}
		if ns, rest, ok := strings.Cut(r.Name, "::"); ok {
			// A package's view, when its namespace's directories are known.
			if !plainViewName(rest) || !f.Views.NamespaceKnown(ns) {
				continue
			}
		} else if !known || !plainViewName(r.Name) {
			continue
		}
		if _, ok := f.Views.ViewURI(r.Name); ok {
			continue
		}
		out = append(out, Diagnostic{Code: rule.UnknownView, Problem: Problem{Span: r.Span, Message: "View `" + r.Name + "` not found"}})
	}
	return out
}

// plainViewName reports whether a view name is one the project's own views
// could have: not a package's (`mail::message`), not a class name, not the
// `raw|` text `@each` takes for its empty view.
func plainViewName(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(isNameByte(c) || c == '.' || c == '-' || c == '/') {
			return false
		}
	}
	return true
}

func (f *Features) load(name string) (string, string, bool) {
	if f.Views == nil || f.Read == nil {
		return "", "", false
	}
	uri, ok := f.Views.ViewURI(name)
	if !ok {
		return "", "", false
	}
	src := f.Read(uri)
	return uri, src, src != ""
}

func baseName(uri string) string {
	return uri[strings.LastIndexByte(uri, '/')+1:]
}

func relativeViewPath(uri string) string {
	if k := strings.Index(uri, "/resources/views/"); k >= 0 {
		return "`" + uri[k+1:] + "`"
	}
	return "`" + strings.TrimPrefix(uri, "file://") + "`"
}

// customInfos describes the directives a project registers, for completion
// and hover: a conditional comes with its `@else…` and `@end…`.
func customInfos(c *Custom) []DirectiveInfo {
	if c == nil {
		return nil
	}
	where := func(name string) string {
		loc, ok := c.Where[name]
		if !ok {
			return ""
		}
		return " in `" + projectPath(loc.URI) + ":" + itoa(loc.Line+1) + "`"
	}
	var out []DirectiveInfo
	for key := range c.Directives {
		name := c.name(key)
		out = append(out, DirectiveInfo{Name: name, Snippet: name, Doc: "Registered with `Blade::directive`" + where(key) + "."})
	}
	for key := range c.Conditionals {
		name := c.name(key)
		doc := "Registered with `Blade::if`" + where(key) + "."
		out = append(out,
			DirectiveInfo{Name: name, Snippet: name + "(${1})\n\t$0\n@end" + name, Closer: "end" + name, Doc: "Renders its body when the conditional is true. " + doc},
			DirectiveInfo{Name: "unless" + name, Snippet: "unless" + name + "(${1})\n\t$0\n@end" + name, Closer: "end" + name, Doc: "Renders its body when the conditional is false. " + doc},
			DirectiveInfo{Name: "else" + name, Snippet: "else" + name + "(${1})", Doc: "A branch rendered when the conditional is true. " + doc},
			DirectiveInfo{Name: "end" + name, Snippet: "end" + name, Doc: "Ends a `@" + name + "` block."},
		)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// projectPath shortens a file's path to where it sits in the project, from
// its app/ or vendor/ directory.
func projectPath(uri string) string {
	path := strings.TrimPrefix(uri, "file://")
	for _, dir := range []string{"/vendor/", "/app/", "/resources/", "/src/"} {
		if k := strings.LastIndex(path, dir); k >= 0 {
			return path[k+1:]
		}
	}
	return path
}

func itoa(n int) string {
	return strconv.Itoa(n)
}
