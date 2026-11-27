package ast

import (
	"sort"
	"strings"
)

type Lines []uint32

func NewLines(src string) Lines {
	starts := make(Lines, 1, len(src)/32+1)
	for i := strings.IndexByte(src, '\n'); i >= 0; {
		starts = append(starts, uint32(i+1))
		next := strings.IndexByte(src[i+1:], '\n')
		if next < 0 {
			break
		}
		i += next + 1
	}
	return starts
}

func (l Lines) Position(offset uint32) (line, col int) {
	line = sort.Search(len(l), func(i int) bool { return l[i] > offset }) - 1
	return line, int(offset - l[line])
}
