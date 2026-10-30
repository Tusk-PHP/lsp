package lsp

import (
	"bytes"
	"errors"
	"io"
	"log"
	"strings"
	"testing"
)

type failingReader struct{ reads int }

func (r *failingReader) Read([]byte) (int, error) {
	r.reads++
	return 0, errors.New("broken stream")
}

func TestRunStopsOnPersistentReadErrors(t *testing.T) {
	var logs bytes.Buffer
	reader := &failingReader{}
	err := NewServer(reader, io.Discard, log.New(&logs, "", 0)).Run()
	if err == nil || !strings.Contains(err.Error(), "broken stream") {
		t.Fatalf("want an error wrapping the read failure, got %v", err)
	}
	if lines := strings.Count(logs.String(), "Read error"); lines >= maxConsecutiveReadErrors {
		t.Errorf("logged %d read errors, want fewer than %d", lines, maxConsecutiveReadErrors)
	}
}
