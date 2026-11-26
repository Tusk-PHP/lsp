package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// TestClientStopsAServerThatStopsReading starts a "server" that never reads
// its input. Once the pipe is full a write would block forever; the client
// must give up, stop the process and fail every later request at once.
func TestClientStopsAServerThatStopsReading(t *testing.T) {
	old := writeTimeout
	writeTimeout = 200 * time.Millisecond
	t.Cleanup(func() { writeTimeout = old })

	c, err := StartClient([]string{"sleep", "60"}, t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(c.Shutdown)

	big := strings.Repeat("x", 32<<10)
	start := time.Now()
	var sendErr error
	for range 16 { // 512 KB: well past any pipe buffer
		if sendErr = c.Notify("textDocument/didOpen", map[string]string{"text": big}); sendErr != nil {
			break
		}
	}
	if sendErr == nil {
		t.Fatal("expected a write to a server that never reads to fail")
	}
	if took := time.Since(start); took > 5*time.Second {
		t.Fatalf("giving up took %s", took)
	}

	select {
	case <-c.exited:
	case <-time.After(5 * time.Second):
		t.Fatal("expected the stalled server to be stopped")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := c.Call(ctx, "textDocument/hover", nil); err == nil || ctx.Err() != nil {
		t.Fatalf("a later request should fail at once, got err=%v ctx=%v", err, ctx.Err())
	}
}
