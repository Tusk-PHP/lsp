package document

import (
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

type Encoding uint8

const (
	UTF16 Encoding = iota
	UTF8
)

func (e Encoding) String() string {
	if e == UTF8 {
		return "utf-8"
	}
	return "utf-16"
}

func Negotiate(offered []string) Encoding {
	for _, name := range offered {
		if name == UTF8.String() {
			return UTF8
		}
	}
	return UTF16
}

type Columns struct {
	text    string
	enc     Encoding
	scanned bool
	starts  []int
}

func NewColumns(text string, enc Encoding) *Columns {
	return &Columns{text: text, enc: enc}
}

func (c *Columns) ToClient(line, col int) int {
	text, ok := c.line(line, col)
	if !ok {
		return col
	}
	n := 0
	i := 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		if i+size > col {
			break
		}
		n += utf16.RuneLen(r)
		i += size
	}
	if col > len(text) {
		n += col - len(text)
	}
	return n
}

func (c *Columns) FromClient(line, col int) int {
	text, ok := c.line(line, col)
	if !ok {
		return col
	}
	n := 0
	i := 0
	for i < len(text) {
		r, size := utf8.DecodeRuneInString(text[i:])
		units := utf16.RuneLen(r)
		if n+units > col {
			return i
		}
		n += units
		i += size
	}
	return len(text) + col - n
}

func (c *Columns) line(line, col int) (string, bool) {
	if c.enc == UTF8 || col <= 0 || line < 0 {
		return "", false
	}
	if !c.scanned {
		c.scanned = true
		if !isASCII(c.text) {
			c.starts = lineStarts(c.text)
		}
	}
	if line >= len(c.starts) {
		return "", false
	}
	end := len(c.text)
	if line+1 < len(c.starts) {
		end = c.starts[line+1] - 1
	}
	text := c.text[c.starts[line]:end]
	if isASCII(text[:min(col, len(text))]) {
		return "", false
	}
	return text, true
}

func lineStarts(text string) []int {
	starts := make([]int, 1, strings.Count(text, "\n")+1)
	for i := 0; ; {
		j := strings.IndexByte(text[i:], '\n')
		if j < 0 {
			return starts
		}
		i += j + 1
		starts = append(starts, i)
	}
}

func isASCII(s string) bool {
	i := 0
	for ; i+8 <= len(s); i += 8 {
		w := uint64(s[i]) | uint64(s[i+1])<<8 | uint64(s[i+2])<<16 | uint64(s[i+3])<<24 |
			uint64(s[i+4])<<32 | uint64(s[i+5])<<40 | uint64(s[i+6])<<48 | uint64(s[i+7])<<56
		if w&0x8080808080808080 != 0 {
			return false
		}
	}
	for ; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return false
		}
	}
	return true
}
