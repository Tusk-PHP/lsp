package document

import (
	"math/bits"
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
	at      lineCursor
}

type lineCursor struct {
	line  int
	text  string
	ascii int
	bytes int
	units int
}

func newLineCursor(line int, text string) lineCursor {
	ascii := asciiPrefix(text)
	return lineCursor{line: line, text: text, ascii: ascii, bytes: ascii, units: ascii}
}

func (at *lineCursor) rewind() {
	at.bytes, at.units = at.ascii, at.ascii
}

func (at *lineCursor) fromClient(col int) int {
	if col <= at.ascii {
		return col
	}
	if col < at.units {
		at.rewind()
	}
	for at.bytes < len(at.text) {
		r, size := utf8.DecodeRuneInString(at.text[at.bytes:])
		units := utf16.RuneLen(r)
		if at.units+units > col {
			return at.bytes
		}
		at.units += units
		at.bytes += size
	}
	return len(at.text) + col - at.units
}

func NewColumns(text string, enc Encoding) *Columns {
	return &Columns{text: text, enc: enc, at: lineCursor{line: -1}}
}

func (c *Columns) ToClient(line, col int) int {
	at, ok := c.cursor(line, col)
	if !ok || col <= at.ascii {
		return col
	}
	if col < at.bytes {
		at.rewind()
	}
	for at.bytes < len(at.text) {
		r, size := utf8.DecodeRuneInString(at.text[at.bytes:])
		if at.bytes+size > col {
			break
		}
		at.units += utf16.RuneLen(r)
		at.bytes += size
	}
	return at.units + max(col-len(at.text), 0)
}

func (c *Columns) FromClient(line, col int) int {
	at, ok := c.cursor(line, col)
	if !ok {
		return col
	}
	return at.fromClient(col)
}

func (c *Columns) cursor(line, col int) (*lineCursor, bool) {
	if c.enc == UTF8 || col <= 0 || line < 0 {
		return nil, false
	}
	if !c.scanned {
		c.scanned = true
		if !isASCII(c.text) {
			c.starts = lineStarts(c.text)
		}
	}
	if line >= len(c.starts) {
		return nil, false
	}
	if c.at.line != line {
		end := len(c.text)
		if line+1 < len(c.starts) {
			end = c.starts[line+1] - 1
		}
		c.at = newLineCursor(line, c.text[c.starts[line]:end])
	}
	return &c.at, true
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
	return asciiPrefix(s) == len(s)
}

func asciiPrefix(s string) int {
	const high = 0x8080808080808080
	i := 0
	for ; i+8 <= len(s); i += 8 {
		w := uint64(s[i]) | uint64(s[i+1])<<8 | uint64(s[i+2])<<16 | uint64(s[i+3])<<24 |
			uint64(s[i+4])<<32 | uint64(s[i+5])<<40 | uint64(s[i+6])<<48 | uint64(s[i+7])<<56
		if w&high != 0 {
			return i + bits.TrailingZeros64(w&high)/8
		}
	}
	for ; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			return i
		}
	}
	return i
}
