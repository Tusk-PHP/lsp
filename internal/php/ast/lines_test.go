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

func TestLinesOffsetStaysOnItsLine(t *testing.T) {
	src := "ab\ncd\n\nef"
	lines := NewLines(src)
	for _, c := range []struct {
		line, col int
		want      uint32
		ok        bool
	}{
		{0, 0, 0, true}, {0, 2, 2, true}, {0, 9, 2, true}, {1, 1, 4, true}, {2, 5, 6, true}, {3, 9, 9, true},
		{4, 0, 0, false}, {-1, 0, 0, false}, {0, -1, 0, false},
	} {
		if got, ok := lines.Offset(c.line, c.col, len(src)); got != c.want || ok != c.ok {
			t.Errorf("%d:%d: %d %v, want %d %v", c.line, c.col, got, ok, c.want, c.ok)
		}
	}
}
