package lsp

import (
	"encoding/json"
	"testing"
)

func BenchmarkSelectionRange(b *testing.B) {
	for _, bc := range []struct {
		name   string
		source string
	}{
		{"ascii", benchmarkSource(false)},
		{"non-ascii", benchmarkSource(true)},
	} {
		b.Run(bc.name, func(b *testing.B) {
			s, uri := benchmarkServer(b, bc.source)
			raw, _ := json.Marshal(map[string]interface{}{
				"textDocument": map[string]interface{}{"uri": uri},
				"positions":    []interface{}{map[string]interface{}{"line": 1000, "character": 40}},
			})
			id := json.RawMessage("2")
			msg := &jsonRPCMessage{JSONRPC: "2.0", ID: &id, Method: "textDocument/selectionRange", Params: raw}
			b.ReportAllocs()
			for b.Loop() {
				s.handleMessage(msg)
			}
		})
	}
}
