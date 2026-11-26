package workspace

import (
	"io"
	"log"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestAnIDEHelperFileWithLoneCRsIsIndexedOnItsLines(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"_ide_helper.php": "<?php\rnamespace App;\rclass Helped {}\r"})
	ws := New(Options{RootPath: root, Framework: "none", Config: config.DefaultConfig(), Logger: log.New(io.Discard, "", 0), BuiltinProfile: symbols.BuiltinProfile{PHPVersion: "8.3"}})
	ws.IndexWorkspace()
	sym := ws.Index.Lookup(`App\Helped`)
	if sym == nil || sym.Range.Start.Line != 2 {
		t.Fatalf("the IDE helper's class is declared on its third line: %+v", sym)
	}
	if src := ws.Index.GetFileSource("file://" + filepath.Join(root, "_ide_helper.php")); src != "<?php\nnamespace App;\nclass Helped {}\n" {
		t.Errorf("the IDE helper's text is kept with its lone CRs read as newlines: %q", src)
	}
}
