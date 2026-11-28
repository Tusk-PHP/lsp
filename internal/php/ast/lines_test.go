package ast

import "testing"

func TestLinesPosition(t *testing.T) {
	lines := NewLines("ab\ncd\n\nef")
	for offset, want := range map[uint32][2]int{0: {0, 0}, 2: {0, 2}, 3: {1, 0}, 5: {1, 2}, 6: {2, 0}, 7: {3, 0}, 9: {3, 2}} {
		if line, col := lines.Position(offset); [2]int{line, col} != want {
			t.Errorf("%d: %d:%d, want %v", offset, line, col, want)
		}
	}
	if got := NewLines(""); len(got) != 1 {
		t.Errorf("empty source has one line, got %v", got)
	}
}
