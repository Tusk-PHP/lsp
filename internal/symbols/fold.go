package symbols

func FoldKey(buf *[64]byte, s string) []byte {
	out := buf[:0]
	if len(s) > len(buf) {
		out = make([]byte, 0, len(s))
	}
	for i := 0; i < len(s); i++ {
		out = append(out, lowerASCII(s[i]))
	}
	return out
}

func FoldName(s string) string {
	for i := 0; i < len(s); i++ {
		if 'A' <= s[i] && s[i] <= 'Z' {
			var buf [64]byte
			return string(FoldKey(&buf, s))
		}
	}
	return s
}

func EqualFold(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := 0; i < len(a); i++ {
		if lowerASCII(a[i]) != lowerASCII(b[i]) {
			return false
		}
	}
	return true
}

func lowerASCII(c byte) byte {
	if 'A' <= c && c <= 'Z' {
		return c + 'a' - 'A'
	}
	return c
}
