package document

type lineBreaks[T string | []byte] struct {
	s     T
	index func(T, byte) int
	nl    int
}

func newLineBreaks[T string | []byte](s T, index func(T, byte) int) *lineBreaks[T] {
	return &lineBreaks[T]{s: s, index: index, nl: -1}
}

func (b *lineBreaks[T]) lineEnd(from int) (end, next int) {
	if b.nl < from {
		b.nl = len(b.s)
		if i := b.index(b.s[from:], '\n'); i >= 0 {
			b.nl = from + i
		}
	}
	if cr := b.index(b.s[from:b.nl], '\r'); cr >= 0 {
		cr += from
		if cr+1 < len(b.s) && b.s[cr+1] == '\n' {
			return cr, cr + 2
		}
		return cr, cr + 1
	}
	if b.nl == len(b.s) {
		return len(b.s), -1
	}
	return b.nl, b.nl + 1
}

func previousLineStart(s []byte, lineStart int) int {
	i := lineStart - 1
	if s[i] == '\n' && i > 0 && s[i-1] == '\r' {
		i--
	}
	for i--; i >= 0; i-- {
		if s[i] == '\n' || s[i] == '\r' {
			return i + 1
		}
	}
	return 0
}
