package lsp

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/workspace"
)

// A Blade view's variables are those passed to it: by the PHP that renders
// it, by the views that include it, and through them (see
// frameworklaravel.ViewData). Each is typed from the expression passed, at the
// place it is passed, when a request first needs it.

// viewVar is a variable a view receives, typed as the union of what each
// place passes.
type viewVar struct {
	Name  string
	Type  string
	Sites []frameworklaravel.ViewVar
}

// viewVarCache keeps each view's typed variables until the index or the
// view data changes.
type viewVarCache struct {
	mu      sync.Mutex
	entries map[string]viewVarEntry
}

type viewVarEntry struct {
	indexGen, dataGen uint64
	vars              []viewVar
}

// maxViewVarCache bounds the cache; it is emptied when full.
const maxViewVarCache = 512

// viewVars returns the variables passed to the Blade view at uri.
func (s *Server) viewVars(uri string) []viewVar {
	if s.workspace == nil || s.workspace.ViewData == nil || !workspace.IsBlade(uri) {
		return nil
	}
	return s.viewVarsOf(s.workspace.ViewData.ViewName(uri), &typingState{visiting: map[string]bool{}, files: map[string]siteFile{}})
}

// typingState is shared while typing one view's variables: the views being
// typed, so a cycle of includes ends, and each file read once.
type typingState struct {
	visiting map[string]bool
	files    map[string]siteFile
}

type siteFile struct {
	source string
	file   *parser.FileNode
}

func (s *Server) viewVarsOf(view string, st *typingState) []viewVar {
	visiting := st.visiting
	data := s.workspace.ViewData
	if view == "" || visiting[view] {
		return nil
	}
	indexGen, dataGen := s.index.Generation(), data.Generation()
	s.viewVarCache.mu.Lock()
	if e, ok := s.viewVarCache.entries[view]; ok && e.indexGen == indexGen && e.dataGen == dataGen {
		s.viewVarCache.mu.Unlock()
		return e.vars
	}
	s.viewVarCache.mu.Unlock()

	visiting[view] = true
	sites, _ := data.Vars(view)
	byName := map[string]*viewVar{}
	var order []string
	types := map[string]map[string]bool{}
	for _, site := range sites {
		v, ok := byName[site.Name]
		if !ok {
			v = &viewVar{Name: site.Name}
			byName[site.Name] = v
			order = append(order, site.Name)
			types[site.Name] = map[string]bool{}
		}
		v.Sites = append(v.Sites, site)
		if t := s.siteType(site, st); t != "" {
			for _, part := range strings.Split(t, "|") {
				types[site.Name][strings.TrimSpace(part)] = true
			}
		}
	}
	vars := make([]viewVar, 0, len(order))
	for _, name := range order {
		v := byName[name]
		parts := make([]string, 0, len(types[name]))
		for t := range types[name] {
			if t != "" && t != "mixed" {
				parts = append(parts, t)
			}
		}
		sort.Strings(parts)
		v.Type = strings.Join(parts, "|")
		vars = append(vars, *v)
	}
	delete(visiting, view)

	s.viewVarCache.mu.Lock()
	if s.viewVarCache.entries == nil || len(s.viewVarCache.entries) >= maxViewVarCache {
		s.viewVarCache.entries = map[string]viewVarEntry{}
	}
	s.viewVarCache.entries[view] = viewVarEntry{indexGen: indexGen, dataGen: dataGen, vars: vars}
	s.viewVarCache.mu.Unlock()
	return vars
}

var (
	intLiteral   = regexp.MustCompile(`^-?\d+$`)
	floatLiteral = regexp.MustCompile(`^-?\d*\.\d+$`)
)

// siteType types the expression passed at one place.
func (s *Server) siteType(site frameworklaravel.ViewVar, st *typingState) string {
	expr := strings.TrimSpace(site.Expr)
	switch {
	case expr == "":
		return ""
	case strings.HasPrefix(expr, "'") || strings.HasPrefix(expr, `"`):
		return "string"
	case intLiteral.MatchString(expr):
		return "int"
	case floatLiteral.MatchString(expr):
		return "float"
	case strings.EqualFold(expr, "true") || strings.EqualFold(expr, "false"):
		return "bool"
	case strings.EqualFold(expr, "null"):
		return "null"
	case strings.HasPrefix(expr, "["):
		return "array"
	}
	if site.FromTemplate {
		// Passed by a Blade view: a variable it received itself has the type
		// it was passed with.
		if strings.HasPrefix(expr, "$") && isPlainVar(expr[1:]) {
			from := s.workspace.ViewData.ViewName(site.URI)
			for _, v := range s.viewVarsOf(from, st) {
				if v.Name == expr[1:] {
					return v.Type
				}
			}
		}
		return ""
	}
	f, ok := st.files[site.URI]
	if !ok {
		f.source = s.siteSource(site.URI)
		if f.source != "" {
			f.file = parser.ParseFile(f.source)
		}
		st.files[site.URI] = f
	}
	if f.source == "" {
		return ""
	}
	return s.completion.ResolveExpressionType(expr, f.source, protocol.Position{Line: site.ExprLine, Character: site.ExprCol}, f.file)
}

// siteSource reads the file a variable is passed from.
func (s *Server) siteSource(uri string) string {
	if src := s.getDocument(uri); src != "" {
		return src
	}
	if src := s.index.GetFileSource(uri); src != "" {
		return src
	}
	data, err := os.ReadFile(symbols.URIToPath(uri))
	if err != nil {
		return ""
	}
	return string(data)
}

func isPlainVar(name string) bool {
	if name == "" {
		return false
	}
	for i := 0; i < len(name); i++ {
		c := name[i]
		if !(c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80) {
			return false
		}
	}
	return true
}

// bladeVarsFor returns the declarations the virtual PHP of a view gets.
func bladeVarsFor(vars []viewVar) []blade.Var {
	out := make([]blade.Var, 0, len(vars))
	for _, v := range vars {
		out = append(out, blade.Var{Name: v.Name, Type: v.Type})
	}
	return out
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
func withViewVarCompletions(items []protocol.CompletionItem, vars []viewVar, src string, offset int) []protocol.CompletionItem {
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
	passed := map[string]viewVar{}
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
func (s *Server) viewVarHover(h *protocol.Hover, vars []viewVar, src string, offset int) *protocol.Hover {
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
func viewVarDefinition(vars []viewVar, src string, offset int) ([]protocol.Location, bool) {
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
	path := symbols.URIToPath(uri)
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
