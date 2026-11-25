package parser

import "testing"

func TestMatchingCloseAndOpenPairEachBracketKind(t *testing.T) {
	tokens := Tokenize("<?php f([1, (2)], function () { return [$a{0}]; }); g(")
	index := func(value string, nth int) int {
		seen := 0
		for i, tok := range tokens {
			if tok.Value != value {
				continue
			}
			if seen == nth {
				return i
			}
			seen++
		}
		t.Fatalf("no %q number %d", value, nth)
		return -1
	}
	pairs := []struct {
		open, close string
		openNth     int
		closeNth    int
	}{
		{"(", ")", 0, 2},
		{"(", ")", 1, 0},
		{"(", ")", 2, 1},
		{"[", "]", 0, 0},
		{"[", "]", 1, 1},
		{"{", "}", 0, 1},
		{"{", "}", 1, 0},
	}
	for _, p := range pairs {
		open, close := index(p.open, p.openNth), index(p.close, p.closeNth)
		if got := MatchingClose(tokens, open); got != close {
			t.Errorf("MatchingClose(%s #%d) = %d, want %d", p.open, p.openNth, got, close)
		}
		if got := MatchingOpen(tokens, close); got != open {
			t.Errorf("MatchingOpen(%s #%d) = %d, want %d", p.close, p.closeNth, got, open)
		}
	}

	unclosed := index("(", 3)
	if got := MatchingClose(tokens, unclosed); got != -1 {
		t.Errorf("an unclosed ( has no match, got %d", got)
	}
	if got := MatchingClose(tokens, index("f", 0)); got != -1 {
		t.Errorf("a token that opens nothing has no match, got %d", got)
	}
	if got := MatchingOpen(tokens, index(";", 0)); got != -1 {
		t.Errorf("a token that closes nothing has no match, got %d", got)
	}
	for _, out := range []int{-1, len(tokens)} {
		if MatchingClose(tokens, out) != -1 || MatchingOpen(tokens, out) != -1 {
			t.Errorf("index %d is out of range and has no match", out)
		}
	}
}
