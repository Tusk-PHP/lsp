package workspace

import (
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestIndexWorkspaceMaxIndexFiles(t *testing.T) {
	root := t.TempDir()
	for i := 0; i < 5; i++ {
		src := fmt.Sprintf("<?php\nclass C%d {}\n", i)
		if err := os.WriteFile(filepath.Join(root, fmt.Sprintf("c%d.php", i)), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	index := func(limit int) int {
		cfg := config.DefaultConfig()
		cfg.MaxIndexFiles = limit
		ws := New(Options{RootPath: root, Framework: "none", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
		return ws.IndexWorkspace()
	}
	if got := index(0); got != 5 {
		t.Errorf("maxIndexFiles 0 should index every file, indexed %d of 5", got)
	}
	if got := index(2); got != 2 {
		t.Errorf("maxIndexFiles 2 should stop at 2, indexed %d", got)
	}
}
