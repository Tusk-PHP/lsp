package blade

import (
	"sort"
	"strings"
)

// Document is a template's PHP as a virtual PHP file. The PHP written in the
// template is copied into it unchanged; everything around it is generated, so
// only positions in copied text map back to the template.
type Document struct {
	// PHP is the virtual PHP source. It starts with `<?php` on a line of its own.
	PHP string
	// Structured is false when the template's blocks don't balance, such as an
	// `@if` without its `@endif`. Directives then become standalone statements,
	// so a broken template can't make the rest of the PHP misread.
	Structured bool

	template string
	segments []segment // copied text, by position in PHP
	phpLines []int     // offsets where each line of PHP starts
	tplLines []int     // offsets where each line of the template starts
}

// segment is template text copied into the virtual PHP.
type segment struct {
	php, template, length int
}

// Compile builds the virtual PHP document of a template.
func Compile(template string) *Document {
	nodes := Parse(template)
	doc := compile(template, nodes, true)
	if !doc.Structured {
		doc = compile(template, nodes, false)
	}
	doc.phpLines = lineStarts(doc.PHP)
	doc.tplLines = lineStarts(template)
	return doc
}

type block int

const (
	blockIf block = iota
	blockForeach
	blockForelse
	blockForelseEmpty
	blockFor
	blockWhile
	blockSwitch
)

type compiler struct {
	tpl      string
	out      strings.Builder
	segments []segment
	last     int // template offset up to which newlines have been carried over
	stack    []block
	ok       bool
}

func compile(template string, nodes []Node, structured bool) *Document {
	c := &compiler{tpl: template, ok: true}
	c.out.Grow(len(template) / 2)
	c.out.WriteString("<?php\n")
	for _, n := range nodes {
		c.lines(n.Span.Start)
		switch n.Kind {
		case KindEcho:
			c.emit("echo (")
			c.copy(n.Expr)
			c.emit(");")
		case KindPHP:
			if n.Raw {
				c.emit("echo (")
				c.copy(n.Expr)
				c.emit(");")
			} else {
				c.copy(n.Expr)
				c.emit("\n;")
			}
		case KindBoundAttribute:
			c.emit("[")
			c.copy(n.Expr)
			c.emit("];")
		case KindDirective:
			if structured {
				c.directive(n)
			} else {
				c.flatDirective(n)
			}
		}
		c.last = max(c.last, n.Span.Start)
	}
	c.lines(len(template))
	if len(c.stack) > 0 {
		c.ok = false
	}
	return &Document{PHP: c.out.String(), Structured: structured && c.ok, template: template, segments: c.segments}
}

// lines carries the template's line breaks up to offset into the PHP, so its
// lines stay roughly where they were.
func (c *compiler) lines(offset int) {
	if offset <= c.last {
		return
	}
	for i := strings.Count(c.tpl[c.last:offset], "\n"); i > 0; i-- {
		c.out.WriteByte('\n')
	}
	c.last = offset
}

func (c *compiler) emit(s string) { c.out.WriteString(s) }

// copy writes template text into the PHP and records where it came from.
func (c *compiler) copy(sp Span) {
	if sp.End <= sp.Start {
		return
	}
	c.segments = append(c.segments, segment{php: c.out.Len(), template: sp.Start, length: sp.End - sp.Start})
	c.out.WriteString(c.tpl[sp.Start:sp.End])
	// The copied text brought its own line breaks.
	c.last = max(c.last, sp.End)
}

// args writes a directive's arguments as an array, which checks each one as
// an expression: `@include('view', ['a' => $b])` becomes `['view', ['a' => $b]];`.
func (c *compiler) args(n Node) {
	if !n.HasArgs {
		return
	}
	c.emit("[")
	c.copy(n.Args)
	c.emit("];")
}

func (c *compiler) condition(n Node, d directive) {
	if !n.HasArgs {
		c.emit("true")
		return
	}
	switch d.cond {
	case condNot:
		c.emit("! (")
		c.copy(n.Args)
		c.emit(")")
	case condIsset:
		c.emit("isset(")
		c.copy(n.Args)
		c.emit(")")
	case condEmpty:
		c.emit("empty(")
		c.copy(n.Args)
		c.emit(")")
	case condOpaque:
		c.emit("[")
		c.copy(n.Args)
		c.emit("]")
	default:
		c.copy(n.Args)
	}
}

func (c *compiler) push(b block) { c.stack = append(c.stack, b) }

// pop closes the innermost block if it's one of want.
func (c *compiler) pop(want ...block) (block, bool) {
	if len(c.stack) == 0 {
		c.ok = false
		return 0, false
	}
	top := c.stack[len(c.stack)-1]
	for _, w := range want {
		if top == w {
			c.stack = c.stack[:len(c.stack)-1]
			return top, true
		}
	}
	c.ok = false
	return top, false
}

func (c *compiler) top() (block, bool) {
	if len(c.stack) == 0 {
		return 0, false
	}
	return c.stack[len(c.stack)-1], true
}

func (c *compiler) directive(n Node) {
	d := lookupDirective(n.Name)
	switch d.kind {
	case dirIf:
		// `@empty` with no argument inside `@forelse` is its else branch.
		if d.cond == condEmpty && !n.HasArgs {
			if top, ok := c.top(); ok && top == blockForelse {
				c.stack[len(c.stack)-1] = blockForelseEmpty
				c.emit("endforeach; if (true):")
				return
			}
		}
		c.emit("if (")
		c.condition(n, d)
		c.emit("):")
		c.push(blockIf)
	case dirElse:
		if top, ok := c.top(); !ok || top != blockIf {
			c.ok = false
		}
		c.emit("else:")
	case dirElseIf:
		if top, ok := c.top(); !ok || top != blockIf {
			c.ok = false
		}
		c.emit("elseif (")
		c.condition(n, d)
		c.emit("):")
	case dirEndIf:
		c.pop(blockIf)
		c.emit("endif;")
	case dirForeach, dirForelse:
		c.emit("foreach (")
		c.copy(n.Args)
		c.emit("):")
		if d.kind == dirForeach {
			c.push(blockForeach)
		} else {
			c.push(blockForelse)
		}
	case dirEndForeach:
		c.pop(blockForeach)
		c.emit("endforeach;")
	case dirEndForelse:
		if b, ok := c.pop(blockForelse, blockForelseEmpty); ok && b == blockForelseEmpty {
			c.emit("endif;")
		} else {
			c.emit("endforeach;")
		}
	case dirFor:
		c.emit("for (")
		c.copy(n.Args)
		c.emit("):")
		c.push(blockFor)
	case dirEndFor:
		c.pop(blockFor)
		c.emit("endfor;")
	case dirWhile:
		c.emit("while (")
		c.copy(n.Args)
		c.emit("):")
		c.push(blockWhile)
	case dirEndWhile:
		c.pop(blockWhile)
		c.emit("endwhile;")
	case dirSwitch:
		c.emit("switch (")
		c.copy(n.Args)
		c.emit("):")
		c.push(blockSwitch)
	case dirCase:
		c.emit("case (")
		c.copy(n.Args)
		c.emit("):")
	case dirDefault:
		c.emit("default:")
	case dirEndSwitch:
		c.pop(blockSwitch)
		c.emit("endswitch;")
	case dirBreak, dirContinue:
		word := "break;"
		if d.kind == dirContinue {
			word = "continue;"
		}
		if n.HasArgs {
			c.emit("if (")
			c.copy(n.Args)
			c.emit(") ")
		}
		c.emit(word)
	case dirUse:
		c.use(n)
	case dirArgs:
		c.args(n)
	}
}

// flatDirective writes a directive as a statement of its own, for templates
// whose blocks don't balance.
func (c *compiler) flatDirective(n Node) {
	d := lookupDirective(n.Name)
	switch d.kind {
	case dirForeach, dirForelse:
		c.emit("foreach (")
		c.copy(n.Args)
		c.emit(") {}")
	case dirFor:
		c.emit("for (")
		c.copy(n.Args)
		c.emit(") {}")
	case dirWhile, dirSwitch:
		c.emit("[")
		c.copy(n.Args)
		c.emit("];")
	case dirIf, dirElseIf, dirCase, dirBreak, dirContinue, dirArgs:
		c.args(n)
	case dirUse:
		c.use(n)
	}
}

// use writes `@use('App\Models\User')` or `@use('App\Models\User', 'Alias')`
// as an import, at the top level only, where PHP allows one.
func (c *compiler) use(n Node) {
	if !n.HasArgs || len(c.stack) > 0 {
		return
	}
	var parts []string
	for _, a := range strings.Split(c.tpl[n.Args.Start:n.Args.End], ",") {
		a = strings.TrimSpace(a)
		if len(a) < 2 || (a[0] != '\'' && a[0] != '"') || a[len(a)-1] != a[0] {
			return
		}
		parts = append(parts, a[1:len(a)-1])
	}
	if len(parts) == 0 || len(parts) > 2 || strings.ContainsAny(parts[0], "{}() ") {
		return
	}
	c.emit("use " + strings.TrimPrefix(parts[0], "\\"))
	if len(parts) == 2 {
		c.emit(" as " + parts[1])
	}
	c.emit(";")
}

func lineStarts(s string) []int {
	starts := make([]int, 1, strings.Count(s, "\n")+1)
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			starts = append(starts, i+1)
		}
	}
	return starts
}

// TemplateOffset maps a byte offset in the PHP to the template. ok is false
// for generated PHP, which has no place in the template.
func (d *Document) TemplateOffset(php int) (int, bool) {
	i := sort.Search(len(d.segments), func(i int) bool { return d.segments[i].php > php }) - 1
	if i < 0 {
		return 0, false
	}
	s := d.segments[i]
	if php >= s.php+s.length {
		return 0, false
	}
	return s.template + php - s.php, true
}

// TemplatePosition maps a 0-based line and byte column in the PHP to the
// template. ok is false for generated PHP.
func (d *Document) TemplatePosition(line, col int) (int, int, bool) {
	if line < 0 || line >= len(d.phpLines) {
		return 0, 0, false
	}
	off, ok := d.TemplateOffset(d.phpLines[line] + col)
	if !ok {
		return 0, 0, false
	}
	return d.templateLineCol(off)
}

// TemplateRange maps a range in the PHP to the template. Its start must be in
// copied text; an end past the copied text it starts in is cut to that text.
func (d *Document) TemplateRange(startLine, startCol, endLine, endCol int) (sl, sc, el, ec int, ok bool) {
	if startLine < 0 || startLine >= len(d.phpLines) {
		return 0, 0, 0, 0, false
	}
	start := d.phpLines[startLine] + startCol
	i := sort.Search(len(d.segments), func(i int) bool { return d.segments[i].php > start }) - 1
	if i < 0 || start >= d.segments[i].php+d.segments[i].length {
		return 0, 0, 0, 0, false
	}
	seg := d.segments[i]
	end := start
	if endLine >= 0 && endLine < len(d.phpLines) {
		end = d.phpLines[endLine] + endCol
	}
	end = min(max(end, start), seg.php+seg.length)
	sl, sc, _ = d.templateLineCol(seg.template + start - seg.php)
	el, ec, _ = d.templateLineCol(seg.template + end - seg.php)
	return sl, sc, el, ec, true
}

func (d *Document) templateLineCol(off int) (int, int, bool) {
	line := sort.Search(len(d.tplLines), func(i int) bool { return d.tplLines[i] > off }) - 1
	if line < 0 {
		return 0, 0, false
	}
	return line, off - d.tplLines[line], true
}

// PHPOffset maps a byte offset in the template to the PHP, when it is in PHP
// the template contains.
func (d *Document) PHPOffset(template int) (int, bool) {
	for _, s := range d.segments {
		if template >= s.template && template <= s.template+s.length {
			return s.php + template - s.template, true
		}
	}
	return 0, false
}
