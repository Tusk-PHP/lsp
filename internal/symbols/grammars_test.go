package symbols

import (
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestIndexParsesWithTheGrammarsItWasMadeFor(t *testing.T) {
	grammars := phpparser.GrammarsBetween(token.PHP74, token.PHP81)
	idx := NewIndexFor(grammars)
	if got := idx.Grammars(); got != grammars {
		t.Errorf("Grammars() = %s, want %s", got, grammars)
	}
	if got := idx.GrammarOf("file:///not/indexed.php"); got != grammars {
		t.Errorf("a file not indexed yet is read with the project's grammars %s, got %s", grammars, got)
	}
}
