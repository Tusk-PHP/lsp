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
		want       string
	}{
		{2, 20, "Room::SIZE"},
		{2, 37, "Room::$area"},
		{2, 61, "Room::measure"},
		{3, 28, "Cases::One"},
		{2, 6, ""},
		{2, 80, ""},
		{3, 6, ""},
		{0, 0, ""},
	}
	for _, c := range cases {
		got := idx.MemberDeclaredAt("file:///a.php", protocol.Position{Line: c.line, Character: c.char})
		switch {
		case c.want == "" && got != nil:
			t.Errorf("%d:%d: got %s, want none", c.line, c.char, got.FQN)
		case c.want != "" && (got == nil || got.FQN != c.want):
			t.Errorf("%d:%d: got %v, want %s", c.line, c.char, got, c.want)
		}
	}
	if got := idx.MemberDeclaredAt("file:///other.php", protocol.Position{Line: 2, Character: 61}); got != nil {
		t.Errorf("another file: got %s", got.FQN)
	}
}
