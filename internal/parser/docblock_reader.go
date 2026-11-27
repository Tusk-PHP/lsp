package parser

import (
	"iter"
	"strings"
	"unicode"
	"unicode/utf8"
)

// DocTag is one tag of a doc comment, before it's interpreted. A value whose
// brackets are still open at the end of its line goes on over the lines that
// follow, up to the line that closes them, the next tag or the end of the
// comment, and Value holds those lines joined into one.
type DocTag struct {
	Name   string
	Value  string
	start  int
	breaks []docBreak
}

type docBreak struct {
	at, offset int
}

// Offset returns where the byte Value[i] sits in the doc comment the tag was
// read from.
func (t DocTag) Offset(i int) int {
	offset := t.start + i
	for _, b := range t.breaks {
		if i < b.at {
			break
		}
		offset = b.offset + i - b.at
	}
	return offset
}

// DocTags yields the tags of a doc comment in order.
func DocTags(comment string) iter.Seq[DocTag] {
	return func(yield func(DocTag) bool) {
		for line := range docLines(comment) {
			if line.isTag && !yield(line.tag) {
				return
			}
		}
	}
}

type docLine struct {
	text  string
	tag   DocTag
	isTag bool
}

func docLines(comment string) iter.Seq[docLine] {
	return func(yield func(docLine) bool) {
		body, base := docBody(comment)
		var r docReader
		for pos := 0; pos < len(body); {
			end := strings.IndexByte(body[pos:], '\n')
			if end < 0 {
				end = len(body)
			} else {
				end += pos
			}
			text, offset := trimDocLine(body[pos:end])
			offset += base + pos
			pos = end + 1
			if text == "" {
				continue
			}
			if text[0] != '@' && r.continues(text, offset) {
				continue
			}
			if r.open && !yield(r.flush()) {
				return
			}
			if text[0] == '@' {
				r.start(text, offset)
				continue
			}
			if !yield(docLine{text: text}) {
				return
			}
		}
		if r.open {
			yield(r.flush())
		}
	}
}

type docReader struct {
	tag    DocTag
	text   string
	value  []byte
	depth  int
	open   bool
	joined bool
}

func (r *docReader) start(text string, offset int) {
	*r = docReader{tag: newDocTag(text, offset), text: text, value: r.value[:0], depth: -1, open: true}
}

func (r *docReader) continues(text string, offset int) bool {
	if !r.open {
		return false
	}
	if r.depth < 0 {
		r.depth = docBracketDepth(r.text, 0)
	}
	if r.depth == 0 {
		return false
	}
	if !r.joined {
		r.value = append(r.value, r.tag.Value...)
		r.joined = true
	}
	r.value = appendDocContinuation(r.value, &r.tag, text, offset)
	r.depth = docBracketDepth(text, r.depth)
	return true
}

func (r *docReader) flush() docLine {
	r.open = false
	if r.joined {
		r.tag.Value = string(r.value)
	}
	return docLine{tag: r.tag, isTag: true}
}

func docBody(comment string) (string, int) {
	body := trimDocSpaceLeft(comment)
	base := len(comment) - len(body)
	body = strings.TrimSpace(body)
	if strings.HasPrefix(body, "/*") {
		body, base = body[2:], base+2
	}
	return strings.TrimSuffix(body, "*/"), base
}

func trimDocLine(line string) (string, int) {
	text := trimDocSpaceLeft(line)
	if strings.HasPrefix(text, "*") {
		text = trimDocSpaceLeft(text[1:])
	}
	return strings.TrimSpace(text), len(line) - len(text)
}

func trimDocSpaceLeft(s string) string {
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= utf8.RuneSelf:
			return strings.TrimLeftFunc(s[i:], unicode.IsSpace)
		case c != ' ' && c != '\t' && c != '\n' && c != '\r' && c != '\v' && c != '\f':
			return s[i:]
		}
	}
	return ""
}

func newDocTag(text string, offset int) DocTag {
	name, value := splitDocTag(text[1:])
	start := offset + len(text)
	if value != "" {
		start -= len(value)
	}
	return DocTag{Name: name, Value: value, start: start}
}

func splitDocTag(s string) (name, value string) {
	for end := 0; end < len(s); end++ {
		if s[end] == ' ' || s[end] == '\t' {
			return s[:end], strings.TrimSpace(s[end+1:])
		}
	}
	return s, ""
}

func appendDocContinuation(value []byte, tag *DocTag, text string, offset int) []byte {
	switch {
	case len(value) == 0, isDocOpener(value[len(value)-1]):
	case isDocCloser(text[0]):
		if value[len(value)-1] == ',' {
			value = value[:len(value)-1]
		}
	default:
		value = append(value, ' ')
	}
	if len(value) == 0 {
		tag.start = offset
	} else {
		tag.breaks = append(tag.breaks, docBreak{at: len(value), offset: offset})
	}
	return append(value, text...)
}

func docBracketDepth(s string, depth int) int {
	if depth == 0 && strings.IndexAny(s, "<{([") < 0 {
		return 0
	}
	quote := byte(0)
	for i := 0; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == quote && s[i-1] != '\\' {
				quote = 0
			}
			continue
		}
		switch {
		case c == '\'' || c == '"':
			quote = c
		case isDocOpener(c):
			depth++
		case isDocCloser(c) && depth > 0:
			depth--
		}
	}
	return depth
}

func isDocOpener(c byte) bool {
	return c == '<' || c == '{' || c == '(' || c == '['
}

func isDocCloser(c byte) bool {
	return c == '>' || c == '}' || c == ')' || c == ']'
}
