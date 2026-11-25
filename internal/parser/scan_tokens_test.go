package parser

import (
	"slices"
	"testing"
)

func TestScanTokensVisitsTheTokensTokenizeReturns(t *testing.T) {
	src := "<?php\n/** @var Foo */\n$a = \"x\"; // c\n?>html<?php #[Attr] fn() => $a::B->c |> d;\n"
	want, _ := tokenize(src)
	var got []Token
	ScanTokens(src, func(tok Token) bool {
		got = append(got, tok)
		return true
	})
	if !slices.Equal(got, want) {
		t.Errorf("ScanTokens visited\n%v\nwant the tokens tokenize returns\n%v", got, want)
	}
}

func TestScanTokensStopsWhenVisitSaysSo(t *testing.T) {
	src := "<?php\n/** d */ /* c */ // l\n\"s\" $v 1 name ?> <?php #[A] <<<EOT\nx\nEOT;\n== -> => :: |> {\n"
	all, _ := tokenize(src)
	for stop := 1; stop <= len(all); stop++ {
		var got []Token
		ScanTokens(src, func(tok Token) bool {
			got = append(got, tok)
			return len(got) < stop
		})
		if !slices.Equal(got, all[:stop]) {
			t.Fatalf("visit asked to stop at token %d, got %v, want %v", stop, got, all[:stop])
		}
	}
}
