package workspace

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestFilesUnderPathsWithSpecialCharactersAreIndexedOnceUnderTheEditorsURI(t *testing.T) {
	for _, dir := range []string{"my app", "issue #42", "100% done", "café ünïcode"} {
		t.Run(dir, func(t *testing.T) {
			root := filepath.Join(t.TempDir(), dir)
			writeFiles(t, root, map[string]string{"app/Order.php": "<?php\nnamespace App;\nclass Order {}\n"})
			ws := New(Options{RootPath: root, Config: config.DefaultConfig()})
			ws.IndexWorkspace()

			path := filepath.Join(root, "app", "Order.php")
			editorURI := symbols.PathToURI(path)
			ws.Index.IndexFile(editorURI, "<?php\nnamespace App;\nclass Order {}\nclass Edited {}\n")

			var uris []string
			for _, uri := range ws.Index.GetAllFileURIs() {
				if strings.HasSuffix(uri, "/app/Order.php") {
					uris = append(uris, uri)
				}
			}
			if len(uris) != 1 || uris[0] != editorURI {
				t.Errorf("the file is indexed once, under the URI an editor sends (%s): %v", editorURI, uris)
			}
			order := ws.Index.Lookup(`App\Order`)
			if order == nil || order.URI != editorURI {
				t.Fatalf("App\\Order is declared at %s: %+v", editorURI, order)
			}
			if _, err := os.Stat(symbols.URIToPath(order.URI)); err != nil {
				t.Errorf("the declaration's URI leads back to the file: %v", err)
			}
		})
	}
}
