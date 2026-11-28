package mcpserver

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestReadFileReadsLoneCRsAsNewlines(t *testing.T) {
	path := filepath.Join(t.TempDir(), "welcome.blade.php")
	if err := os.WriteFile(path, []byte("<div>\r{{ $name }}\r</div>\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readFile(symbols.PathToURI(path)); got != "<div>\n{{ $name }}\n</div>\r\n" {
		t.Errorf("readFile = %q", got)
	}
	if got := readFile(symbols.PathToURI(filepath.Join(t.TempDir(), "missing.blade.php"))); got != "" {
		t.Errorf("a file that can't be read reads as empty, got %q", got)
	}
}
