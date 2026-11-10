package scope

import (
	"slices"
	"sort"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// DeclWrite stands for a binding's declaration among the writes ReachingWrites
// returns; any other write is an index into Binding.Rebinds.
const DeclWrite = -1

// NoWrite is Reach.Latest when no write comes before the read.
const NoWrite = -2

// MaxReachingWrites bounds the writes ReachingWrites looks at for one read, so
// a read costs at most this many steps, each checked against at most this
// many later writes. A read that more writes may reach, as after a switch with
// hundreds of cases assigning the variable, is left unknown (Reach.TooMany).
const MaxReachingWrites = 32

// Reach is what a binding can hold at a read.
type Reach struct {
	// Writes are the writes whose value may be the binding's at the read, in
	// the order they're written: DeclWrite or an index into Binding.Rebinds.
	Writes []int
	// OnWrite is set when the read is on the variable a write assigns to. Its
	// value there is that write's, the only one in Writes.
	OnWrite bool
	// Latest is the last write that has taken effect at the read (DeclWrite
	// when that's the declaration), or NoWrite.
	Latest int
	// TooMany is set when more than MaxReachingWrites writes would have to be
	// looked at; Writes is then empty and the value isn't known.
	TooMany bool
}

// Single reports whether the value at the read is the last write before it,
// or there's no write before it at all.
func (r Reach) Single() bool {
	return !r.TooMany && (len(r.Writes) == 0 || len(r.Writes) == 1 && r.Writes[0] == r.Latest)
}

// WriteSite is where a write is: the variable it assigns to (At), where its
// value is read from (RHS: reads there still see the previous value), and the
// end of the assignment, where the new value holds. Statement is set when the
// assignment is a whole statement ended by `;`.
type WriteSite struct {
	At, RHS, End protocol.Position
	Statement    bool
}

// Site returns where a write of the binding is.
func (b *Binding) Site(write int) WriteSite {
	e := b.writeEffectOf(write)
	at := b.Decl.Start
	if write != DeclWrite {
		at = b.Rebinds[write].At
	}
	return WriteSite{At: at, RHS: e.rhs, End: e.end, Statement: e.statement}
}

// Caught returns the classes a write catches, as written, when it is the
// variable of `catch (A|B $e)`; nil for any other write.
func (b *Binding) Caught(write int) []string {
	if write == DeclWrite {
		return b.declCaught
	}
	return b.Rebinds[write].Caught
}

// ReadsItself reports whether the value a write assigns reads the binding, as
// `$x = $x->next()` does.
func (b *Binding) ReadsItself(write int) bool {
	e := b.writeEffectOf(write)
	refs := b.References
	k := sort.Search(len(refs), func(i int) bool { return !positionBefore(refs[i].Start, e.rhs) })
	return k < len(refs) && positionBefore(refs[k].Start, e.end)
}

// AssignmentAround returns the write to the binding whose right-hand side
// pos is in, before that write takes effect: in `$x = $x->next()`, the second
// `$x` is.
func (b *Binding) AssignmentAround(pos protocol.Position) (WriteSite, bool) {
	k := sort.Search(len(b.Rebinds), func(i int) bool { return !positionBefore(b.Rebinds[i].At, pos) }) - 1
	if k >= 0 && !b.Rebinds[k].takesEffectBy(pos) {
		return b.Site(k), true
	}
	if d := b.declEffectOrToken(); positionBefore(b.Decl.Start, pos) && positionBefore(pos, d.end) {
		return b.Site(DeclWrite), true
	}
	return WriteSite{}, false
}

// onWrite returns the write whose variable pos is on, or NoWrite.
func (b *Binding) onWrite(pos protocol.Position) int {
	if containsPos(b.Decl, pos) {
		return DeclWrite
	}
	k := sort.Search(len(b.Rebinds), func(i int) bool { return positionBefore(pos, b.Rebinds[i].At) }) - 1
	if k >= 0 {
		at := b.Rebinds[k].At
		if at.Line == pos.Line && pos.Character <= at.Character+len(b.Name) {
			return k
		}
	}
	return NoWrite
}

// killer is a write, or an if/elseif/else chain that writes in every branch,
// after which earlier writes no longer reach: those inside block before at,
// with no jump out between them and end.
type killer struct {
	at, end protocol.Position
	block   int
}

// ReachingWrites returns the writes to b whose value b may hold at pos: the
// last one that certainly holds there, and every later one that may or may not
// have run, in a branch or loop that has closed or under a condition. In a loop,
// a write later in its body reaches a read earlier in it through the next
// iteration. A write holds from the end of its assignment, so the right-hand
// side of `$x = $x->next()` reads the previous value. A shape it can't follow
// makes a write reach rather than not.
func (d *Document) ReachingWrites(b *Binding, pos protocol.Position) Reach {
	reach := Reach{Latest: NoWrite}
	if b == nil {
		return reach
	}
	if w := b.onWrite(pos); w != NoWrite {
		reach.Writes, reach.OnWrite, reach.Latest = []int{w}, true, w
		return reach
	}
	// The last rebind with its variable before pos, then back to the last one
	// that has taken effect: only an assignment nested in another's right-hand
	// side ends after a later one starts.
	latest := sort.Search(len(b.Rebinds), func(i int) bool { return !positionBefore(b.Rebinds[i].At, pos) }) - 1
	for steps := 0; latest >= 0 && !b.Rebinds[latest].takesEffectBy(pos); latest-- {
		if steps++; steps > MaxReachingWrites {
			return Reach{Latest: NoWrite, TooMany: true}
		}
	}
	declBefore := !positionBefore(pos, b.declEffectOrToken().end)
	switch {
	case latest >= 0:
		reach.Latest = latest
	case declBefore:
		reach.Latest = DeclWrite
	}

	// Each write examined adds at most two killers and one covered block, so
	// these stay on the stack.
	var killerBuf [2 * MaxReachingWrites]killer
	var coveredBuf, writeBuf [MaxReachingWrites]int
	killers := killerBuf[:0]
	covered := coveredBuf[:0] // blocks of an if/elseif/else chain written in throughout
	writes := writeBuf[:0]
	stopAt, stopped := protocol.Position{}, false
	for i, steps := latest, 0; i >= 0 && !stopped; i-- {
		if steps++; steps > MaxReachingWrites {
			return Reach{Latest: reach.Latest, TooMany: true}
		}
		rb := &b.Rebinds[i]
		if !rb.takesEffectBy(pos) || d.killed(rb.At, killers, pos) {
			continue
		}
		writes = append(writes, i)
		if rb.HoldsAt(pos) && !d.entersBetween(rb.block, rb.At, pos) {
			stopAt, stopped = rb.At, true
			break
		}
		if rb.Conditional {
			continue
		}
		killers = append(killers, killer{at: rb.At, end: rb.At, block: rb.block})
		if rb.block < 0 {
			continue
		}
		if k := d.blocks[rb.block].kind; k == blockIf || k == blockElseIf || k == blockElse {
			covered = append(covered, rb.block)
			if head, last, ok := d.chainCovered(rb.block, covered); ok {
				k := killer{at: d.blocks[head].open, end: d.blocks[last].close, block: d.blocks[head].parent}
				killers = append(killers, k)
				// Every way to pos goes through the chain: nothing before it reaches.
				if d.containsAt(k.block, pos) && !d.leavesBetween(k.at, k.end) && !d.entersBetween(k.block, k.at, pos) {
					stopAt, stopped = k.at, true
				}
			}
		}
	}
	if !stopped && declBefore && !d.killed(b.Decl.Start, killers, pos) {
		writes = append(writes, DeclWrite)
		stopAt, stopped = b.Decl.Start, true
	}
	reach.Writes = append(make([]int, 0, len(writes)+1), writes...)
	reach = d.addLoopWrites(b, pos, reach, stopAt, stopped)
	// DeclWrite is -1 and comes first, and rebinds are in order.
	slices.Sort(reach.Writes)
	return reach
}

// killed reports whether a write at u is overwritten on every way from it to
// pos by one of killers.
func (d *Document) killed(u protocol.Position, killers []killer, pos protocol.Position) bool {
	for _, k := range killers {
		if positionBefore(u, k.at) && d.containsAt(k.block, u) && !d.leavesBetween(u, k.end) && !d.tryBetween(k.block, pos) {
			return true
		}
	}
	return false
}

// chainCovered reports whether the if/elseif/else chain that block idx is in
// ends in an else and has a write in each of its blocks, and returns its first
// and last block.
func (d *Document) chainCovered(idx int, covered []int) (head, last int, ok bool) {
	head = idx
	for d.blocks[head].prev >= 0 {
		head = d.blocks[head].prev
	}
	if d.blocks[head].kind != blockIf {
		return 0, 0, false
	}
	for last = head; ; last = d.blocks[last].next {
		if !slices.Contains(covered, last) {
			return 0, 0, false
		}
		if d.blocks[last].next < 0 {
			break
		}
	}
	return head, last, d.blocks[last].kind == blockElse
}

// addLoopWrites adds the writes that reach pos from a later point of a loop
// around it, through the next iteration: those that take effect after pos in
// a loop that started after the write the walk stopped at.
func (d *Document) addLoopWrites(b *Binding, pos protocol.Position, reach Reach, stopAt protocol.Position, stopped bool) Reach {
	end, inLoop := protocol.Position{}, false
	for idx := d.innermostBlockAt(pos); idx >= 0; idx = d.blocks[idx].parent {
		bl := &d.blocks[idx]
		if bl.kind == blockLoop && (!stopped || positionBefore(stopAt, bl.open)) {
			end, inLoop = bl.close, true
		}
	}
	if !inLoop {
		return reach
	}
	if positionBefore(pos, b.declEffectOrToken().end) && !positionBefore(end, b.Decl.Start) {
		reach.Writes = append(reach.Writes, DeclWrite)
	}
	// From the first rebind that hasn't taken effect at pos.
	i := sort.Search(len(b.Rebinds), func(i int) bool { return !positionBefore(b.Rebinds[i].At, pos) })
	for i > 0 && !b.Rebinds[i-1].takesEffectBy(pos) {
		i--
	}
	for ; i < len(b.Rebinds) && !positionBefore(end, b.Rebinds[i].At); i++ {
		if b.Rebinds[i].takesEffectBy(pos) {
			continue
		}
		if len(reach.Writes) >= MaxReachingWrites {
			return Reach{Latest: reach.Latest, TooMany: true}
		}
		reach.Writes = append(reach.Writes, i)
	}
	return reach
}
