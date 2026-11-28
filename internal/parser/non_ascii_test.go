package parser

import "testing"

func TestTokenizeKeepsNonASCIINamesWhole(t *testing.T) {
	tokens := tokensOf("<?php\nclass Ätna { function größe($préféré) { return PRIX_€; } }\n")
	want := map[string]TokenKind{"Ätna": TokenIdentifier, "größe": TokenIdentifier, "$préféré": TokenVariable, "PRIX_€": TokenIdentifier}
	for _, tok := range tokens {
		if kind, ok := want[tok.Value]; ok && tok.Kind == kind {
			delete(want, tok.Value)
		}
	}
	if len(want) != 0 {
		t.Errorf("not tokenized as whole names: %v", want)
	}
}
