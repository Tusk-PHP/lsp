package laravel

import (
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

// viewScan reads what one PHP file says about views from its tokens.
type viewScan struct {
	uri, source string
	toks        []parser.Token
	namespace   string
	imports     map[string]string // lower-case alias → class
	classes     []classSpan
	f           phpViewFile
}

// classSpan is a class declared in the file, by the tokens of its body.
type classSpan struct {
	fqn        string
	start, end int
}

// parseViewFile reads the views a PHP file renders, the data it shares with
// every view, the composers it registers and the data composer classes in
// it add.
func parseViewFile(uri, source string) phpViewFile {
	all := parser.Tokenize(source)
	toks := make([]parser.Token, 0, len(all))
	for _, t := range all {
		if t.Kind != parser.TokenComment && t.Kind != parser.TokenDocComment {
			toks = append(toks, t)
		}
	}
	s := &viewScan{uri: uri, source: source, toks: toks, imports: map[string]string{}}
	s.declarations()
	for i := 0; i+1 < len(toks); i++ {
		if next, ok := s.render(i); ok {
			i = next
			continue
		}
		if next, ok := s.content(i); ok {
			i = next
			continue
		}
		if next, ok := s.registration(i); ok {
			i = next
			continue
		}
		s.compose(i)
	}
	return s.f
}

func (s *viewScan) consume(view string) {
	if s.f.consumed == nil {
		s.f.consumed = map[uint64]int{}
	}
	s.f.consumed[viewHash(view)]++
}

func (s *viewScan) addRender(r viewRender) {
	r.uri = s.uri
	for k := range r.vars {
		r.vars[k].URI = s.uri
	}
	s.f.renders = append(s.f.renders, r)
}

// declarations reads the file's namespace, imports and classes.
func (s *viewScan) declarations() {
	toks := s.toks
	for i := 0; i < len(toks); i++ {
		switch toks[i].Kind {
		case parser.TokenNamespace:
			s.namespace, _ = qualifiedName(toks, i+1)
		case parser.TokenUse:
			if s.classAt(i) != "" || i+1 < len(toks) && toks[i+1].Kind == parser.TokenOpenParen {
				continue // a trait or a closure's use
			}
			name, j := qualifiedName(toks, i+1)
			alias := name[strings.LastIndexByte(name, '\\')+1:]
			if j+1 < len(toks) && strings.EqualFold(toks[j].Value, "as") {
				alias = toks[j+1].Value
			}
			if name != "" {
				s.imports[strings.ToLower(alias)] = name
			}
		case parser.TokenClass:
			if i > 0 && (toks[i-1].Kind == parser.TokenDoubleColon || toks[i-1].Kind == parser.TokenNew) {
				continue
			}
			if i+1 >= len(toks) || toks[i+1].Kind != parser.TokenIdentifier {
				continue
			}
			open := i + 2
			for open < len(toks) && toks[open].Kind != parser.TokenOpenBrace {
				open++
			}
			end := parser.MatchingClose(toks, open)
			if end < 0 {
				continue
			}
			fqn := toks[i+1].Value
			if s.namespace != "" {
				fqn = s.namespace + `\` + fqn
			}
			s.classes = append(s.classes, classSpan{fqn: fqn, start: open, end: end})
		}
	}
}

// qualifiedName reads a name such as `App\Models\User` starting at toks[i],
// returning it and the index after it.
func qualifiedName(toks []parser.Token, i int) (string, int) {
	var b strings.Builder
	for ; i < len(toks); i++ {
		switch toks[i].Kind {
		case parser.TokenIdentifier:
			b.WriteString(toks[i].Value)
		case parser.TokenBackslash:
			if b.Len() > 0 {
				b.WriteByte('\\')
			}
		default:
			return strings.Trim(b.String(), `\`), i
		}
	}
	return strings.Trim(b.String(), `\`), i
}

// classAt returns the innermost class whose body contains toks[i].
func (s *viewScan) classAt(i int) string {
	fqn := ""
	for _, c := range s.classes {
		if c.start < i && i < c.end {
			fqn = c.fqn
		}
	}
	return fqn
}

// resolveClass resolves a class name as written in the file.
func (s *viewScan) resolveClass(name string) string {
	if strings.HasPrefix(name, `\`) {
		return name[1:]
	}
	first, rest, _ := strings.Cut(name, `\`)
	if target, ok := s.imports[strings.ToLower(first)]; ok {
		if rest != "" {
			return target + `\` + rest
		}
		return target
	}
	if s.namespace != "" {
		return s.namespace + `\` + name
	}
	return name
}

// renderCall says where a render call takes its view name and data.
type renderCall struct {
	nameArg, dataArg int
	open             int  // the index of its `(`
	chain            bool // whether `->with()` calls after it add data
	opaque           bool // whether the view gets data the call doesn't show
}

// render reads a render call starting at toks[i]: view(), View::make(),
// response()->view(), Route::view() or ->markdown(). It returns the index
// to go on from.
func (s *viewScan) render(i int) (int, bool) {
	toks := s.toks
	call, ok := renderCallAt(toks, i)
	if !ok {
		return 0, false
	}
	args, close := splitTokenArgs(toks, call.open)
	if close < 0 {
		return 0, false
	}
	if call.nameArg >= len(args) {
		return close, true // view() with no name, as in view()->share()
	}
	name, ok := literalString(toks, args[call.nameArg])
	if !ok || name == "" {
		s.f.dynamic++
		return close, true
	}
	s.consume(name)
	r := viewRender{view: name, opaque: call.opaque, class: s.classAt(i)}
	if call.dataArg < len(args) {
		vars, opaque := readData(s.uri, s.source, toks, args[call.dataArg], i)
		r.vars, r.opaque = vars, r.opaque || opaque
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
			vars, opaque := readWith(s.uri, s.source, toks, method, wargs, i)
			r.vars = append(r.vars, vars...)
			r.opaque = r.opaque || opaque
			k = wclose + 1
			close = wclose
		}
	}
	s.addRender(r)
	return close, true
}

// renderCallAt reports whether a render call starts at toks[i].
func renderCallAt(toks []parser.Token, i int) (renderCall, bool) {
	t := toks[i]
	if t.Kind != parser.TokenIdentifier || i+1 >= len(toks) {
		return renderCall{}, false
	}
	prev := parser.TokenEOF
	if i > 0 {
		prev = toks[i-1].Kind
	}
	switch {
	case (t.Value == "view" || t.Value == "markdown") && toks[i+1].Kind == parser.TokenOpenParen:
		if prev == parser.TokenArrow {
			// response()->view('name', $data); a Mailable's or a mail
			// message's ->view() and ->markdown(), whose views get more data
			// than they're passed.
			return renderCall{nameArg: 0, dataArg: 1, open: i + 1, opaque: t.Value == "markdown"}, true
		}
		if t.Value == "markdown" || prev == parser.TokenDoubleColon || prev == parser.TokenFunction || prev == parser.TokenNew {
			return renderCall{}, false
		}
		return renderCall{nameArg: 0, dataArg: 1, open: i + 1, chain: true}, true
	case (t.Value == "View" || t.Value == "Route") && i+3 < len(toks) && toks[i+1].Kind == parser.TokenDoubleColon && toks[i+3].Kind == parser.TokenOpenParen:
		if prev == parser.TokenArrow || prev == parser.TokenDoubleColon {
			return renderCall{}, false
		}
		method := toks[i+2].Value
		if t.Value == "View" && method == "make" {
			return renderCall{nameArg: 0, dataArg: 1, open: i + 3, chain: true}, true
		}
		if t.Value == "Route" && method == "view" {
			return renderCall{nameArg: 1, dataArg: 2, open: i + 3}, true
		}
	}
	return renderCall{}, false
}

// content reads a Mailable's `new Content(view: 'mail.x', with: [...])`.
func (s *viewScan) content(i int) (int, bool) {
	toks := s.toks
	if toks[i].Kind != parser.TokenNew || i+2 >= len(toks) {
		return 0, false
	}
	name, open := qualifiedName(toks, i+1)
	if name == "" || name[strings.LastIndexByte(name, '\\')+1:] != "Content" || open >= len(toks) || toks[open].Kind != parser.TokenOpenParen {
		return 0, false
	}
	args, close := splitTokenArgs(toks, open)
	if close < 0 {
		return 0, false
	}
	var views []string
	markdown := false
	var vars []ViewVar
	opaque := false
	for _, a := range args {
		if a.end-a.start < 3 || toks[a.start].Kind != parser.TokenIdentifier || toks[a.start+1].Kind != parser.TokenColon {
			opaque = true // positional arguments
			continue
		}
		value := tokenSpan{a.start + 2, a.end}
		switch key := toks[a.start].Value; key {
		case "view", "html", "text", "markdown":
			v, ok := literalString(toks, value)
			if !ok {
				s.f.dynamic++
				continue
			}
			s.consume(v)
			views = append(views, v)
			markdown = markdown || key == "markdown"
		case "with":
			vars, opaque = readData(s.uri, s.source, toks, value, i)
		}
	}
	class := s.classAt(i)
	for _, v := range views {
		r := viewRender{view: v, vars: append([]ViewVar(nil), vars...), opaque: opaque || markdown, class: class}
		s.addRender(r)
	}
	return close, true
}

// registration reads View::share(), View::composer(), View::creator() and
// View::first(), or the same on view().
func (s *viewScan) registration(i int) (int, bool) {
	toks := s.toks
	t := toks[i]
	if t.Kind != parser.TokenIdentifier || i+1 >= len(toks) || toks[i+1].Kind != parser.TokenOpenParen {
		return 0, false
	}
	onFacade := i >= 2 && toks[i-1].Kind == parser.TokenDoubleColon && toks[i-2].Value == "View"
	onHelper := i >= 4 && toks[i-1].Kind == parser.TokenArrow && toks[i-2].Kind == parser.TokenCloseParen &&
		toks[i-3].Kind == parser.TokenOpenParen && toks[i-4].Value == "view"
	if !onFacade && !onHelper {
		return 0, false
	}
	args, close := splitTokenArgs(toks, i+1)
	if close < 0 {
		return 0, false
	}
	switch t.Value {
	case "share":
		r := viewRender{view: sharedView}
		switch len(args) {
		case 1:
			r.vars, r.opaque = readData(s.uri, s.source, toks, args[0], i)
		case 2:
			if name, ok := literalString(toks, args[0]); ok {
				v := exprVar(s.source, toks, name, args[1])
				key := toks[args[0].start]
				v.Line, v.Col, v.EndCol = key.Line, key.Column+1, key.Column+1+len(name)
				r.vars = []ViewVar{v}
			} else {
				r.opaque = true
			}
		default:
			r.opaque = true
		}
		s.addRender(r)
	case "composer", "creator", "composers":
		s.composer(args, i)
	case "first":
		if len(args) == 0 || toks[args[0].start].Kind != parser.TokenOpenBracket {
			s.f.dynamic++
			break
		}
		for _, item := range splitItems(toks, tokenSpan{args[0].start + 1, args[0].end - 1}) {
			name, ok := literalString(toks, item)
			if !ok {
				s.f.dynamic++
				continue
			}
			s.consume(name)
			s.addRender(viewRender{view: name, opaque: true})
		}
	default:
		return 0, false
	}
	return close, true
}

// composer reads a composer registration: its views, and its class or the
// data its closure adds.
func (s *viewScan) composer(args []tokenSpan, call int) {
	toks := s.toks
	c := viewComposer{}
	if len(args) != 2 {
		c.views, c.opaque = []string{sharedView}, true
		s.f.composers = append(s.f.composers, c)
		return
	}
	// The views: a name, or a list of names.
	if name, ok := literalString(toks, args[0]); ok {
		c.views = []string{name}
	} else if toks[args[0].start].Kind == parser.TokenOpenBracket {
		for _, item := range splitItems(toks, tokenSpan{args[0].start + 1, args[0].end - 1}) {
			name, ok := literalString(toks, item)
			if !ok {
				c.views, c.opaque = []string{sharedView}, true
				break
			}
			c.views = append(c.views, name)
		}
	} else {
		c.views, c.opaque = []string{sharedView}, true
	}
	for _, v := range c.views {
		if v != sharedView {
			s.consume(v)
		}
	}
	// The composer: Composer::class, 'App\…\Composer', or a closure.
	a := args[1]
	switch {
	case a.end-a.start >= 3 && toks[a.end-1].Value == "class" && toks[a.end-2].Kind == parser.TokenDoubleColon:
		name, _ := qualifiedName(toks, a.start)
		c.class = s.resolveClass(name)
	case a.end-a.start == 1 && toks[a.start].Kind == parser.TokenStringLiteral:
		if name, ok := literalString(toks, a); ok && strings.Contains(name, `\`) {
			c.class = strings.TrimPrefix(name, `\`)
		} else {
			c.opaque = true
		}
	default:
		vars, opaque, ok := s.closureWith(a)
		if !ok {
			c.opaque = true
		}
		c.vars, c.opaque = vars, c.opaque || opaque
	}
	for k := range c.vars {
		c.vars[k].URI = s.uri
	}
	s.f.composers = append(s.f.composers, c)
}

// closureWith reads the data a closure composer adds: the ->with() calls on
// its first parameter.
func (s *viewScan) closureWith(a tokenSpan) ([]ViewVar, bool, bool) {
	toks := s.toks
	k := a.start
	if toks[k].Kind == parser.TokenStatic {
		k++
	}
	if k >= a.end || (toks[k].Kind != parser.TokenFunction && toks[k].Value != "fn") || k+1 >= a.end || toks[k+1].Kind != parser.TokenOpenParen {
		return nil, true, false
	}
	params, pclose := splitTokenArgs(toks, k+1)
	if pclose < 0 || len(params) == 0 {
		return nil, true, len(params) == 0 && pclose >= 0
	}
	param := lastVariable(toks, params[0])
	body := tokenSpan{pclose + 1, a.end}
	vars, opaque := s.withCalls(param, body)
	return vars, opaque, param != ""
}

// compose reads `public function compose(View $view)` in a class, and the
// data its ->with() calls on $view add.
func (s *viewScan) compose(i int) {
	toks := s.toks
	if toks[i].Kind != parser.TokenFunction || i+2 >= len(toks) || toks[i+1].Value != "compose" || toks[i+2].Kind != parser.TokenOpenParen {
		return
	}
	class := s.classAt(i)
	if class == "" {
		return
	}
	params, pclose := splitTokenArgs(toks, i+2)
	if pclose < 0 || len(params) == 0 {
		return
	}
	open := pclose + 1
	for open < len(toks) && toks[open].Kind != parser.TokenOpenBrace && toks[open].Kind != parser.TokenSemicolon {
		open++
	}
	if open >= len(toks) || toks[open].Kind != parser.TokenOpenBrace {
		return
	}
	end := parser.MatchingClose(toks, open)
	if end < 0 {
		return
	}
	vars, opaque := s.withCalls(lastVariable(toks, params[0]), tokenSpan{open, end})
	if s.f.composes == nil {
		s.f.composes = map[string]viewRender{}
	}
	for k := range vars {
		vars[k].URI = s.uri
	}
	s.f.composes[class] = viewRender{uri: s.uri, view: class, vars: vars, opaque: opaque}
}

// withCalls reads the ->with() calls on a variable in a body. Anything else
// done with the variable may add data, and makes it opaque.
func (s *viewScan) withCalls(param string, body tokenSpan) ([]ViewVar, bool) {
	toks := s.toks
	if param == "" {
		return nil, true
	}
	var vars []ViewVar
	opaque := false
	for k := body.start; k < body.end; k++ {
		if toks[k].Kind != parser.TokenVariable || toks[k].Value != param {
			continue
		}
		if k+3 < body.end && toks[k+1].Kind == parser.TokenArrow && strings.HasPrefix(toks[k+2].Value, "with") && toks[k+3].Kind == parser.TokenOpenParen {
			args, close := splitTokenArgs(toks, k+3)
			if close < 0 {
				return vars, true
			}
			v, o := readWith(s.uri, s.source, toks, toks[k+2].Value, args, k)
			vars = append(vars, v...)
			opaque = opaque || o
			// ->with(…)->with(…)
			for close+3 < body.end && toks[close+1].Kind == parser.TokenArrow && strings.HasPrefix(toks[close+2].Value, "with") && toks[close+3].Kind == parser.TokenOpenParen {
				wargs, wclose := splitTokenArgs(toks, close+3)
				if wclose < 0 {
					break
				}
				v, o := readWith(s.uri, s.source, toks, toks[close+2].Value, wargs, k)
				vars = append(vars, v...)
				opaque = opaque || o
				close = wclose
			}
			k = close
			continue
		}
		opaque = true
	}
	return vars, opaque
}

// lastVariable returns the variable a parameter declares, such as `$view`
// in `View $view`.
func lastVariable(toks []parser.Token, s tokenSpan) string {
	for k := s.end - 1; k >= s.start; k-- {
		if toks[k].Kind == parser.TokenVariable {
			return toks[k].Value
		}
	}
	return ""
}
