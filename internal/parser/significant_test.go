package parser

import (
	"reflect"
	"testing"
)

func TestSignificantTokensSkipComments(t *testing.T) {
	tokens := Tokenize("<?php /** doc */ $a /* c */ = // line\n 1;")
	values := func(idx ...int) []string {
		out := make([]string, 0, len(idx))
		for _, i := range idx {
			if i < 0 {
				out = append(out, "-")
				continue
			}
			out = append(out, tokens[i].Value)
		}
		return out
	}
	eq := 0
	for i, tok := range tokens {
		if tok.Value == "=" {
			eq = i
		}
	}
	if got := values(NextSignificant(tokens, 1), NextSignificant(tokens, eq+1), PrevSignificant(tokens, eq-1)); !reflect.DeepEqual(got, []string{"$a", "1", "$a"}) {
		t.Errorf("next/prev: %v", got)
	}
	if got := values(NextSignificant(tokens, len(tokens)), PrevSignificant(tokens, -1), NextSignificant(tokens, -5), PrevSignificant(tokens, len(tokens)+3)); !reflect.DeepEqual(got, []string{"-", "-", tokens[0].Value, tokens[len(tokens)-1].Value}) {
		t.Errorf("out of range: %v", got)
	}
	var kept []string
	for _, tok := range SignificantTokens(tokens) {
		if tok.Kind.IsComment() {
			t.Errorf("comment kept: %q", tok.Value)
		}
		kept = append(kept, tok.Value)
	}
	if len(kept) != len(tokens)-3 {
		t.Errorf("kept %v of %d tokens", kept, len(tokens))
	}
}
