package laravel

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/types"
)

// TypedVar is a variable a view receives, typed as the union of what each
// place passes.
type TypedVar struct {
	Name  string
	Type  string
	Sites []ViewVar
}

// TypedInputs is what a view receives, typed.
type TypedInputs struct {
	Vars []TypedVar
	// Complete is set when every place that renders the view is known and
	// all the data it gets can be read (see ViewInputs).
	Complete bool
}

// ResolveFunc types a PHP expression written at a line and column of a
// file.
type ResolveFunc func(expr, source string, file *parser.FileNode, line, col int) string

// ViewTypes types the variables views receive (see ViewData). A view's
// types are worked out the first time they are needed and kept until the
// index or the view data change.
type ViewTypes struct {
	Data  *ViewData
	Index *symbols.Index
	// Resolve types an expression where it is written.
	Resolve ResolveFunc
	// Read returns a file's source, open in the editor or on disk.
	Read func(uri string) string

	mu    sync.Mutex
	cache map[string]viewTypesEntry
}

type viewTypesEntry struct {
	indexGen, dataGen uint64
	in                TypedInputs
}

// maxViewTypes bounds the cache; it is emptied when full.
const maxViewTypes = 512

// For returns the typed variables the Blade view at uri receives.
func (t *ViewTypes) For(uri string) TypedInputs {
	if t == nil || t.Data == nil {
		return TypedInputs{}
	}
	return t.of(t.Data.ViewName(uri), &typingState{visiting: map[string]bool{}, files: map[string]siteFile{}})
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

func (t *ViewTypes) of(view string, st *typingState) TypedInputs {
	if view == "" || st.visiting[view] {
		return TypedInputs{}
	}
	var indexGen uint64
	if t.Index != nil {
		indexGen = t.Index.Generation()
	}
	dataGen := t.Data.Generation()
	t.mu.Lock()
	if e, ok := t.cache[view]; ok && e.indexGen == indexGen && e.dataGen == dataGen {
		t.mu.Unlock()
		return e.in
	}
	t.mu.Unlock()

	st.visiting[view] = true
	in := t.Data.Inputs(view)
	collector := newVarCollector()
	for _, site := range in.Vars {
		collector.add(site, t.siteType(site, st))
	}
	complete := in.Complete
	for _, class := range in.Classes {
		if !t.addClassVars(collector, class, st) {
			complete = false
		}
	}
	if strings.HasPrefix(view, "components.") && t.Read != nil {
		// An anonymous component's @props, typed from their defaults.
		if uri, ok := t.viewURI(view); ok {
			for _, a := range blade.Props(t.Read(uri)) {
				name := camel(a.Name)
				collector.add(ViewVar{Name: name, URI: uri, Expr: a.Default}, literalType(a.Default))
			}
		}
	}
	delete(st.visiting, view)
	result := TypedInputs{Vars: collector.vars(), Complete: complete}

	t.mu.Lock()
	if t.cache == nil || len(t.cache) >= maxViewTypes {
		t.cache = map[string]viewTypesEntry{}
	}
	t.cache[view] = viewTypesEntry{indexGen: indexGen, dataGen: dataGen, in: result}
	t.mu.Unlock()
	return result
}

func (t *ViewTypes) viewURI(view string) (string, bool) {
	rel := strings.ReplaceAll(view, ".", "/") + ".blade.php"
	for _, dir := range t.Data.viewDirs {
		path := dir + "/" + rel
		if isFile(path) {
			return "file://" + path, true
		}
	}
	return "", false
}

// varCollector merges the places a variable is passed, and their types.
type varCollector struct {
	byName map[string]*TypedVar
	types  map[string]map[string]bool
	order  []string
}

func newVarCollector() *varCollector {
	return &varCollector{byName: map[string]*TypedVar{}, types: map[string]map[string]bool{}}
}

func (c *varCollector) add(site ViewVar, typ string) {
	v, ok := c.byName[site.Name]
	if !ok {
		v = &TypedVar{Name: site.Name}
		c.byName[site.Name] = v
		c.order = append(c.order, site.Name)
		c.types[site.Name] = map[string]bool{}
	}
	v.Sites = append(v.Sites, site)
	for _, member := range unionMembers(typ) {
		if part := member.String(); part != "" && part != "mixed" {
			c.types[site.Name][part] = true
		}
	}
}

func unionMembers(typ string) []types.TypeExpr {
	expr := types.ParseTypeExpr(typ)
	if expr.Kind == types.TypeKindUnion {
		return expr.Types
	}
	return []types.TypeExpr{expr}
}

func (c *varCollector) vars() []TypedVar {
	out := make([]TypedVar, 0, len(c.order))
	for _, name := range c.order {
		v := c.byName[name]
		parts := make([]string, 0, len(c.types[name]))
		for t := range c.types[name] {
			parts = append(parts, t)
		}
		sort.Strings(parts)
		v.Type = strings.Join(parts, "|")
		out = append(out, *v)
	}
	return out
}

// Base classes whose subclasses pass their public properties to the views
// they render. Their own members aren't passed.
var viewDataBases = []string{`Illuminate\View\Component`, `Illuminate\Mail\Mailable`}

// addClassVars adds the public properties a component or a Mailable passes
// to its view, and a component's public methods. It reports false when the
// class can't be read, so its view's data isn't known in full.
func (t *ViewTypes) addClassVars(c *varCollector, class string, st *typingState) bool {
	if t.Index == nil {
		return false
	}
	sym := t.Index.Lookup(class)
	if sym == nil {
		return false
	}
	isComponent, passes := false, false
	for _, parent := range t.Index.GetInheritanceChain(class) {
		for _, base := range viewDataBases {
			if strings.EqualFold(parent, base) {
				passes = true
				isComponent = isComponent || base == viewDataBases[0]
			}
		}
	}
	if !passes {
		return true // a controller: its properties aren't the view's
	}
	for _, m := range t.Index.GetClassMembers(class) {
		if m.Visibility != "public" || m.IsStatic || strings.HasPrefix(m.ParentFQN, `Illuminate\`) {
			continue
		}
		site := ViewVar{URI: m.URI, Line: m.Range.Start.Line, Col: m.Range.Start.Character, EndCol: m.Range.End.Character}
		switch m.Kind {
		case symbols.KindProperty:
			site.Name = strings.TrimPrefix(m.Name, "$")
			c.add(site, t.qualify(m.Type, m.URI, st))
		case symbols.KindMethod:
			if strings.EqualFold(m.Name, "__get") {
				return false
			}
			if !isComponent || strings.HasPrefix(m.Name, "__") || componentOwnMethods[strings.ToLower(m.Name)] {
				continue
			}
			site.Name = m.Name
			c.add(site, "Closure")
		}
	}
	return true
}

// scalarTypes are the type names that aren't classes.
var scalarTypes = map[string]bool{
	"int": true, "integer": true, "float": true, "string": true, "bool": true, "boolean": true, "array": true,
	"object": true, "mixed": true, "null": true, "void": true, "callable": true, "iterable": true, "false": true,
	"true": true, "never": true, "self": true, "static": true, "parent": true, "resource": true,
}

// qualify resolves the class names in a type as written in a file, such as
// a property's `Order`, against the file's namespace and imports.
func (t *ViewTypes) qualify(typ, uri string, st *typingState) string {
	if typ == "" {
		return ""
	}
	f, ok := st.files[uri]
	if !ok {
		f.source = t.source(uri)
		if f.source != "" {
			f.file = parser.ParseFile(f.source)
		}
		st.files[uri] = f
	}
	if f.file == nil {
		return typ
	}
	members := unionMembers(typ)
	parts := make([]string, len(members))
	for i, member := range members {
		if member.Kind != types.TypeKindNamed {
			parts[i] = member.String()
			continue
		}
		name := member.Name
		switch {
		case name == "" || scalarTypes[strings.ToLower(name)]:
		case strings.HasPrefix(name, `\`):
			name = name[1:]
		default:
			first, rest, _ := strings.Cut(name, `\`)
			resolved := ""
			for _, u := range f.file.Uses {
				alias := u.Alias
				if alias == "" {
					alias = u.FullName[strings.LastIndexByte(u.FullName, '\\')+1:]
				}
				if strings.EqualFold(alias, first) {
					resolved = strings.TrimPrefix(u.FullName, `\`)
					if rest != "" {
						resolved += `\` + rest
					}
					break
				}
			}
			// A qualified name is already resolved, as the index keeps types.
			if resolved == "" && rest == "" && f.file.Namespace != "" {
				resolved = f.file.Namespace + `\` + name
			}
			if resolved != "" {
				name = resolved
			}
		}
		if member.Nullable {
			name = "?" + name
		}
		parts[i] = name
	}
	return strings.Join(parts, "|")
}

// componentOwnMethods are the methods a component has that aren't passed to
// its view (Component::ignoredMethods).
var componentOwnMethods = map[string]bool{
	"data": true, "render": true, "resolve": true, "resolveview": true, "shouldrender": true, "view": true,
	"withname": true, "withattributes": true, "flushcache": true, "forgetfactory": true,
	"forgetcomponentsresolver": true, "resolvecomponentsusing": true,
}

var (
	intLiteral   = regexp.MustCompile(`^-?\d+$`)
	floatLiteral = regexp.MustCompile(`^-?\d*\.\d+$`)
)

// literalType types a literal expression, or returns "".
func literalType(expr string) string {
	expr = strings.TrimSpace(expr)
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
	return ""
}

// siteType types the expression passed at one place.
func (t *ViewTypes) siteType(site ViewVar, st *typingState) string {
	expr := strings.TrimSpace(site.Expr)
	if typ := literalType(expr); typ != "" || expr == "" {
		return typ
	}
	if site.FromTemplate {
		// Passed by a Blade view: a variable it received itself has the type
		// it was passed with.
		if strings.HasPrefix(expr, "$") && isVarName(expr[1:]) {
			for _, v := range t.of(t.Data.ViewName(site.URI), st).Vars {
				if v.Name == expr[1:] {
					return v.Type
				}
			}
		}
		return ""
	}
	if t.Resolve == nil {
		return ""
	}
	f, ok := st.files[site.URI]
	if !ok {
		f.source = t.source(site.URI)
		if f.source != "" {
			f.file = parser.ParseFile(f.source)
		}
		st.files[site.URI] = f
	}
	if f.source == "" {
		return ""
	}
	return t.Resolve(expr, f.source, f.file, site.ExprLine, site.ExprCol)
}

func (t *ViewTypes) source(uri string) string {
	if t.Read != nil {
		if src := t.Read(uri); src != "" {
			return src
		}
	}
	if t.Index != nil {
		if src := t.Index.GetFileSource(uri); src != "" {
			return src
		}
	}
	data, err := os.ReadFile(symbols.URIToPath(uri))
	if err != nil {
		return ""
	}
	return string(data)
}

// camel turns an attribute name such as `alert-type` into the variable a
// view gets, `alertType`.
func camel(s string) string {
	var b strings.Builder
	up := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c == '-' || c == '_' {
			up = true
			continue
		}
		if up && 'a' <= c && c <= 'z' {
			c -= 'a' - 'A'
		}
		up = false
		b.WriteByte(c)
	}
	return b.String()
}

// BladeVars returns the declarations a view's virtual PHP gets.
func (in TypedInputs) BladeVars() []blade.Var {
	out := make([]blade.Var, 0, len(in.Vars))
	for _, v := range in.Vars {
		out = append(out, blade.Var{Name: v.Name, Type: v.Type})
	}
	return out
}
