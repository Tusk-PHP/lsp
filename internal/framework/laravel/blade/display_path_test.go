package blade

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestADocumentIsShownByItsPathOrElseItsURI(t *testing.T) {
	if got := projectPath(symbols.PathToURI("/p/app/View/Components/Alert.php")); got != "app/View/Components/Alert.php" {
		t.Errorf("a file is shown by its path in the project, got %q", got)
	}
	if got := relativeViewPath("untitled:alert.blade.php"); got != "`untitled:alert.blade.php`" {
		t.Errorf("a document that is not a file is shown by its URI, got %q", got)
	}
}
