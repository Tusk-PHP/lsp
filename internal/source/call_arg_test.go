package sourcectx

import (
	"testing"

	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func analyzeMarked(t *testing.T, marked string) *CursorContext {
	t.Helper()
	source, pos := cursorIn("<?php\n" + marked)
	ctx := Analyze("file:///a.php", source, pos, phpparser.OnlyGrammar(token.Latest))
	if ctx == nil {
		t.Fatalf("no context for %q", marked)
	}
	return ctx
}

// The cursor context reads its call argument, string offset and name slot
// from the file's tree at the cursor.
func TestCursorContextSlots(t *testing.T) {
	if arg, ok := analyzeMarked(t, "f('a|');").CallArg(); !ok || arg.Function != "f" || arg.Partial != "a" {
		t.Errorf("CallArg = %#v, %v", arg, ok)
	}
	if dim, ok := analyzeMarked(t, "$a['k|").StringDim(); !ok || dim.Partial != "k" {
		t.Errorf("StringDim = %#v, %v", dim, ok)
	}
	if slot, ok := analyzeMarked(t, "$a = new Fo|").NameSlot(); !ok || slot.Partial != "Fo" {
		t.Errorf("NameSlot = %#v, %v", slot, ok)
	}
}
