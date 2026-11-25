package lsp

import (
	"encoding/json"
	"io"
	"log"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/document"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func benchmarkSource(nonASCII bool) string {
	var b strings.Builder
	b.WriteString("<?php\nnamespace App;\n\nclass Big\n{\n")
	for i := range 2000 {
		label := "'total'"
		if nonASCII && i%2 == 0 {
			label = "'Größe 😀 漢字'"
		}
		b.WriteString("    public function m")
		b.WriteString(strings.Repeat("x", i%7+1))
		b.WriteString("(): string { $v = ")
		b.WriteString(label)
		b.WriteString("; return $v . undefinedFn(); }\n")
	}
	b.WriteString("}\n")
	return b.String()
}

func benchmarkServer(b *testing.B, source string) (*Server, string) {
	b.Helper()
	s := NewServer(strings.NewReader(""), io.Discard, log.New(io.Discard, "", 0))
	root := b.TempDir()
	message := func(method string, params interface{}) *jsonRPCMessage {
		raw, err := json.Marshal(params)
		if err != nil {
			b.Fatal(err)
		}
		id := json.RawMessage("1")
		return &jsonRPCMessage{JSONRPC: "2.0", ID: &id, Method: method, Params: raw}
	}
	s.handleMessage(message("initialize", map[string]interface{}{"rootUri": symbols.PathToURI(root), "capabilities": map[string]interface{}{}}))
	uri := symbols.PathToURI(root + "/Big.php")
	s.docMu.Lock()
	s.documents[uri] = source
	s.docMu.Unlock()
	s.index.IndexFile(uri, source)
	s.index.MarkReady()
	return s, uri
}

func BenchmarkSemanticTokensFull(b *testing.B) {
	for _, bc := range []struct {
		name   string
		source string
	}{
		{"ascii", benchmarkSource(false)},
		{"non-ascii", benchmarkSource(true)},
		{"non-ascii-100KB-line", "<?php $é = 1; " + strings.Repeat("$a = foo('x'); ", 7000) + "\n"},
	} {
		b.Run(bc.name, func(b *testing.B) {
			s, uri := benchmarkServer(b, bc.source)
			raw, _ := json.Marshal(map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
			id := json.RawMessage("2")
			msg := &jsonRPCMessage{JSONRPC: "2.0", ID: &id, Method: "textDocument/semanticTokens/full", Params: raw}
			b.ReportAllocs()
			for b.Loop() {
				s.handleMessage(msg)
			}
		})
	}
}

func BenchmarkPublishDiagnostics(b *testing.B) {
	for _, bc := range []struct {
		name     string
		nonASCII bool
	}{{"ascii", false}, {"non-ascii", true}} {
		b.Run(bc.name, func(b *testing.B) {
			source := benchmarkSource(bc.nonASCII)
			s, uri := benchmarkServer(b, source)
			b.ReportAllocs()
			for b.Loop() {
				s.publishDiagnostics(uri, source)
			}
		})
	}
}

func BenchmarkCompletionsOut(b *testing.B) {
	for _, bc := range []struct {
		name string
		line string
	}{{"ascii", "<?php $x = 1; $shop->to"}, {"non-ascii", "<?php $é = 'Größe'; $shop->to"}} {
		b.Run(bc.name, func(b *testing.B) {
			col := len(bc.line)
			fresh := func() []protocol.CompletionItem {
				items := make([]protocol.CompletionItem, 2000)
				for i := range items {
					items[i] = protocol.CompletionItem{
						Label:    "total",
						TextEdit: &protocol.TextEdit{Range: protocol.Range{Start: protocol.Position{Character: col - 2}, End: protocol.Position{Character: col}}, NewText: "total"},
					}
				}
				return items
			}
			p := &positions{enc: document.UTF16, text: func(string) (string, bool) { return bc.line + "\n", true }, logf: func(string, ...any) {}}
			b.ReportAllocs()
			for b.Loop() {
				b.StopTimer()
				items := fresh()
				p.columns = nil
				b.StartTimer()
				p.completionsOut("file:///a.php", items)
			}
		})
	}
}
