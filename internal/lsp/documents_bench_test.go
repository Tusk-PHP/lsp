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
	for _, bc := range []struct {
		name   string
		change map[string]any
	}{
		{"incremental edit", map[string]any{"range": map[string]any{"start": map[string]any{"line": 10000, "character": 20}, "end": map[string]any{"line": 10000, "character": 20}}, "text": "x"}},
		{"full text", map[string]any{"text": edited}},
	} {
		b.Run(bc.name, func(b *testing.B) {
			body, err := json.Marshal(map[string]any{
				"jsonrpc": "2.0",
				"method":  "textDocument/didChange",
				"params": map[string]any{
					"textDocument":   map[string]any{"uri": uri, "version": 2},
					"contentChanges": []any{bc.change},
				},
			})
			if err != nil {
				b.Fatal(err)
			}
			s := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
			b.ReportAllocs()
			for b.Loop() {
				s.docs.Open(uri, 1, text)
				var msg jsonRPCMessage
				if err := json.Unmarshal(body, &msg); err != nil {
					b.Fatal(err)
				}
				s.handleMessage(&msg)
			}
		})
	}
}
