package symbols

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
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
		got := idx.MemberDeclaredAt("file:///a.php", protocol.Position{Line: c.line, Character: c.char}, c.word)
		switch {
		case c.want == "" && got != nil:
			t.Errorf("%d:%d %q: got %s, want none", c.line, c.char, c.word, got.FQN)
		case c.want != "" && (got == nil || got.FQN != c.want):
			t.Errorf("%d:%d %q: got %v, want %s", c.line, c.char, c.word, got, c.want)
		}
	}
	if got := idx.MemberDeclaredAt("file:///other.php", protocol.Position{Line: 2, Character: 61}, "measure"); got != nil {
		t.Errorf("another file: got %s", got.FQN)
	}
}
