package lsp

import (
	"bytes"
	"encoding/json"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Tusk-PHP/lsp/internal/session"
)

type publishSignal chan struct{}

func (p publishSignal) Write(frame []byte) (int, error) {
	if bytes.Contains(frame, []byte("textDocument/publishDiagnostics")) {
		p <- struct{}{}
	}
	return len(frame), nil
}

func BenchmarkDidOpenLargeFile(b *testing.B) {
	var src strings.Builder
	src.WriteString("<?php\n\nnamespace App;\n\nclass Big\n{\n")
	for i := range 900 {
		n := strconv.Itoa(i)
		src.WriteString("    public function m" + n + "(Foo $foo): int\n    {\n        $x = $foo->bar(" + n + ");\n        return strlen((string) $x);\n    }\n\n")
	}
	src.WriteString("}\n")
	uri := "file:///big.php"
	published := make(publishSignal, 1)
	s := newServerWithWorkspace(b, published)
	s.analysis = session.New(s.workspace, session.Options{})
	open := didOpenMessage(b, uri, 1, src.String())
	closeParams, err := json.Marshal(map[string]any{"textDocument": map[string]any{"uri": uri}})
	if err != nil {
		b.Fatal(err)
	}
	closeMsg := &jsonRPCMessage{JSONRPC: "2.0", Method: "textDocument/didClose", Params: closeParams}
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		s.handleMessage(open)
		b.StopTimer()
		<-published
		s.handleMessage(closeMsg)
		for !s.docIndexIdle(uri) {
			time.Sleep(time.Millisecond)
		}
		b.StartTimer()
	}
}
