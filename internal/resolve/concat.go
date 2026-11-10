package resolve

// isConcatenation reports whether expr is a string concatenation at its top
// level, as `$dir . '/' . $name` is: a `.` outside strings, comments,
// parentheses and brackets that isn't part of `...`, `.=` or a number. An
// operator that binds looser than `.` and gives something else (a
// comparison, `??`, `?:`, `and`, `or`, `instanceof` and the like) at the top
// level makes it not one, as do a heredoc or a closure, which aren't read.
func isConcatenation(expr string) bool {
	found := false
	depth := 0
	for i := 0; i < len(expr); i++ {
		c := expr[i]
		switch c {
		case '\'', '"', '`':
			end := closingQuote(expr, i)
			if end < 0 {
				return false
			}
			i = end
			continue
		case '#':
			return false
		case '/':
			if i+1 < len(expr) && (expr[i+1] == '/' || expr[i+1] == '*') {
				return false
			}
		case '(', '[', '{':
			depth++
			continue
		case ')', ']', '}':
			depth--
			if depth < 0 {
				return false
			}
			continue
		}
		if depth > 0 {
			continue
		}
		switch c {
		case '.':
			next := byte(0)
			if i+1 < len(expr) {
				next = expr[i+1]
			}
			switch {
			case next == '.' || i > 0 && expr[i-1] == '.':
				return false // `...`: spread or variadic
			case next == '=':
				return false // `.=`
			case next >= '0' && next <= '9':
				continue // `.5`, `1.5`: a number
			case i > 0 && expr[i-1] >= '0' && expr[i-1] <= '9' && numberEndsAt(expr, i):
				continue // `1.`
			}
			found = true
		case '?', '<', '>', '=', '!', '&', '|', '^':
			if c == '>' && i > 0 && (expr[i-1] == '-' || expr[i-1] == '=') {
				continue // `->`, `=>`
			}
			if c == '?' && i+1 < len(expr) && expr[i+1] == '-' {
				continue // `?->`
			}
			if c == '!' && (i+1 >= len(expr) || expr[i+1] != '=') {
				continue // `!$x` binds tighter than `.`
			}
			if c == '<' && i+2 < len(expr) && expr[i+1] == '<' && expr[i+2] == '<' {
				return false // heredoc
			}
			if (c == '<' || c == '>') && i+1 < len(expr) && expr[i+1] == c {
				i++
				continue // `<<`, `>>` bind tighter than `.`
			}
			return false
		default:
			if isIdentByte(c) && (i == 0 || !isIdentByte(expr[i-1]) && expr[i-1] != '$' && expr[i-1] != '\\') {
				j := i
				for j < len(expr) && (isIdentByte(expr[j]) || expr[j] >= '0' && expr[j] <= '9') {
					j++
				}
				switch lowerWord(expr[i:j]) {
				case "and", "or", "xor", "instanceof", "fn", "function", "yield", "throw", "print", "match", "include", "include_once", "require", "require_once":
					return false
				}
				i = j - 1
			}
		}
	}
	return found
}

// closingQuote returns the index of the quote that closes the one at start.
func closingQuote(expr string, start int) int {
	q := expr[start]
	for i := start + 1; i < len(expr); i++ {
		switch expr[i] {
		case '\\':
			i++
		case q:
			return i
		}
	}
	return -1
}

// numberEndsAt reports whether the digits before i are a number, not the end
// of a name like `$a1`.
func numberEndsAt(expr string, i int) bool {
	j := i
	for j > 0 && expr[j-1] >= '0' && expr[j-1] <= '9' {
		j--
	}
	return j == 0 || !(isIdentByte(expr[j-1]) || expr[j-1] == '$')
}

func lowerWord(s string) string {
	b := []byte(s)
	for i, c := range b {
		if c >= 'A' && c <= 'Z' {
			b[i] = c + 'a' - 'A'
		}
	}
	return string(b)
}
