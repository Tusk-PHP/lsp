package document

import (
	"bytes"
	"fmt"
	"slices"
	"unsafe"
)

type Position struct {
	Line      int
	Character int
}

type Range struct {
	Start Position
	End   Position
}

type Change struct {
	Range *Range
	Text  string
}

func Apply(text string, enc Encoding, changes []Change) (string, error) {
	first := 0
	if full := lastFullText(changes); full >= 0 {
		text, first = changes[full].Text, full+1
	}
	if first == len(changes) {
		return text, nil
	}
	size := len(text)
	for _, c := range changes[first:] {
		size += len(c.Text)
	}
	e := editor{buf: make([]byte, len(text), size), enc: enc}
	copy(e.buf, text)
	for i := first; i < len(changes); i++ {
		if err := e.apply(*changes[i].Range, changes[i].Text); err != nil {
			return "", fmt.Errorf("change %d of %d: %w", i+1, len(changes), err)
		}
	}
	return unsafe.String(unsafe.SliceData(e.buf), len(e.buf)), nil
}

func lastFullText(changes []Change) int {
	for i, c := range slices.Backward(changes) {
		if c.Range == nil {
			return i
		}
	}
	return -1
}

type editor struct {
	buf       []byte
	enc       Encoding
	line      int
	lineStart int
}

func (e *editor) apply(r Range, text string) error {
	start, err := e.offset(r.Start)
	if err != nil {
		return fmt.Errorf("range start: %w", err)
	}
	anchorLine, anchorStart := e.line, e.lineStart
	end, err := e.offset(r.End)
	if err != nil {
		return fmt.Errorf("range end: %w", err)
	}
	if end < start {
		return fmt.Errorf("range end %d:%d is before its start %d:%d", r.End.Line, r.End.Character, r.Start.Line, r.Start.Character)
	}
	e.buf = splice(e.buf, start, end, text)
	e.line, e.lineStart = anchorLine, anchorStart
	return nil
}

func (e *editor) offset(p Position) (int, error) {
	if p.Line < 0 || p.Character < 0 {
		return 0, fmt.Errorf("position %d:%d is negative", p.Line, p.Character)
	}
	start, ok := e.seekLine(p.Line)
	if !ok {
		return len(e.buf), nil
	}
	content := e.buf[start:]
	if nl := bytes.IndexByte(content, '\n'); nl >= 0 {
		content = content[:nl]
	}
	content = bytes.TrimSuffix(content, []byte{'\r'})
	col := p.Character
	if e.enc != UTF8 {
		at := newLineCursor(0, unsafe.String(unsafe.SliceData(content), len(content)))
		col = at.fromClient(col)
	}
	return start + min(col, len(content)), nil
}

func (e *editor) seekLine(line int) (int, bool) {
	if line < e.line-line {
		e.line, e.lineStart = 0, 0
	}
	for e.line > line {
		e.lineStart = bytes.LastIndexByte(e.buf[:e.lineStart-1], '\n') + 1
		e.line--
	}
	for e.line < line {
		nl := bytes.IndexByte(e.buf[e.lineStart:], '\n')
		if nl < 0 {
			return 0, false
		}
		e.lineStart += nl + 1
		e.line++
	}
	return e.lineStart, true
}

func splice(buf []byte, start, end int, text string) []byte {
	oldLen := len(buf)
	size := oldLen - (end - start) + len(text)
	if size > oldLen {
		buf = slices.Grow(buf, size-oldLen)[:size]
	}
	copy(buf[start+len(text):], buf[end:oldLen])
	copy(buf[start:], text)
	return buf[:size]
}
