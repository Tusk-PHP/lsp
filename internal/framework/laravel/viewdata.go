package laravel

import (
	"hash/fnv"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// ViewVar is a variable passed to a view at one place.
type ViewVar struct {
	Name string // without `$`
	// URI and Line, Col, EndCol locate the key that names the variable: the
	// `'user'` of `'user' => $user`, or the `'user'` of `compact('user')`.
	URI               string
	Line, Col, EndCol int
	// Expr is the PHP expression passed, and ExprLine, ExprCol where it
	// starts. Expr is empty when the value can't be read, as for the item
	// `@each` passes.
	Expr              string
	ExprLine, ExprCol int
	// FromTemplate is set when the variable is passed by a Blade view, whose
	// PHP the expression is written in.
	FromTemplate bool
}

// sharedView is the name renders are recorded under for data every view
// gets: View::share() and composers registered for '*'.
const sharedView = "*"

// viewRender is one place a view is rendered with its data.
type viewRender struct {
	uri  string
	view string
	vars []ViewVar
	// opaque is set when some of the data can't be read, such as
	// view('x', $request->all()).
	opaque bool
	// class is the class the render is written in, whose public properties
	// a component's or a Mailable's view gets too.
	class string
}

// viewComposer is a composer registered for views: a class whose compose()
// adds data, or a closure whose data was read where it's registered.
type viewComposer struct {
	views  []string
	class  string
	vars   []ViewVar
	opaque bool
}

// viewEdge is a Blade view rendering another inside it: `@include`,
// `@extends`, `@each` or `@component`. The view rendered sees the variables
// of the one rendering it, plus vars.
type viewEdge struct {
	from, to string
	vars     []ViewVar
	// isolated is set for `@each`, whose view only gets the item.
	isolated bool
}

// phpViewFile is what one PHP file says about views.
type phpViewFile struct {
	renders   []viewRender
	composers []viewComposer
	// composes are the data each composer class in the file adds, by class.
	composes map[string]viewRender
	// dynamic counts the views rendered by a name the server can't read,
	// such as view($name).
	dynamic int
	// mentions counts the string literals that could name a view, and
	// consumed those of them read as view names by a render or composer.
	mentions, consumed map[uint64]int
}

type templateViewFile struct {
	edges   []viewEdge
	dynamic int
}

// ViewData indexes the data passed to each view: by PHP that renders it
// (`view()`, `View::make()`, `response()->view()`, `Route::view()`, a
// Mailable's `Content`, and their `->with()` calls), by `View::share()` and
// view composers, and by the Blade views that include or extend it. It is
// updated per file as files are indexed, so a request looks a view up and
// never scans.
type ViewData struct {
	rootPath string

	mu        sync.RWMutex
	php       map[string]phpViewFile      // PHP file → what it says
	templates map[string]templateViewFile // Blade file → the views it renders

	// Totals over every file, kept as files change.
	byView      map[string][]viewRender   // view name → its renders
	composersOf map[string][]viewComposer // view name → its composers
	composes    map[string][]viewRender   // composer class → the data it adds
	edgesTo     map[string][]viewEdge     // view name → the views rendering it
	dynamic     int
	mentions    map[uint64]int // by viewHash
	consumed    map[uint64]int

	viewDirs []string
	// generation counts changes, so answers built from the index can tell
	// when they're stale.
	generation atomic.Uint64
}

// NewViewData returns an empty index for the Laravel app at rootPath.
func NewViewData(rootPath string) *ViewData {
	extra, _ := livewireLocations(rootPath)
	return &ViewData{
		rootPath:    rootPath,
		php:         map[string]phpViewFile{},
		templates:   map[string]templateViewFile{},
		byView:      map[string][]viewRender{},
		composersOf: map[string][]viewComposer{},
		composes:    map[string][]viewRender{},
		edgesTo:     map[string][]viewEdge{},
		mentions:    map[uint64]int{},
		consumed:    map[uint64]int{},
		viewDirs:    append([]string{filepath.Join(rootPath, "resources", "views")}, extra...),
	}
}

// Generation counts the changes to the index.
func (d *ViewData) Generation() uint64 {
	if d == nil {
		return 0
	}
	return d.generation.Load()
}

// IndexFile reads what a file says about views, replacing what was read
// from it before: the views a PHP file renders and the data it passes them,
// or the views a Blade view includes and extends.
func (d *ViewData) IndexFile(uri, source string) {
	if d == nil {
		return
	}
	if strings.HasSuffix(uri, ".blade.php") {
		d.indexTemplate(uri, source)
		return
	}
	var f phpViewFile
	if mayTouchViews(source) {
		f = parseViewFile(uri, source)
	}
	f.mentions = literalMentions(source)
	d.mu.Lock()
	defer d.mu.Unlock()
	old, had := d.php[uri]
	if !had && f.empty() || had && reflect.DeepEqual(old, f) {
		return
	}
	d.generation.Add(1)
	if had {
		d.applyPHP(uri, old, -1)
	}
	if f.empty() {
		delete(d.php, uri)
		return
	}
	d.php[uri] = f
	d.applyPHP(uri, f, 1)
}

// mayTouchViews is a cheap test of whether a file can render a view or
// register view data, so most files are never tokenized.
func mayTouchViews(source string) bool {
	return strings.Contains(source, "view") || strings.Contains(source, "View")
}

func (f phpViewFile) empty() bool {
	return len(f.renders) == 0 && len(f.composers) == 0 && len(f.composes) == 0 && f.dynamic == 0 && len(f.mentions) == 0
}

// applyPHP adds (sign 1) or removes (sign -1) a PHP file's part of the
// totals.
func (d *ViewData) applyPHP(uri string, f phpViewFile, sign int) {
	for _, r := range f.renders {
		if sign > 0 {
			d.byView[r.view] = append(d.byView[r.view], r)
		} else {
			d.byView[r.view] = withoutRendersOf(d.byView[r.view], uri)
		}
	}
	for _, c := range f.composers {
		for _, v := range c.views {
			if sign > 0 {
				d.composersOf[v] = append(d.composersOf[v], c)
			} else {
				kept := d.composersOf[v][:0]
				for _, o := range d.composersOf[v] {
					if !reflect.DeepEqual(o, c) {
						kept = append(kept, o)
					}
				}
				d.composersOf[v] = kept
			}
		}
	}
	for class, r := range f.composes {
		if sign > 0 {
			d.composes[class] = append(d.composes[class], r)
		} else {
			d.composes[class] = withoutRendersOf(d.composes[class], uri)
		}
	}
	d.dynamic += sign * f.dynamic
	for s, n := range f.mentions {
		d.mentions[s] += sign * n
		if d.mentions[s] <= 0 {
			delete(d.mentions, s)
		}
	}
	for s, n := range f.consumed {
		d.consumed[s] += sign * n
		if d.consumed[s] <= 0 {
			delete(d.consumed, s)
		}
	}
}

func withoutRendersOf(renders []viewRender, uri string) []viewRender {
	out := renders[:0]
	for _, r := range renders {
		if r.uri != uri {
			out = append(out, r)
		}
	}
	return out
}

func (d *ViewData) indexTemplate(uri, source string) {
	from := d.ViewName(uri)
	var f templateViewFile
	if from != "" {
		f.edges, f.dynamic = parseViewEdges(uri, from, source)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	old, had := d.templates[uri]
	empty := len(f.edges) == 0 && f.dynamic == 0
	if !had && empty || had && reflect.DeepEqual(old, f) {
		return
	}
	d.generation.Add(1)
	for _, e := range old.edges {
		kept := d.edgesTo[e.to][:0]
		for _, o := range d.edgesTo[e.to] {
			if o.from != from {
				kept = append(kept, o)
			}
		}
		d.edgesTo[e.to] = kept
	}
	d.dynamic += f.dynamic - old.dynamic
	if empty {
		delete(d.templates, uri)
		return
	}
	d.templates[uri] = f
	for _, e := range f.edges {
		d.edgesTo[e.to] = append(d.edgesTo[e.to], e)
	}
}

// ViewName returns the name of the view at uri, such as `users.show` for
// resources/views/users/show.blade.php, or "" outside the view directories.
func (d *ViewData) ViewName(uri string) string {
	if d == nil {
		return ""
	}
	path, err := symbols.URIToPath(uri)
	if err != nil {
		return ""
	}
	path = filepath.ToSlash(path)
	for _, dir := range d.viewDirs {
		prefix := filepath.ToSlash(dir) + "/"
		if strings.HasPrefix(path, prefix) {
			rel := strings.TrimSuffix(strings.TrimSuffix(path[len(prefix):], ".blade.php"), ".php")
			return strings.ReplaceAll(rel, "/", ".")
		}
	}
	return ""
}

// maxViewDepth bounds how far variables are followed through views that
// include or extend each other.
const maxViewDepth = 8

// ViewInputs is what a view receives.
type ViewInputs struct {
	// Vars are the variables passed, at every place they're passed.
	Vars []ViewVar
	// Classes are the classes rendering the view, or a view that includes
	// it, whose public properties it gets when they are components or
	// Mailables.
	Classes []string
	// Complete is set when every place that renders the view, or a view it
	// inherits variables from, is known, and all the data they pass can be
	// read: a variable none of them passes isn't defined.
	Complete bool
}

// Inputs returns what a view receives: from the PHP that renders it, from
// View::share() and composers, from the views that include it with data,
// and, through those views, what is passed to them.
func (d *ViewData) Inputs(view string) ViewInputs {
	var in ViewInputs
	if d == nil || view == "" {
		return in
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	in.Complete = d.dynamic == 0 && ownViewName(view)
	seen := map[string]bool{}
	classes := map[string]bool{}
	var walk func(view string, depth int)
	walk = func(view string, depth int) {
		if seen[view] {
			return
		}
		if depth > maxViewDepth {
			in.Complete = false
			return
		}
		seen[view] = true
		renders, edges := d.byView[view], d.edgesTo[view]
		if len(renders)+len(edges) == 0 || d.mentions[viewHash(view)] > d.consumed[viewHash(view)] {
			// Nothing known renders it, or its name is used somewhere a
			// render isn't recognized.
			in.Complete = false
		}
		for _, r := range renders {
			in.Vars = append(in.Vars, r.vars...)
			in.Complete = in.Complete && !r.opaque
			if r.class != "" && !classes[r.class] {
				classes[r.class] = true
				in.Classes = append(in.Classes, r.class)
			}
		}
		d.addComposed(&in, view)
		for _, e := range edges {
			in.Vars = append(in.Vars, e.vars...)
			if !e.isolated {
				walk(e.from, depth+1)
			}
		}
	}
	walk(view, 0)
	for _, r := range d.byView[sharedView] {
		in.Vars = append(in.Vars, r.vars...)
		in.Complete = in.Complete && !r.opaque
	}
	d.addComposed(&in, sharedView)
	return in
}

// addComposed adds the data the composers of a view add.
func (d *ViewData) addComposed(in *ViewInputs, view string) {
	for _, c := range d.composersOf[view] {
		in.Vars = append(in.Vars, c.vars...)
		in.Complete = in.Complete && !c.opaque
		if c.class == "" {
			continue
		}
		composes := d.composes[c.class]
		if len(composes) == 0 {
			in.Complete = false // a composer whose compose() wasn't read
		}
		for _, r := range composes {
			in.Vars = append(in.Vars, r.vars...)
			in.Complete = in.Complete && !r.opaque
		}
	}
}

// ownViewName reports whether a view is one the app renders itself with the
// data it passes: not a component's or Livewire's, an error page, or a
// package's published view, which get data the app doesn't pass.
func ownViewName(view string) bool {
	for _, prefix := range []string{"components.", "livewire.", "errors.", "vendor.", "flux."} {
		if strings.HasPrefix(view, prefix) {
			return false
		}
	}
	return !strings.Contains(view, "::")
}

// RenderedFrom reports whether anything is known to render the view.
func (d *ViewData) RenderedFrom(view string) bool {
	if d == nil {
		return false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	return len(d.byView[view]) > 0 || len(d.edgesTo[view]) > 0
}

// viewHash is how a string that could name a view is counted, so a file's
// mentions take little room.
func viewHash(s string) uint64 {
	h := fnv.New64a()
	h.Write([]byte(s))
	return h.Sum64()
}

// literalMentions counts the quoted strings in a file that could name a
// view, without tokenizing it: a view named somewhere other than a render
// the server reads may be rendered with data it doesn't see.
func literalMentions(source string) map[uint64]int {
	var out map[uint64]int
	for i := 0; i < len(source); i++ {
		q := source[i]
		if q != '\'' && q != '"' {
			continue
		}
		j := i + 1
		for j < len(source) && j-i <= 128 && isViewNameByte(source[j]) {
			j++
		}
		if j < len(source) && source[j] == q && j > i+1 {
			if out == nil {
				out = map[uint64]int{}
			}
			out[viewHash(source[i+1:j])]++
			i = j
		}
	}
	return out
}

func isViewNameByte(c byte) bool {
	return c == '.' || c == '-' || c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9')
}
