package lsp

import (
	"strings"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// A Blade view's variables are those passed to it: by the PHP that renders
// it, by the views that include it, and through them (see
// frameworklaravel.ViewData). Each is typed from the expression passed, at the
// place it is passed, when a request first needs it.

// viewVars returns what the Blade view at uri receives, typed.
func (s *Server) viewVars(uri string) frameworklaravel.TypedInputs {
	if s.session == nil || !workspace.IsBlade(uri) {
		return frameworklaravel.TypedInputs{}
	}
	return s.session.ViewTypes().For(uri)
}

// bladeCustom is what the project registers with Blade, or nil before
// initialize.
func (s *Server) bladeCustom() *blade.Custom {
	if s.session == nil {
		return nil
	}
	return s.session.BladeProviders().Custom()
}

// variableAt returns the `$name` at offset in a template, without `$`.
func variableAt(src string, offset int) (string, protocol.Range, bool) {
	start, end := offset, offset
	for start > 0 && isNameChar(src[start-1]) {
		start--
	}
	for end < len(src) && isNameChar(src[end]) {
		end++
	}
	if start == 0 || src[start-1] != '$' || start == end {
		return "", protocol.Range{}, false
	}
	// `$$name` and `::$name` aren't plain variables.
	if start >= 2 && (src[start-2] == '$' || src[start-2] == ':') {
		return "", protocol.Range{}, false
	}
	lines := blade.NewLines(src)
	return src[start:end], offsetRange(lines, start-1, end), true
}

func isNameChar(c byte) bool {
	return c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80
}

// assignedIn reports whether a template assigns the variable itself, in
// PHP (`$name = …`) or as a loop variable (`as $name`, `as $key => $name`),
// so it isn't only the one passed in.
func assignedIn(src, name string) bool {
	v := "$" + name
	for i := 0; ; {
		k := strings.Index(src[i:], v)
		if k < 0 {
			return false
		}
		at := i + k
		end := at + len(v)
		i = end
		if end < len(src) && isNameChar(src[end]) {
			continue // a longer name
		}
		after := strings.TrimLeft(src[end:], " \t")
		if strings.HasPrefix(after, "=") && !strings.HasPrefix(after, "==") && !strings.HasPrefix(after, "=>") {
			return true
		}
		before := strings.TrimRight(src[:at], " \t")
		if strings.HasSuffix(before, " as") || strings.HasSuffix(before, "(as") {
			return true
		}
		if strings.HasSuffix(before, "=>") {
			key := strings.TrimRight(strings.TrimSuffix(before, "=>"), " \t")
			j := len(key)
			for j > 0 && isNameChar(key[j-1]) {
				j--
			}
			if j > 0 && key[j-1] == '$' && strings.HasSuffix(strings.TrimRight(key[:j-1], " \t"), " as") {
				return true
			}
		}
	}
}

// withViewVarCompletions lists the variables passed to the view first when
// a variable is being typed.
func withViewVarCompletions(items []protocol.CompletionItem, vars []frameworklaravel.TypedVar, src string, offset int) []protocol.CompletionItem {
	if len(vars) == 0 {
		return items
	}
	start := offset
	for start > 0 && isNameChar(src[start-1]) {
		start--
	}
	if start == 0 || src[start-1] != '$' {
		return items
	}
	typed := strings.ToLower(src[start:offset])
	passed := map[string]frameworklaravel.TypedVar{}
	for _, v := range vars {
		passed["$"+v.Name] = v
	}
	seen := map[string]bool{}
	for i := range items {
		if v, ok := passed[items[i].Label]; ok {
			items[i].SortText = "!" + items[i].Label
			if v.Type != "" {
				items[i].Detail = v.Type
			}
			seen[items[i].Label] = true
		}
	}
	for _, v := range vars {
		label := "$" + v.Name
		if seen[label] || !strings.HasPrefix(strings.ToLower(v.Name), typed) {
			continue
		}
		seen[label] = true
		detail := v.Type
		if detail == "" {
			detail = "passed to the view"
		}
		items = append(items, protocol.CompletionItem{Label: label, Kind: protocol.CompletionItemKindVariable, Detail: detail, SortText: "!" + label})
	}
	return items
}

// viewVarHover adds where a variable passed to the view comes from to the
// hover on it.
func (s *Server) viewVarHover(h *protocol.Hover, vars []frameworklaravel.TypedVar, src string, offset int) *protocol.Hover {
	name, rng, ok := variableAt(src, offset)
	if !ok {
		return h
	}
	for _, v := range vars {
		if v.Name != name {
			continue
		}
		var b strings.Builder
		if h == nil {
			t := v.Type
			if t == "" {
				t = "mixed"
			}
			b.WriteString("```php\n" + t + " $" + name + "\n```")
			h = &protocol.Hover{Contents: protocol.MarkupContent{Kind: "markdown"}, Range: &rng}
		} else {
			b.WriteString(h.Contents.Value)
		}
		b.WriteString("\n\nPassed to this view from:\n")
		for i, site := range v.Sites {
			if i == 5 {
				b.WriteString("\n- and " + itoa(len(v.Sites)-5) + " more")
				break
			}
			b.WriteString("\n- `" + s.relativePath(site.URI) + ":" + itoa(site.Line+1) + "`")
		}
		h.Contents.Value = b.String()
		return h
	}
	return h
}

// viewVarDefinition goes to where a variable passed to the view is passed:
// the `'name' =>` key at each place. ok is false for a variable the view
// assigns itself or isn't passed.
func viewVarDefinition(vars []frameworklaravel.TypedVar, src string, offset int) ([]protocol.Location, bool) {
	name, _, ok := variableAt(src, offset)
	if !ok || assignedIn(src, name) {
		return nil, false
	}
	for _, v := range vars {
		if v.Name != name {
			continue
		}
		locs := make([]protocol.Location, 0, len(v.Sites))
		for _, site := range v.Sites {
			locs = append(locs, protocol.Location{URI: site.URI, Range: protocol.Range{
				Start: protocol.Position{Line: site.Line, Character: site.Col},
				End:   protocol.Position{Line: site.Line, Character: site.EndCol},
			}})
		}
		return locs, len(locs) > 0
	}
	return nil, false
}

func (s *Server) relativePath(uri string) string {
	path, err := symbols.URIToPath(uri)
	if err != nil {
		return uri
	}
	root := strings.TrimSuffix(s.rootPath, "/") + "/"
	return strings.TrimPrefix(path, root)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
