package lsp

import (
	"path/filepath"
	"sync"
	"testing"
	"time"
)

type stoppedToolRuns struct {
	fileAnalysis
	ran   chan struct{}
	mu    sync.Mutex
	saved []string
}

func (a *stoppedToolRuns) RunTools(uri, filePath string, unchanged func() bool) bool {
	a.ran <- struct{}{}
	return false
}

func (a *stoppedToolRuns) Saved(uri, source string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.saved = append(a.saved, uri)
}

func TestASaveWhoseToolRunWasStoppedAnalyzesNothing(t *testing.T) {
	h, root := startWatchedProject(t, map[string]string{"composer.json": `{"require": {"php": "^8.1"}}`}, map[string]interface{}{}, nil)
	<-h.server.indexed
	analysis := &stoppedToolRuns{fileAnalysis: h.server.analysis, ran: make(chan struct{}, 1)}
	h.server.analysis = analysis
	uri := "file://" + filepath.Join(root, "app/Order.php")
	h.notify("textDocument/didSave", map[string]interface{}{"textDocument": map[string]interface{}{"uri": uri}})
	select {
	case <-analysis.ran:
	case <-time.After(5 * time.Second):
		t.Fatal("a save runs the external tools")
	}
	time.Sleep(200 * time.Millisecond)
	analysis.mu.Lock()
	defer analysis.mu.Unlock()
	if len(analysis.saved) != 0 {
		t.Errorf("a save whose tool run a newer save, a close or new settings stopped leaves the on-save rules to the run that stopped it, got %v", analysis.saved)
	}
}
