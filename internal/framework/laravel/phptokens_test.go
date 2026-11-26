package laravel

import (
	"slices"
	"testing"
)

func TestPHPTokensKeepEachStringWhole(t *testing.T) {
	src := "<?php\n$a = \"x {$b[\"k$c\"]} `y`\"; `ls $d`; ?>html\n<?php $e = \"open {$f"
	var got []string
	for _, tok := range phpTokens(src) {
		got = append(got, tok.Value)
	}
	want := []string{"<?php\n", "$a", "=", "\"x {$b[\"k$c\"]} `y`\"", ";", "`ls $d`", ";", "?>", "<?php ", "$e", "=", "\"open {$f"}
	if !slices.Equal(got, want) {
		t.Errorf("got %q\nwant %q", got, want)
	}
	toks := phpTokens(src)
	if toks[5].Line != 1 || toks[5].Column != 26 || toks[7].Kind != toks[4].Kind {
		t.Errorf("positions or close tag: %+v %+v", toks[5], toks[7])
	}
}

func TestMatchingClose(t *testing.T) {
	toks := phpTokens("<?php f([#[A] 1, (2)]) {")
	for open, want := range map[int]int{2: 13, 3: 12, 9: 11, 1: -1, 14: -1, 20: -1} {
		if got := matchingClose(toks, open); got != want {
			t.Errorf("matchingClose(%d) = %d, want %d", open, got, want)
		}
	}
}
