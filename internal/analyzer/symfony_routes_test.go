package analyzer

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func setupSymfonyRouteAnalyzer(t *testing.T) (*Analyzer, string) {
	t.Helper()

	root := filepath.Join("..", "..", "testdata", "symfony")
	sourcePath := filepath.Join(root, "src", "Controller", "ProductController.php")
	sourceBytes, err := os.ReadFile(sourcePath)
	if err != nil {
		t.Fatalf("failed to read Symfony controller: %v", err)
	}
	source := string(sourceBytes)

	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFileWithSource("file:///src/Controller/ProductController.php", source, symbols.SourceProject)

	ca := container.NewContainerAnalyzer(idx, root, "symfony")
	return NewAnalyzer(idx, ca), source
}

func TestDefinitionSymfonyRouteName(t *testing.T) {
	a, src := setupSymfonyRouteAnalyzer(t)

	var pos protocol.Position
	found := false
	for i, line := range strings.Split(src, "\n") {
		col := strings.Index(line, "product_show")
		if col >= 0 && strings.Contains(line, "generateUrl") {
			pos = protocol.Position{Line: i, Character: col + 3}
			found = true
			break
		}
	}
	if !found {
		t.Fatal("failed to locate product_show route usage")
	}

	loc := a.FindDefinition("file:///src/Controller/ProductController.php", src, pos)
	if loc == nil {
		t.Fatal("expected route definition location")
	}
	if loc.URI != "file:///src/Controller/ProductController.php" {
		t.Fatalf("URI = %q, want controller URI", loc.URI)
	}

	lines := strings.Split(src, "\n")
	defLine := lines[loc.Range.Start.Line]
	if !strings.Contains(defLine, "name: 'show'") {
		t.Fatalf("definition line = %q, want method route attribute", defLine)
	}
}
