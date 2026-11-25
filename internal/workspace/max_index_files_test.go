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
	index := func(limit int) IndexedFiles {
		cfg := config.DefaultConfig()
		cfg.MaxIndexFiles = limit
		ws := New(Options{RootPath: root, Framework: "none", Config: cfg, Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
		return ws.IndexWorkspace()
	}
	cases := []struct {
		limit int
		want  IndexedFiles
	}{
		{0, IndexedFiles{Count: 5}},
		{2, IndexedFiles{Count: 2, Truncated: true}},
		{5, IndexedFiles{Count: 5}},
		{6, IndexedFiles{Count: 5}},
	}
	for _, c := range cases {
		if got := index(c.limit); got != c.want {
			t.Errorf("maxIndexFiles %d over 5 files: got %+v, want %+v", c.limit, got, c.want)
		}
	}
}
