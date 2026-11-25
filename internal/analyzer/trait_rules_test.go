package analyzer

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestDefinitionFollowsTraitInsteadofAndAs(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFile("file:///A.php", "<?php\nnamespace App;\ntrait A {\n    public function talk(): void {}\n}\n")
	idx.IndexFile("file:///B.php", "<?php\nnamespace App;\ntrait B {\n    public function talk(): void {}\n}\n")
	idx.IndexFile("file:///Talker.php", "<?php\nnamespace App;\nclass Talker { use B, A { A::talk insteadof B; B::talk as bTalk; } }\n")
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"))

	source := "<?php\nnamespace App;\n$t = new Talker();\n$t->talk();\n$t->bTalk();\n"
	cases := []struct{ word, hint, wantURI string }{
		{"talk", "$t->talk", "file:///A.php"},
		{"bTalk", "$t->bTalk", "file:///B.php"},
	}
	for _, c := range cases {
		loc := a.FindDefinition("file:///use.php", source, charPosOf(t, source, c.word, c.hint))
		if loc == nil {
			t.Errorf("%s: no definition", c.word)
			continue
		}
		if loc.URI != c.wantURI || loc.Range.Start.Line != 3 {
			t.Errorf("%s: got %s:%d, want %s:3", c.word, loc.URI, loc.Range.Start.Line, c.wantURI)
		}
	}
}
