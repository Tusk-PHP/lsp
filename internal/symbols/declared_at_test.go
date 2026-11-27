package symbols

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestMemberDeclaredAt(t *testing.T) {
	idx := NewIndex()
	idx.IndexFile("file:///a.php", `<?php
/** @method void magic() */
class Room { const SIZE = 1; public $area; public function measure() { return measure(); } }
enum Cases: string { case One = 'one'; }
`)
	cases := []struct {
		line, char int
		word, want string
	}{
		{2, 20, "SIZE", "Room::SIZE"},
		{2, 37, "area", "Room::$area"},
		{2, 37, "$area", "Room::$area"},
		{2, 61, "measure", "Room::measure"},
		{2, 61, "MEASURE", "Room::measure"},
		{3, 28, "One", "Cases::One"},
		{2, 20, "size", ""},
		{2, 37, "AREA", ""},
		{3, 28, "one", ""},
		{2, 61, "size", ""},
		{2, 6, "Room", ""},
		{2, 80, "measure", ""},
		{3, 6, "Cases", ""},
		{0, 0, "", ""},
	}
	for _, c := range cases {
		got := idx.MemberDeclaredAt("file:///a.php", textpos.Position{Line: c.line, Character: c.char}, c.word)
		switch {
		case c.want == "" && got != nil:
			t.Errorf("%d:%d %q: got %s, want none", c.line, c.char, c.word, got.FQN)
		case c.want != "" && (got == nil || got.FQN != c.want):
			t.Errorf("%d:%d %q: got %v, want %s", c.line, c.char, c.word, got, c.want)
		}
	}
	if got := idx.MemberDeclaredAt("file:///other.php", textpos.Position{Line: 2, Character: 61}, "measure"); got != nil {
		t.Errorf("another file: got %s", got.FQN)
	}
}

func TestDeclarationBeforeIsTheLastDeclarationNameInUnchangedText(t *testing.T) {
	src := "<?php\nnamespace App;\n/**\n * @method void magic()\n */\nclass Room {\n    public int $size = 0;\n    public function area() {\n        return f(1);\n    }\n}\nfunction helper() {}\n"
	idx := NewIndex()
	idx.IndexFile("file:///room.php", src)
	offsetOf := func(s string) uint32 { return uint32(strings.Index(src, s)) }
	at := func(s string) textpos.Position {
		o := strings.Index(src, s)
		return textpos.Position{Line: strings.Count(src[:o], "\n"), Character: o - strings.LastIndex(src[:o], "\n") - 1}
	}
	for _, tc := range []struct {
		name   string
		uri    string
		source string
		pos    textpos.Position
		want   uint32
	}{
		{"in a method, its name", "file:///room.php", src, at("f(1)"), offsetOf("area")},
		{"after the last function", "file:///room.php", src, at("{}\n"), offsetOf("helper")},
		{"on a declaration's own name, the one before it", "file:///room.php", src, at("area"), offsetOf("Room")},
		{"before any declaration", "file:///room.php", src, at("namespace"), 0},
		{"after an edit above the declaration", "file:///room.php", strings.Replace(src, "Room", "Rooms", 1), at("f(1)"), 0},
		{"after an edit below the declaration", "file:///room.php", strings.Replace(src, "f(1)", "f(1, 2)", 1), at("f(1)"), offsetOf("area")},
		{"in a shorter text", "file:///room.php", src[:offsetOf("area")-1], at("f(1)"), 0},
		{"in a file that isn't indexed", "file:///other.php", src, at("f(1)"), 0},
	} {
		got, at := idx.DeclarationBefore(tc.uri, tc.source, tc.pos)
		if got != tc.want {
			t.Errorf("%s: got %d, want %d", tc.name, got, tc.want)
		}
		if line := strings.Count(src[:got], "\n"); at.Line != line || at.Character != int(got)-strings.LastIndexByte(src[:got], '\n')-1 {
			t.Errorf("%s: at %v, want the offset's position", tc.name, at)
		}
	}
}

func TestOffsetInStaysInsideTheText(t *testing.T) {
	for _, pos := range []textpos.Position{{Line: 3}, {Line: 1, Character: 9}} {
		if _, ok := offsetIn("a\nb", pos); ok {
			t.Errorf("%v is outside the text", pos)
		}
	}
}
