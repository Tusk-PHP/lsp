package lsp

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestWordRangeCoversNonASCIINames(t *testing.T) {
	pr := parser.New().Parse("<?php\necho PRIX_€ + $größe;\n")
	for _, c := range []struct {
		at, start, end int
	}{{6, 5, 13}, {17, 16, 24}} {
		rng := findWordRange(pr, protocol.Position{Line: 1, Character: c.at})
		if rng == nil || rng.Start.Character != c.start || rng.End.Character != c.end {
			t.Errorf("word at %d: got %+v, want %d-%d", c.at, rng, c.start, c.end)
		}
	}
}

func TestWordRangeInTextFollowsLetters(t *testing.T) {
	src := "<?php\n// prix€total 😀emoji naïve—word\n/** prix€total naïve—word */\n$s = 'prix€total naïve—word';\n$h = <<<TXT\nprix€total naïve—word\nTXT;\necho prix€total;\n"
	pr := parser.New().Parse(src)
	lines := strings.Split(src, "\n")
	for _, c := range []struct {
		line          int
		cursor, label string
	}{
		{1, "total", "total"},
		{1, "word", "word"},
		{1, "emoji", "emoji"},
		{1, "ïve", "naïve"},
		{2, "total", "total"},
		{2, "word", "word"},
		{3, "total", "total"},
		{3, "word", "word"},
		{5, "total", "total"},
		{5, "word", "word"},
		{7, "total", "prix€total"},
	} {
		at := strings.Index(lines[c.line], c.cursor)
		start := strings.Index(lines[c.line], c.label)
		want := protocol.Range{
			Start: protocol.Position{Line: c.line, Character: start},
			End:   protocol.Position{Line: c.line, Character: start + len(c.label)},
		}
		for _, ch := range []int{at, at + len(c.cursor)} {
			rng := findWordRange(pr, protocol.Position{Line: c.line, Character: ch})
			if rng == nil || *rng != want {
				t.Errorf("line %d at %d (%q): got %+v, want %q %+v", c.line, ch, c.cursor, rng, c.label, want)
			}
		}
	}
}

func TestWordRangeOutsideTokensIsEmpty(t *testing.T) {
	pr := parser.New().Parse("<b>\n<?php\n$a  =  'x'  ;\n")
	for _, pos := range []protocol.Position{{Line: 0, Character: 0}, {Line: 2, Character: 3}, {Line: 2, Character: 11}} {
		rng := findWordRange(pr, pos)
		if rng == nil || rng.Start != pos || rng.End != pos {
			t.Errorf("at %+v: got %+v, want an empty range", pos, rng)
		}
	}
}
