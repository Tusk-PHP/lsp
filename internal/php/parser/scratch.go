package parser

import "slices"

type scratch[T any] struct {
	buf []T
}

func (s *scratch[T]) mark() int {
	return len(s.buf)
}

func (s *scratch[T]) push(v T) {
	s.buf = append(s.buf, v)
}

func (s *scratch[T]) take(from int) []T {
	if len(s.buf) == from {
		return nil
	}
	out := slices.Clone(s.buf[from:])
	clear(s.buf[from:])
	s.buf = s.buf[:from]
	return out
}
