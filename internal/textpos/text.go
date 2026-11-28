package textpos

import (
	"os"
	"strings"
	"unsafe"
)

func hasLoneCR(text string) bool {
	for i := 0; ; {
		j := strings.IndexByte(text[i:], '\r')
		if j < 0 {
			return false
		}
		i += j + 1
		if i == len(text) || text[i] != '\n' {
			return true
		}
	}
}

// CoreText returns text with every lone \r turned into \n.
func CoreText(text string) string {
	if !hasLoneCR(text) {
		return text
	}
	b := []byte(text)
	for i, c := range b {
		if c == '\r' && (i+1 == len(b) || b[i+1] != '\n') {
			b[i] = '\n'
		}
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// ReadText reads the file at path as core text.
func ReadText(path string) (string, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return CoreText(string(content)), nil
}
