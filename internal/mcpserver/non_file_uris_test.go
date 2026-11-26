package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestReferencesToASymbolThatIsNotDeclaredInAFileAreAnError(t *testing.T) {
	svc, err := New(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	svc.runtime().Workspace.Index.IndexFile("untitled:Scratch.php", "<?php\nnamespace App;\nclass Scratch {}\n")
	if _, err := svc.findReferences(`App\Scratch`); err == nil || !strings.Contains(err.Error(), "not declared in a file") {
		t.Errorf("got %v", err)
	}
}

func TestReadFileReadsOnlyFiles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "Order.php")
	if err := os.WriteFile(path, []byte("<?php\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(symbols.PathToURI(path)); got != "<?php\n" {
		t.Errorf("got %q", got)
	}
	if got := readFile("untitled:Order.php"); got != "" {
		t.Errorf("a document that is not a file is not read, got %q", got)
	}
}
