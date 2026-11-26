package lsp

import (
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"
)

func BenchmarkDidChangeLargeFile(b *testing.B) {
	line := "        \"value\": \"findOneBy name größe\",\n"
	text := "{\n" + strings.Repeat(line, 20000) + "}\n"
	edited := text[:len(text)/2] + "x" + text[len(text)/2:]
	uri := "file:///big.json"
	insertAt := func(line int) map[string]any {
		pos := map[string]any{"line": line, "character": 20}
		return map[string]any{"range": map[string]any{"start": pos, "end": pos}, "text": "x"}
	}
	cursors := make([]any, 0, 100)
	for line := 20000 - 100; line > 0; line -= 200 {
		cursors = append(cursors, insertAt(line))
	}
	for _, bc := range []struct {
		name    string
		changes []any
	}{
		{"incremental edit", []any{insertAt(10000)}},
		{"100 cursors", cursors},
		{"full text", []any{map[string]any{"text": edited}}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			body, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"method":  "textDocument/didChange",
				"params": map[string]any{
					"textDocument":   map[string]any{"uri": uri, "version": 2},
					"contentChanges": bc.changes,
				},
			})
			if err != nil {
				b.Fatal(err)
			}
			s := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
			b.ReportAllocs()
			for b.Loop() {
				s.openDocument(uri, 1, text)
				var msg jsonRPCMessage
				if err := json.Unmarshal(body, &msg); err != nil {
					b.Fatal(err)
				}
				s.handleMessage(&msg)
			}
		})
	}
}
