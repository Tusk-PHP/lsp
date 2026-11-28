package session

import (
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// readFile returns a file's source from disk by URI, or "" when it can't be
// read.
func readFile(uri string) string {
	data, err := symbols.ReadURI(uri)
	if err != nil {
		return ""
	}
	return string(data)
}
