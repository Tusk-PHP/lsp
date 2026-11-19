package laravel

import (
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
)

// ViewVar is a variable passed to a view at one place.
type ViewVar struct {
	Name string // without `$`
	// URI and Line, Col, EndCol locate the key that names the variable: the
	// `'user'` of `'user' => $user`, or the `'user'` of `compact('user')`.
	URI               string
	Line, Col, EndCol int
	// Expr is the PHP expression passed, and ExprLine, ExprCol where it
	// starts. Expr is empty when the value can't be read, as for a variable
	// a view receives from the view that includes it.
	Expr              string
	ExprLine, ExprCol int
	// FromTemplate is set when the variable is passed by a Blade view, whose
	// PHP the expression is written in.
	FromTemplate bool
}

// viewRender is one place a view is rendered with its data.
type viewRender struct {
	uri  string
	view string
	vars []ViewVar
	// opaque is set when some of the data can't be read, such as
	// view('x', $request->all()).
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

// ViewData indexes the data passed to each view: by PHP that renders it
// (`view()`, `View::make()`, `response()->view()`, `Route::view()` and their
// `->with()` calls) and by the Blade views that include or extend it. It is
// updated per file as files are indexed, so a request looks a view up and
// never scans.
type ViewData struct {
	rootPath string

	mu       sync.RWMutex
	byURI    map[string][]viewRender // PHP file → its renders
	byView   map[string][]viewRender // view name → its renders
	edgesOf  map[string][]viewEdge   // Blade file → the views it renders
	edgesTo  map[string][]viewEdge   // view name → the Blade views rendering it
	viewDirs []string
	// generation counts changes, so answers built from the index can tell
	// when they're stale.
	generation atomic.Uint64
}

// Generation counts the changes to the index.
func (d *ViewData) Generation() uint64 {
	if d == nil {
		return 0
	}
	return d.generation.Load()
}

// NewViewData returns an empty index for the Laravel app at rootPath.
func NewViewData(rootPath string) *ViewData {
	extra, _ := livewireLocations(rootPath)
	return &ViewData{
		rootPath: rootPath,
		byURI:    map[string][]viewRender{},
		byView:   map[string][]viewRender{},
		edgesOf:  map[string][]viewEdge{},
		edgesTo:  map[string][]viewEdge{},
		viewDirs: append([]string{filepath.Join(rootPath, "resources", "views")}, extra...),
	}
}

// IndexFile reads the views a PHP file renders and the data it passes them,
// replacing what was read from the file before. A Blade view is read for the
// views it includes and extends.
func (d *ViewData) IndexFile(uri, source string) {
	if d == nil {
		return
	}
	if strings.HasSuffix(uri, ".blade.php") {
		d.indexTemplate(uri, source)
		return
	}
	var renders []viewRender
	if strings.Contains(source, "view(") || strings.Contains(source, "View::make(") {
		renders = parseViewRenders(uri, source)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if reflect.DeepEqual(renders, d.byURI[uri]) || len(renders) == 0 && len(d.byURI[uri]) == 0 {
		return
	}
	d.generation.Add(1)
	for _, r := range d.byURI[uri] {
		d.byView[r.view] = withoutURI(d.byView[r.view], uri)
	}
	if len(renders) == 0 {
		delete(d.byURI, uri)
		return
	}
	d.byURI[uri] = renders
	for _, r := range renders {
		d.byView[r.view] = append(d.byView[r.view], r)
	}
}

func withoutURI(renders []viewRender, uri string) []viewRender {
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
	var edges []viewEdge
	if from != "" {
		edges = parseViewEdges(uri, from, source)
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if reflect.DeepEqual(edges, d.edgesOf[uri]) || len(edges) == 0 && len(d.edgesOf[uri]) == 0 {
		return
	}
	d.generation.Add(1)
	for _, e := range d.edgesOf[uri] {
		kept := d.edgesTo[e.to][:0]
		for _, o := range d.edgesTo[e.to] {
			if o.from != from {
				kept = append(kept, o)
			}
		}
		d.edgesTo[e.to] = kept
	}
	if len(edges) == 0 {
		delete(d.edgesOf, uri)
		return
	}
	d.edgesOf[uri] = edges
	for _, e := range edges {
		d.edgesTo[e.to] = append(d.edgesTo[e.to], e)
	}
}

// ViewName returns the name of the view at uri, such as `users.show` for
// resources/views/users/show.blade.php, or "" outside the view directories.
func (d *ViewData) ViewName(uri string) string {
	if d == nil {
		return ""
	}
	path := strings.TrimPrefix(uri, "file://")
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

// Vars returns the variables passed to a view, at every place they are
// passed: by PHP that renders it, by the views that include it with data,
// and, through those views, what is passed to them. complete is false when
// some data passed to the view, or to a view it inherits from, can't be read.
func (d *ViewData) Vars(view string) (vars []ViewVar, complete bool) {
	if d == nil || view == "" {
		return nil, false
	}
	d.mu.RLock()
	defer d.mu.RUnlock()
	complete = true
	seen := map[string]bool{}
	var walk func(view string, depth int)
	walk = func(view string, depth int) {
		if seen[view] || depth > maxViewDepth {
			return
		}
		seen[view] = true
		for _, r := range d.byView[view] {
			vars = append(vars, r.vars...)
			if r.opaque {
				complete = false
			}
		}
		for _, e := range d.edgesTo[view] {
			vars = append(vars, e.vars...)
			if !e.isolated {
				walk(e.from, depth+1)
			}
		}
	}
	walk(view, 0)
	return vars, complete
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

// renderCall says where a render call takes its view name and data.
type renderCall struct {
	nameArg, dataArg int
	chain            bool // whether `->with()` calls after it add data
}

// parseViewRenders reads the views a PHP file renders.
func parseViewRenders(uri, source string) []viewRender {
	all := parser.Tokenize(source)
	toks := make([]parser.Token, 0, len(all))
	for _, t := range all {
		if t.Kind != parser.TokenComment && t.Kind != parser.TokenDocComment {
			toks = append(toks, t)
		}
	}
	var out []viewRender
	for i := 0; i+1 < len(toks); i++ {
		call, ok := renderCallAt(toks, i)
		if !ok {
			continue
		}
		open := i + 1
		if toks[i].Value == "View" || toks[i].Value == "Route" {
			open = i + 3
		}
		args, close := splitTokenArgs(toks, open)
		if close < 0 || call.nameArg >= len(args) {
			continue
		}
		name, ok := literalString(toks, args[call.nameArg])
		if !ok || name == "" {
			continue
		}
		r := viewRender{uri: uri, view: name}
		if call.dataArg < len(args) {
			r.vars, r.opaque = readData(uri, source, toks, args[call.dataArg], i)
		}
		if call.chain {
			for k := close + 1; k+2 < len(toks) && toks[k].Kind == parser.TokenArrow && toks[k+2].Kind == parser.TokenOpenParen; {
				method := toks[k+1].Value
				if !strings.HasPrefix(method, "with") {
					break
				}
				wargs, wclose := splitTokenArgs(toks, k+2)
				if wclose < 0 {
					break
				}
				vars, opaque := readWith(uri, source, toks, method, wargs, i)
				r.vars = append(r.vars, vars...)
				r.opaque = r.opaque || opaque
				k = wclose + 1
			}
		}
		for k := range r.vars {
			r.vars[k].URI = uri
		}
		out = append(out, r)
		i = close
	}
	return out
}

// renderCallAt reports whether a render call starts at toks[i].
func renderCallAt(toks []parser.Token, i int) (renderCall, bool) {
	t := toks[i]
	if t.Kind != parser.TokenIdentifier {
		return renderCall{}, false
	}
	prev := parser.TokenEOF
	if i > 0 {
		prev = toks[i-1].Kind
	}
	switch {
	case t.Value == "view" && toks[i+1].Kind == parser.TokenOpenParen:
		if prev == parser.TokenArrow {
			// response()->view('name', $data), a Mailable's ->view().
			return renderCall{nameArg: 0, dataArg: 1}, true
		}
		if prev == parser.TokenDoubleColon || prev == parser.TokenFunction || prev == parser.TokenNew {
			return renderCall{}, false
		}
		return renderCall{nameArg: 0, dataArg: 1, chain: true}, true
	case (t.Value == "View" || t.Value == "Route") && i+3 < len(toks) && toks[i+1].Kind == parser.TokenDoubleColon && toks[i+3].Kind == parser.TokenOpenParen:
		if prev == parser.TokenArrow || prev == parser.TokenDoubleColon {
			return renderCall{}, false
		}
		method := toks[i+2].Value
		if t.Value == "View" && method == "make" {
			return renderCall{nameArg: 0, dataArg: 1, chain: true}, true
		}
		if t.Value == "Route" && method == "view" {
			return renderCall{nameArg: 1, dataArg: 2}, true
		}
	}
	return renderCall{}, false
}

// tokenSpan is a range of tokens [start, end).
type tokenSpan struct{ start, end int }

// splitTokenArgs splits the arguments of the call whose `(` is toks[open],
// returning them and the index of its `)`, or -1 when it isn't closed.
func splitTokenArgs(toks []parser.Token, open int) ([]tokenSpan, int) {
	if open >= len(toks) || toks[open].Kind != parser.TokenOpenParen {
		return nil, -1
	}
	var args []tokenSpan
	depth, start := 0, open+1
	for k := open; k < len(toks); k++ {
		switch toks[k].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
			if depth == 0 {
				if k > start {
					args = append(args, tokenSpan{start, k})
				}
				return args, k
			}
		case parser.TokenComma:
			if depth == 1 {
				args = append(args, tokenSpan{start, k})
				start = k + 1
			}
		case parser.TokenSemicolon, parser.TokenEOF:
			return nil, -1
		}
	}
	return nil, -1
}

// literalString returns the value of a span that is one string literal with
// nothing interpolated.
func literalString(toks []parser.Token, s tokenSpan) (string, bool) {
	if s.end-s.start != 1 || toks[s.start].Kind != parser.TokenStringLiteral {
		return "", false
	}
	v := toks[s.start].Value
	if len(v) < 2 || (v[0] == '"' && strings.ContainsAny(v, "${")) {
		return "", false
	}
	return v[1 : len(v)-1], true
}

// readData reads a render's data argument: an array literal, compact(), or
// a variable assigned an array literal earlier in the same function.
func readData(uri, source string, toks []parser.Token, s tokenSpan, call int) ([]ViewVar, bool) {
	if s.end <= s.start {
		return nil, false
	}
	first := toks[s.start]
	switch {
	case first.Kind == parser.TokenOpenBracket && matchingClose(toks, s.start) == s.end-1:
		return readArray(source, toks, tokenSpan{s.start + 1, s.end - 1})
	case first.Kind == parser.TokenIdentifier && strings.EqualFold(first.Value, "compact") && s.end-s.start >= 3:
		return readCompact(toks, s)
	case first.Kind == parser.TokenVariable && s.end-s.start == 1:
		return readAssignedArray(source, toks, first.Value, call)
	}
	return nil, true
}

// readWith reads a `->with()` call: with('key', $value), with([...]) or
// withKey($value).
func readWith(uri, source string, toks []parser.Token, method string, args []tokenSpan, call int) ([]ViewVar, bool) {
	if method != "with" {
		if len(args) != 1 || len(method) <= 4 {
			return nil, true
		}
		key := toks[args[0].start-2] // the method name token
		name := strings.ToLower(method[4:5]) + method[5:]
		v := exprVar(source, toks, name, args[0])
		v.Line, v.Col, v.EndCol = key.Line, key.Column+4, key.Column+len(method)
		return []ViewVar{v}, false
	}
	switch len(args) {
	case 1:
		return readData(uri, source, toks, args[0], call)
	case 2:
		name, ok := literalString(toks, args[0])
		if !ok {
			return nil, true
		}
		v := exprVar(source, toks, name, args[1])
		key := toks[args[0].start]
		v.Line, v.Col, v.EndCol = key.Line, key.Column+1, key.Column+1+len(name)
		return []ViewVar{v}, false
	}
	return nil, true
}

// readArray reads `'key' => expr` items; any other item makes the data
// opaque.
func readArray(source string, toks []parser.Token, inner tokenSpan) ([]ViewVar, bool) {
	var vars []ViewVar
	opaque := false
	for _, item := range splitItems(toks, inner) {
		arrow := -1
		for k := item.start; k < item.end; k++ {
			if toks[k].Kind == parser.TokenDoubleArrow {
				arrow = k
				break
			}
		}
		if arrow < 0 {
			opaque = true
			continue
		}
		name, ok := literalString(toks, tokenSpan{item.start, arrow})
		if !ok || !isVarName(name) {
			opaque = true
			continue
		}
		v := exprVar(source, toks, name, tokenSpan{arrow + 1, item.end})
		key := toks[item.start]
		v.Line, v.Col, v.EndCol = key.Line, key.Column+1, key.Column+1+len(name)
		vars = append(vars, v)
	}
	return vars, opaque
}

func readCompact(toks []parser.Token, s tokenSpan) ([]ViewVar, bool) {
	args, close := splitTokenArgs(toks, s.start+1)
	if close != s.end-1 {
		return nil, true
	}
	var vars []ViewVar
	opaque := false
	for _, a := range args {
		name, ok := literalString(toks, a)
		if !ok || !isVarName(name) {
			opaque = true
			continue
		}
		t := toks[a.start]
		vars = append(vars, ViewVar{
			Name: name, Line: t.Line, Col: t.Column + 1, EndCol: t.Column + 1 + len(name),
			Expr: "$" + name, ExprLine: t.Line, ExprCol: t.Column,
		})
	}
	return vars, opaque
}

// readAssignedArray reads `$data = [...]` assigned before the call in the
// same function, and the `$data['key'] = expr;` that follow it.
func readAssignedArray(source string, toks []parser.Token, name string, call int) ([]ViewVar, bool) {
	start := -1
	for k := call - 1; k >= 0; k-- {
		if toks[k].Kind == parser.TokenFunction {
			break
		}
		if toks[k].Kind == parser.TokenVariable && toks[k].Value == name && k+2 < len(toks) &&
			toks[k+1].Kind == parser.TokenEquals && toks[k+2].Kind == parser.TokenOpenBracket {
			start = k
			break
		}
	}
	if start < 0 {
		return nil, true
	}
	close := matchingClose(toks, start+2)
	if close < 0 || close > call {
		return nil, true
	}
	vars, opaque := readArray(source, toks, tokenSpan{start + 3, close})
	// $data['key'] = expr; between the assignment and the call.
	for k := close; k < call; k++ {
		if toks[k].Kind != parser.TokenVariable || toks[k].Value != name {
			continue
		}
		if k+4 < call && toks[k+1].Kind == parser.TokenOpenBracket && toks[k+3].Kind == parser.TokenCloseBracket && toks[k+4].Kind == parser.TokenEquals {
			key, ok := literalString(toks, tokenSpan{k + 2, k + 3})
			if !ok || !isVarName(key) {
				opaque = true
				continue
			}
			end := k + 5
			for end < call && toks[end].Kind != parser.TokenSemicolon {
				end++
			}
			v := exprVar(source, toks, key, tokenSpan{k + 5, end})
			kt := toks[k+2]
			v.Line, v.Col, v.EndCol = kt.Line, kt.Column+1, kt.Column+1+len(key)
			vars = append(vars, v)
			continue
		}
		if k+1 < call && toks[k+1].Kind != parser.TokenOpenBracket && toks[k+1].Kind != parser.TokenComma && toks[k+1].Kind != parser.TokenCloseParen {
			// Something else is done with the array: it may change.
			opaque = true
		}
	}
	return vars, opaque
}

// splitItems splits array items at top-level commas.
func splitItems(toks []parser.Token, s tokenSpan) []tokenSpan {
	var items []tokenSpan
	depth, start := 0, s.start
	for k := s.start; k < s.end; k++ {
		switch toks[k].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
		case parser.TokenComma:
			if depth == 0 {
				if k > start {
					items = append(items, tokenSpan{start, k})
				}
				start = k + 1
			}
		}
	}
	if s.end > start {
		items = append(items, tokenSpan{start, s.end})
	}
	return items
}

func matchingClose(toks []parser.Token, open int) int {
	depth := 0
	for k := open; k < len(toks); k++ {
		switch toks[k].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			depth--
			if depth == 0 {
				return k
			}
		}
	}
	return -1
}

// exprVar is a variable whose value is the expression in s.
func exprVar(source string, toks []parser.Token, name string, s tokenSpan) ViewVar {
	v := ViewVar{Name: name}
	if s.end <= s.start {
		return v
	}
	first, last := toks[s.start], toks[s.end-1]
	end := last.Offset + len(last.Value)
	if first.Offset < end && end <= len(source) {
		v.Expr = strings.TrimSpace(source[first.Offset:end])
		v.ExprLine, v.ExprCol = first.Line, first.Column
	}
	return v
}

func isVarName(s string) bool {
	if s == "" || ('0' <= s[0] && s[0] <= '9') {
		return false
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		if !(c == '_' || ('a' <= c && c <= 'z') || ('A' <= c && c <= 'Z') || ('0' <= c && c <= '9') || c >= 0x80) {
			return false
		}
	}
	return true
}

// parseViewEdges reads the views a Blade view renders inside it, and the
// data it passes them.
func parseViewEdges(uri, from, source string) []viewEdge {
	nodes := blade.Parse(source)
	lines := blade.NewLines(source)
	var edges []viewEdge
	for _, inc := range blade.Includes(source, nodes) {
		e := viewEdge{from: from, to: inc.View, isolated: inc.Isolated}
		for _, v := range inc.Vars {
			line, col := lines.Position(v.Key.Start)
			el, ec := lines.Position(v.Value.Start)
			vv := ViewVar{Name: v.Name, URI: uri, Line: line, Col: col, EndCol: col + len(v.Name), FromTemplate: true}
			if v.Value.End > v.Value.Start {
				vv.Expr = source[v.Value.Start:v.Value.End]
				vv.ExprLine, vv.ExprCol = el, ec
			}
			e.vars = append(e.vars, vv)
		}
		edges = append(edges, e)
	}
	sort.SliceStable(edges, func(i, j int) bool { return edges[i].to < edges[j].to })
	return edges
}
