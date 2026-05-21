package analyzer

func leadingWhitespace(line string) string {
	idx := 0
	for idx < len(line) {
		if line[idx] != ' ' && line[idx] != '\t' {
			break
		}
		idx++
	}
	return line[:idx]
}
