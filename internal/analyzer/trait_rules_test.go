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

func TestDeclarationFollowsTraitInsteadofAndAs(t *testing.T) {
	traits := "<?php\nnamespace App;\ntrait A { public function talk(): void {} }\ntrait B { public function talk(): void {} public function whisper(): void {} }\ntrait Nested { use B; }\n"
	classes := `<?php
namespace App;
class Talker { use B, A { A::talk insteadof B; } }
class NestedAlias { use Nested { Nested::whisper as murmur; } }
class Child extends NestedAlias { public function murmur(): void {} }
`
	use := "<?php\nnamespace App;\n(new Talker())->talk();\n(new Child())->murmur();\n"
	a, _ := setupRenameAnalyzer(map[string]string{"file:///traits.php": traits, "file:///classes.php": classes, "file:///use.php": use})
	cases := []struct{ needle, why string }{
		{"talk", "the trait insteadof keeps, not the one it leaves out"},
		{"murmur", "the nested trait's method a parent class aliases"},
	}
	for _, c := range cases {
		got := a.FindDeclaration("file:///use.php", use, positionOf(t, use, c.needle))
		want := map[string]int{"talk": 2, "murmur": 3}[c.needle]
		if got == nil || got.URI != "file:///traits.php" || got.Range.Start.Line != want {
			t.Errorf("declaration of %s should open %s on line %d: got %v", c.needle, c.why, want, got)
		}
	}
}

func TestTraitAliasIsNotRenamedAndItsReferencesAreItsOwn(t *testing.T) {
	traits := "<?php\nnamespace App;\ntrait B {\n    public function talk(): void {}\n}\n"
	talker := "<?php\nnamespace App;\nclass Talker { use B { talk as bTalk; } }\n"
	use := "<?php\nnamespace App;\n$t = new Talker();\n$t->bTalk();\n$t->talk();\n"
	a, read := setupRenameAnalyzer(map[string]string{"file:///B.php": traits, "file:///Talker.php": talker, "file:///use.php": use})
	pos := positionOf(t, use, "bTalk")

	if got := a.PrepareRename("file:///use.php", use, pos); got != nil {
		t.Errorf("prepare rename on an alias: got %+v, want refused", got)
	}
	if got := a.Rename("file:///use.php", use, pos, "shout", read); got != nil {
		t.Errorf("rename on an alias: got %+v, want refused", got.Changes)
	}

	want := map[string]int{"file:///Talker.php": 2, "file:///use.php": 3}
	refs := a.FindAllReferences("file:///use.php", use, pos, read)
	if len(refs) != len(want) {
		t.Fatalf("references of bTalk = %v, want the alias rule and the call", refs)
	}
	for _, ref := range refs {
		if line, ok := want[ref.URI]; !ok || ref.Range.Start.Line != line {
			t.Errorf("reference of bTalk at %s:%d, want only the alias rule and the call", ref.URI, ref.Range.Start.Line)
		}
	}
}
