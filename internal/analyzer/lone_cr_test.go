package analyzer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestReferencesInAViewOnDiskCountLoneCRsAsLineBreaks(t *testing.T) {
	user := "<?php\nnamespace App\\Models;\nclass User {}\n"
	sources := map[string]string{"file:///user.php": user}
	a, reader := setupRenameAnalyzer(sources)
	view := filepath.Join(t.TempDir(), "profile.blade.php")
	if err := os.WriteFile(view, []byte("<div>\r<p>profile</p>\r{{ \\App\\Models\\User::class }}\r</div>\r"), 0o644); err != nil {
		t.Fatal(err)
	}
	viewURI := symbols.PathToURI(view)
	a.index.AddTemplateFile(viewURI)

	locs := a.FindAllReferences("file:///user.php", user, protocol.Position{Line: 2, Character: 7}, reader)
	for _, loc := range locs {
		if loc.URI != viewURI {
			continue
		}
		if loc.Range.Start.Line != 2 || loc.Range.Start.Character != 15 {
			t.Errorf("the reference is on the view's third line, after the namespace: %+v", loc.Range)
		}
		return
	}
	t.Fatalf("the view read from disk is searched: %+v", locs)
}
