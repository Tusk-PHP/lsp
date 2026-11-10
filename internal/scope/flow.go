package scope

import (
	"sort"
	"strings"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// The collector records enough of a file's control flow to tell which writes
// to a variable can reach a read: the braced blocks and what opens them, the
// jumps out of them (break, continue, goto) and the case labels that enter a
// switch part-way. It's not flow analysis: where the shape is one this can't
// follow, a write is taken to reach rather than not, so a variable's type gets
// wider, never wrong.

type blockKind uint8

const (
	blockOther blockKind = iota
	blockIf
	blockElseIf
	blockElse
	blockLoop
	blockSwitch
	blockTry
)

// block is a pair of braces.
type block struct {
	open, close protocol.Position // the `{` and the `}`
	parent      int               // the block around it, or -1
	kind        blockKind
	// prev and next link the blocks of one if/elseif/else chain, or -1.
	prev, next int
}

// flowMark is a jump, with the loop or switch it leaves (-1 when that isn't
// known).
type flowMark struct {
	at    protocol.Position
	block int
}

// flowState follows the open blocks while the collector walks the tokens.
type flowState struct {
	doc  *Document
	open []int       // indexes into doc.blocks of the blocks still open
	ifs  map[int]int // `}` token of an if or elseif block → the block
}

func newFlowState(doc *Document, tokens []parser.Token) *flowState {
	n := 0
	for i := range tokens {
		if tokens[i].Kind == parser.TokenOpenBrace {
			n++
		}
	}
	doc.blocks = make([]block, 0, n)
	return &flowState{doc: doc, ifs: map[int]int{}}
}

// current is the innermost open block, or -1.
func (f *flowState) current() int {
	if len(f.open) == 0 {
		return -1
	}
	return f.open[len(f.open)-1]
}

// openBlock records the block opened by the `{` at token i and returns it.
func (f *flowState) openBlock(tokens []parser.Token, i int) int {
	b := block{open: tokenRange(tokens[i]).Start, parent: f.current(), prev: -1, next: -1}
	b.kind, b.prev = f.blockOpener(tokens, i)
	idx := len(f.doc.blocks)
	if b.prev >= 0 {
		f.doc.blocks[b.prev].next = idx
	}
	f.doc.blocks = append(f.doc.blocks, b)
	f.open = append(f.open, idx)
	return idx
}

// closeBlock ends the innermost block at the `}` at token i.
func (f *flowState) closeBlock(tokens []parser.Token, i int) {
	idx := f.current()
	if idx < 0 {
		return
	}
	f.open = f.open[:len(f.open)-1]
	b := &f.doc.blocks[idx]
	b.close = tokenRange(tokens[i]).Start
	if b.kind == blockIf || b.kind == blockElseIf {
		f.ifs[i] = idx
	}
}

// closeAll ends the blocks left open at end.
func (f *flowState) closeAll(end protocol.Position) {
	for _, idx := range f.open {
		f.doc.blocks[idx].close = end
	}
	f.open = nil
}

// keyword records a jump or a case label at token i.
func (f *flowState) keyword(tokens []parser.Token, i int) {
	switch strings.ToLower(tokens[i].Value) {
	case "break", "continue":
		target := -1
		// `break 2` leaves more than the innermost loop or switch.
		if n := nextSignificant(tokens, i+1); n < 0 || tokens[n].Kind != parser.TokenNumber {
			target = f.innermost(blockLoop, blockSwitch)
		}
		f.doc.jumps = append(f.doc.jumps, flowMark{at: tokenRange(tokens[i]).Start, block: target})
	case "goto":
		f.doc.jumps = append(f.doc.jumps, flowMark{at: tokenRange(tokens[i]).Start, block: -1})
	case "case", "default":
		if idx := f.current(); idx >= 0 {
			if f.doc.labels == nil {
				f.doc.labels = map[int][]protocol.Position{}
			}
			f.doc.labels[idx] = append(f.doc.labels[idx], tokenRange(tokens[i]).Start)
		}
	}
}

func (f *flowState) innermost(kinds ...blockKind) int {
	for j := len(f.open) - 1; j >= 0; j-- {
		for _, k := range kinds {
			if f.doc.blocks[f.open[j]].kind == k {
				return f.open[j]
			}
		}
	}
	return -1
}

// blockOpener tells what opens the `{` at token i and, for an elseif or else,
// the block before it in its chain.
func (f *flowState) blockOpener(tokens []parser.Token, i int) (blockKind, int) {
	prev := prevSignificant(tokens, i-1)
	if prev < 0 {
		return blockOther, -1
	}
	kw := prev
	if tokens[prev].Kind == parser.TokenCloseParen {
		open := matchingOpenParen(tokens, prev)
		if open < 0 {
			return blockOther, -1
		}
		kw = prevSignificant(tokens, open-1)
	}
	if kw < 0 || tokens[kw].Kind != parser.TokenIdentifier {
		return blockOther, -1
	}
	word := strings.ToLower(tokens[kw].Value)
	switch {
	case word == "if" && kw != prev:
		// `else if (…) {` continues a chain like `elseif`.
		if e := prevSignificant(tokens, kw-1); e >= 0 && tokens[e].Kind == parser.TokenIdentifier && strings.EqualFold(tokens[e].Value, "else") {
			return blockElseIf, f.chainBefore(tokens, e)
		}
		return blockIf, -1
	case word == "elseif" && kw != prev:
		return blockElseIf, f.chainBefore(tokens, kw)
	case word == "else" && kw == prev:
		return blockElse, f.chainBefore(tokens, kw)
	case (word == "while" || word == "for" || word == "foreach") && kw != prev, word == "do" && kw == prev:
		return blockLoop, -1
	case word == "switch" && kw != prev:
		return blockSwitch, -1
	case word == "try" && kw == prev:
		return blockTry, -1
	}
	return blockOther, -1
}

// chainBefore returns the if or elseif block whose `}` comes right before the
// keyword at token kw, or -1.
func (f *flowState) chainBefore(tokens []parser.Token, kw int) int {
	c := prevSignificant(tokens, kw-1)
	if c < 0 || tokens[c].Kind != parser.TokenCloseBrace {
		return -1
	}
	if idx, ok := f.ifs[c]; ok {
		return idx
	}
	return -1
}

func matchingOpenParen(tokens []parser.Token, close int) int {
	depth := 0
	for i := close; i >= 0; i-- {
		switch tokens[i].Kind {
		case parser.TokenCloseParen:
			depth++
		case parser.TokenOpenParen:
			depth--
			if depth == 0 {
				return i
			}
		}
	}
	return -1
}

// containsAt reports whether block idx holds p, -1 being the whole file.
func (d *Document) containsAt(idx int, p protocol.Position) bool {
	if idx < 0 {
		return true
	}
	b := &d.blocks[idx]
	return positionBefore(b.open, p) && !positionBefore(b.close, p)
}

// innermostBlockAt returns the innermost block holding p, or -1. Blocks are
// recorded in the order they open, so it's the last one opening before p, or
// one around that.
func (d *Document) innermostBlockAt(p protocol.Position) int {
	i := sort.Search(len(d.blocks), func(i int) bool { return !positionBefore(d.blocks[i].open, p) }) - 1
	for i >= 0 && !d.containsAt(i, p) {
		i = d.blocks[i].parent
	}
	return i
}

// maxJumpsChecked bounds the jumps looked at between two writes: past it, a
// jump is taken to leave, which only keeps an earlier write reaching.
const maxJumpsChecked = 32

// leavesBetween reports whether a jump between from and to may leave that
// stretch: one that leaves a loop or switch begun before from or ended after
// to, or one whose target isn't known.
func (d *Document) leavesBetween(from, to protocol.Position) bool {
	lo := sort.Search(len(d.jumps), func(i int) bool { return positionBefore(from, d.jumps[i].at) })
	for n, j := range d.jumps[lo:] {
		if !positionBefore(j.at, to) {
			return false
		}
		if n >= maxJumpsChecked || j.block < 0 {
			return true
		}
		t := &d.blocks[j.block]
		if !positionBefore(from, t.open) || !positionBefore(t.close, to) {
			return true
		}
	}
	return false
}

// entersBetween reports whether a case label between from and to lets code in
// at to without passing from: a label of block idx, which holds from, or of a
// block around it.
func (d *Document) entersBetween(idx int, from, to protocol.Position) bool {
	if len(d.labels) == 0 {
		return false
	}
	for ; idx >= 0; idx = d.blocks[idx].parent {
		labels := d.labels[idx]
		k := sort.Search(len(labels), func(i int) bool { return positionBefore(from, labels[i]) })
		if k < len(labels) && positionBefore(labels[k], to) {
			return true
		}
	}
	return false
}

// tryBetween reports whether a block from idx outwards, up to the first that
// holds p, is a try block: an exception there may skip what follows it in the
// block and still reach p through a catch.
func (d *Document) tryBetween(idx int, p protocol.Position) bool {
	for ; idx >= 0 && !d.containsAt(idx, p); idx = d.blocks[idx].parent {
		if d.blocks[idx].kind == blockTry {
			return true
		}
	}
	return false
}

// effect is where a write's value comes from and from where on it holds.
type effect struct {
	rhs protocol.Position // where the value is read: reads there still see the previous one
	end protocol.Position // the end of the assignment, where the new value holds
	// statement is set when the assignment is a whole statement ended by `;`.
	statement bool
}

// writeEffect returns the effect of the write to the variable at token i. An
// assignment takes effect at the end of its expression, so `$x = $x->next()`
// reads the previous value on its right; a foreach variable, or any other
// write, right after the variable.
func writeEffect(tokens []parser.Token, i int, foreach bool) effect {
	end := tokenRange(tokens[i]).End
	e := effect{rhs: end, end: end}
	op := assignmentOperator(tokens, i)
	if foreach || op < 0 {
		return e
	}
	if start := nextSignificant(tokens, op+1); start >= 0 {
		e.rhs = tokenRange(tokens[start]).Start
	}
	startsStatement := startsStatement(tokens, i)
	depth := 0
	for j := op + 1; j < len(tokens); j++ {
		switch tokens[j].Kind {
		case parser.TokenOpenParen, parser.TokenOpenBracket, parser.TokenOpenBrace:
			depth++
			continue
		case parser.TokenCloseParen, parser.TokenCloseBracket, parser.TokenCloseBrace:
			if depth > 0 {
				depth--
				continue
			}
		case parser.TokenSemicolon:
			if depth > 0 {
				continue
			}
			e.statement = startsStatement && tokens[j].Value == ";"
		case parser.TokenComma:
			if depth > 0 {
				continue
			}
		default:
			continue
		}
		e.end = tokenRange(tokens[j]).Start
		return e
	}
	if n := len(tokens); n > 0 {
		e.end = tokenRange(tokens[n-1]).End
	}
	return e
}

// startsStatement reports whether the variable at token i begins a statement:
// it follows the end of another, a brace, an open tag, a `case …:` or the
// condition of a braceless control structure.
func startsStatement(tokens []parser.Token, i int) bool {
	p := prevSignificant(tokens, i-1)
	if p < 0 {
		return true
	}
	switch tokens[p].Kind {
	case parser.TokenSemicolon, parser.TokenOpenBrace, parser.TokenCloseBrace, parser.TokenPHP, parser.TokenColon, parser.TokenCloseParen:
		return true
	case parser.TokenIdentifier:
		return strings.EqualFold(tokens[p].Value, "else") || strings.EqualFold(tokens[p].Value, "do")
	}
	return false
}

// isDynamicMemberName reports whether the variable at token i names a member,
// as in `$this->$key = …`, which assigns to the property, not to $key.
func isDynamicMemberName(tokens []parser.Token, i int) bool {
	p := prevSignificant(tokens, i-1)
	return p >= 0 && tokens[p].Kind == parser.TokenArrow
}

// isShortCircuitOperand reports whether the write at token i is on the right
// of `&&`, `||`, `and`, `or`, `??` or a ternary `?`, possibly in parentheses,
// so it may not run.
func isShortCircuitOperand(tokens []parser.Token, i int) bool {
	p := prevSignificant(tokens, i-1)
	for p >= 0 && tokens[p].Kind == parser.TokenOpenParen {
		p = prevSignificant(tokens, p-1)
	}
	if p < 0 {
		return false
	}
	switch tokens[p].Kind {
	case parser.TokenQuestion:
		return true
	case parser.TokenAmpersand, parser.TokenPipe:
		return p > 0 && tokens[p-1].Kind == tokens[p].Kind
	case parser.TokenIdentifier:
		return strings.EqualFold(tokens[p].Value, "and") || strings.EqualFold(tokens[p].Value, "or")
	}
	return false
}

// takesEffectBy reports whether the value written here is written by pos.
func (r *Rebind) takesEffectBy(pos protocol.Position) bool {
	return !positionBefore(pos, r.effect.end)
}

// declEffectOrToken is the declaration's effect, which is the variable itself
// when the declaration isn't an assignment.
func (b *Binding) declEffectOrToken() effect {
	if b.declEffect.end == (protocol.Position{}) {
		return effect{rhs: b.Decl.End, end: b.Decl.End}
	}
	return b.declEffect
}

func (b *Binding) writeEffectOf(write int) effect {
	if write == DeclWrite {
		return b.declEffectOrToken()
	}
	return b.Rebinds[write].effect
}
