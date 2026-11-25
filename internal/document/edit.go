package document

import (
	"fmt"
	"strings"
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
	for i, c := range changes {
		next, err := apply(text, enc, c)
		if err != nil {
			return "", fmt.Errorf("change %d of %d: %w", i+1, len(changes), err)
		}
		text = next
	}
	return text, nil
}

func apply(text string, enc Encoding, c Change) (string, error) {
	if c.Range == nil {
		return c.Text, nil
	}
	start, err := offset(text, enc, c.Range.Start)
	if err != nil {
		return "", fmt.Errorf("range start: %w", err)
	}
	end, err := offset(text, enc, c.Range.End)
	if err != nil {
		return "", fmt.Errorf("range end: %w", err)
	}
	if end < start {
		return "", fmt.Errorf("range end %d:%d is before its start %d:%d", c.Range.End.Line, c.Range.End.Character, c.Range.Start.Line, c.Range.Start.Character)
	}
	var b strings.Builder
	b.Grow(len(text) - (end - start) + len(c.Text))
	b.WriteString(text[:start])
	b.WriteString(c.Text)
	b.WriteString(text[end:])
	return b.String(), nil
}

func offset(text string, enc Encoding, p Position) (int, error) {
	if p.Line < 0 || p.Character < 0 {
		return 0, fmt.Errorf("position %d:%d is negative", p.Line, p.Character)
	}
	start := 0
	for line := 0; line < p.Line; line++ {
		nl := strings.IndexByte(text[start:], '\n')
		if nl < 0 {
			return 0, fmt.Errorf("line %d is past the last line, %d", p.Line, line)
		}
		start += nl + 1
	}
	content := text[start:]
	if nl := strings.IndexByte(content, '\n'); nl >= 0 {
		content = content[:nl]
	}
	content = strings.TrimSuffix(content, "\r")
	col := NewColumns(content, enc).FromClient(0, p.Character)
	return start + min(col, len(content)), nil
}
