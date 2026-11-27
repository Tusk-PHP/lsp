// Package textpos holds the positions and ranges the analysis engine works
// in. A Position is a zero-based line and a byte column on that line (the
// UTF-8 offset, not the UTF-16 one the LSP speaks): the language server and
// the MCP server convert at their edge. The LSP's protocol.Position and
// protocol.Range are aliases of these types, so the engine and the server
// share values without copying.
package textpos

// Position is a zero-based line and the byte column on it.
type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

// Range runs from Start up to, but not including, End.
type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}
