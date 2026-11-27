package config

import "strings"

// DotenvEntry is one assignment in a dotenv file. Line and Column are
// zero-based and locate the first byte of Key.
type DotenvEntry struct {
	Key    string
	Value  string
	Line   int
	Column int
}

type dotenvScanner struct {
	src  string
	pos  int
	line int
	col  int
}

// ParseDotenv reads the assignments of a dotenv file in the format
// vlucas/phpdotenv reads: `KEY=value` lines, optionally prefixed with
// `export`, `#` comments, and single- or double-quoted values that may span
// lines. Lines it can't read as an assignment are skipped.
func ParseDotenv(content string) []DotenvEntry {
	s := dotenvScanner{src: content}
	var entries []DotenvEntry
	for !s.done() {
		if entry, ok := s.assignment(); ok {
			entries = append(entries, entry)
		}
		s.skipLine()
	}
	return entries
}

func (s *dotenvScanner) done() bool {
	return s.pos >= len(s.src)
}

func (s *dotenvScanner) peek() byte {
	return s.src[s.pos]
}

func (s *dotenvScanner) advance() {
	if s.src[s.pos] == '\n' {
		s.line++
		s.col = 0
	} else {
		s.col++
	}
	s.pos++
}

func (s *dotenvScanner) skipBlanks() {
	for !s.done() && isDotenvBlank(s.peek()) {
		s.advance()
	}
}

func (s *dotenvScanner) skipLine() {
	for !s.done() && s.peek() != '\n' {
		s.advance()
	}
	if !s.done() {
		s.advance()
	}
}

func (s *dotenvScanner) word() string {
	start := s.pos
	for !s.done() && isDotenvKeyByte(s.peek()) {
		s.advance()
	}
	return s.src[start:s.pos]
}

func (s *dotenvScanner) assignment() (DotenvEntry, bool) {
	s.skipBlanks()
	line, col := s.line, s.col
	key := s.word()
	if key == "export" && !s.done() && isDotenvBlank(s.peek()) {
		s.skipBlanks()
		line, col = s.line, s.col
		key = s.word()
	}
	if key == "" {
		return DotenvEntry{}, false
	}
	s.skipBlanks()
	if s.done() || s.peek() != '=' {
		return DotenvEntry{}, false
	}
	s.advance()
	s.skipBlanks()
	return DotenvEntry{Key: key, Value: s.value(), Line: line, Column: col}, true
}

func (s *dotenvScanner) value() string {
	if s.done() {
		return ""
	}
	switch quote := s.peek(); quote {
	case '"', '\'':
		return s.quoted(quote)
	}
	start := s.pos
	for !s.done() && s.peek() != '\n' {
		if s.peek() == '#' && s.pos > start && isDotenvBlank(s.src[s.pos-1]) {
			break
		}
		s.advance()
	}
	return strings.TrimRight(s.src[start:s.pos], " \t\r")
}

func (s *dotenvScanner) quoted(quote byte) string {
	s.advance()
	var out strings.Builder
	for !s.done() {
		c := s.peek()
		if c == quote {
			s.advance()
			return out.String()
		}
		if c == '\\' && s.pos+1 < len(s.src) {
			next := s.src[s.pos+1]
			if decoded, ok := dotenvEscape(quote, next); ok {
				out.WriteByte(decoded)
				s.advance()
				s.advance()
				continue
			}
		}
		out.WriteByte(c)
		s.advance()
	}
	return out.String()
}

func dotenvEscape(quote, c byte) (byte, bool) {
	if c == quote || c == '\\' {
		return c, true
	}
	if quote == '\'' {
		return 0, false
	}
	switch c {
	case 'n':
		return '\n', true
	case 'r':
		return '\r', true
	case 't':
		return '\t', true
	case 'f':
		return '\f', true
	case 'v':
		return '\v', true
	case '$':
		return '$', true
	}
	return 0, false
}

func isDotenvBlank(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r'
}

func isDotenvKeyByte(c byte) bool {
	return c == '_' || c == '.' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9'
}
