package manifest

import (
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

// scanner is a simple byte-oriented JSON tokenizer that tracks 0-based
// line/column position. It does no UTF-8 decoding; the column count is in
// bytes, which matches what the LSP layer above us treats as "character"
// for JSON files (UTF-8 byte offsets are sufficient for composer.json
// content in practice, which is overwhelmingly ASCII).
type scanner struct {
	src  string
	pos  int
	line int
	col  int
}

func (s *scanner) peek() byte {
	if s.pos >= len(s.src) {
		return 0
	}
	return s.src[s.pos]
}

func (s *scanner) advance() {
	if s.pos >= len(s.src) {
		return
	}
	if s.src[s.pos] == '\n' {
		s.line++
		s.col = 0
	} else {
		s.col++
	}
	s.pos++
}

func (s *scanner) position() protocol.Position {
	return protocol.Position{Line: s.line, Character: s.col}
}

func (s *scanner) skipWS() {
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		if c == ' ' || c == '\t' || c == '\n' || c == '\r' {
			s.advance()
			continue
		}
		break
	}
}

// readString consumes a JSON string starting at the current opening
// double-quote. Returns the unescaped content, the range covering only
// the content (quotes excluded), and ok=false if the string was not
// terminated before EOF.
func (s *scanner) readString() (string, protocol.Range, bool) {
	if s.peek() != '"' {
		return "", protocol.Range{}, false
	}
	s.advance() // opening "
	start := s.position()
	var out []byte
	for s.pos < len(s.src) {
		c := s.src[s.pos]
		switch c {
		case '"':
			end := s.position()
			s.advance() // closing "
			return string(out), protocol.Range{Start: start, End: end}, true
		case '\\':
			s.advance()
			esc := s.peek()
			if esc == 0 {
				return string(out), protocol.Range{Start: start, End: s.position()}, false
			}
			switch esc {
			case '"', '\\', '/':
				out = append(out, esc)
			case 'n':
				out = append(out, '\n')
			case 't':
				out = append(out, '\t')
			case 'r':
				out = append(out, '\r')
			case 'b':
				out = append(out, '\b')
			case 'f':
				out = append(out, '\f')
			case 'u':
				// Skip the 4 hex digits. We don't decode unicode escapes —
				// composer.json package names and URLs don't use them in
				// practice, and the resulting string content is only used
				// for equality compares + URL building.
				s.advance()
				for i := 0; i < 4 && s.pos < len(s.src); i++ {
					s.advance()
				}
				continue
			default:
				out = append(out, esc)
			}
			s.advance()
		default:
			out = append(out, c)
			s.advance()
		}
	}
	return string(out), protocol.Range{Start: start, End: s.position()}, false
}

// skipValue consumes one JSON value of arbitrary type (string, number,
// literal, array, object). Returns false only on hard EOF mid-token, so
// callers can decide to bail.
func (s *scanner) skipValue() bool {
	s.skipWS()
	c := s.peek()
	switch c {
	case 0:
		return false
	case '"':
		_, _, ok := s.readString()
		return ok
	case '{':
		s.advance()
		return s.skipObject()
	case '[':
		s.advance()
		return s.skipArray()
	default:
		// number, true, false, null — read until structural char.
		for s.pos < len(s.src) {
			ch := s.src[s.pos]
			if ch == ',' || ch == '}' || ch == ']' || ch == ' ' || ch == '\t' || ch == '\n' || ch == '\r' {
				return true
			}
			s.advance()
		}
		return false
	}
}

func (s *scanner) skipObject() bool {
	for {
		s.skipWS()
		c := s.peek()
		if c == 0 {
			return false
		}
		if c == '}' {
			s.advance()
			return true
		}
		if c == ',' {
			s.advance()
			continue
		}
		// key
		if c != '"' {
			// recover by stepping one byte
			s.advance()
			continue
		}
		if _, _, ok := s.readString(); !ok {
			return false
		}
		s.skipWS()
		if s.peek() != ':' {
			continue
		}
		s.advance()
		if !s.skipValue() {
			return false
		}
	}
}

func (s *scanner) skipArray() bool {
	for {
		s.skipWS()
		c := s.peek()
		if c == 0 {
			return false
		}
		if c == ']' {
			s.advance()
			return true
		}
		if c == ',' {
			s.advance()
			continue
		}
		if !s.skipValue() {
			return false
		}
	}
}
