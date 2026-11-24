package symbols

const (
	wordFilterWords  = 32
	wordFilterBits   = wordFilterWords * 64
	wordFilterHashes = 3
	fnvOffset        = uint64(14695981039346656037)
	fnvPrime         = uint64(1099511628211)
)

type wordFilter [wordFilterWords]uint64

var wordFold = func() (fold [256]byte) {
	for c := range 256 {
		switch {
		case 'A' <= c && c <= 'Z':
			fold[c] = byte(c) + 'a' - 'A'
		case c == '_' || c >= 0x80 || '0' <= c && c <= '9' || 'a' <= c && c <= 'z':
			fold[c] = byte(c)
		}
	}
	return fold
}()

func newWordFilter(source string) wordFilter {
	var f wordFilter
	h, inWord := fnvOffset, false
	for i := 0; i < len(source); i++ {
		if c := wordFold[source[i]]; c != 0 {
			h = (h ^ uint64(c)) * fnvPrime
			inWord = true
			continue
		}
		if inWord {
			f.add(h)
			h, inWord = fnvOffset, false
		}
	}
	if inWord {
		f.add(h)
	}
	return f
}

func (f *wordFilter) add(h uint64) {
	for range wordFilterHashes {
		bit := h % wordFilterBits
		f[bit/64] |= 1 << (bit % 64)
		h >>= 21
	}
}

func (f *wordFilter) mayContain(word string) bool {
	h := fnvOffset
	for i := 0; i < len(word); i++ {
		h = (h ^ uint64(wordFold[word[i]])) * fnvPrime
	}
	for range wordFilterHashes {
		bit := h % wordFilterBits
		if f[bit/64]&(1<<(bit%64)) == 0 {
			return false
		}
		h >>= 21
	}
	return true
}
