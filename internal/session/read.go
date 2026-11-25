package session

import (
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// readFile returns a file's source from disk by URI, or "" when it can't be
// read.
func readFile(uri string) string {
	data, err := textpos.ReadText(symbols.URIToPath(uri))
	if err != nil {
		return ""
	}
	return data
}
