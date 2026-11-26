package symbols

import (
	"strings"
	"testing"
)

func TestConstantDocCommentsAreIndexedWithQualifiedTypes(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///room.php", "<?php\nnamespace App;\nuse App\\Units\\Metre;\nclass Room {\n    /** @var list<Metre> */\n    const NAMES = [];\n}\n")
	sym := idx.Lookup(`App\Room::NAMES`)
	if sym == nil || !strings.Contains(sym.DocComment, `App\Units\Metre`) {
		t.Errorf("the constant's doc comment is indexed with its types qualified, got %+v", sym)
	}
}
