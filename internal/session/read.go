package session

import (
	"os"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// readFile returns a file's source from disk by URI, or "" when it can't be
// read.
func readFile(uri string) string {
	data, err := os.ReadFile(symbols.URIToPath(uri))
	if err != nil {
		return ""
	}
	return string(data)
}
