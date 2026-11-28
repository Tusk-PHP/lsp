package parser

import "testing"

func TestUnterminatedBlockCommentRunsToEndOfInput(t *testing.T) {
	tests := []struct {
		name    string
		source  string
		comment string
		kind    TokenKind
	}{
		{"block comment", "1 /* a,", "/* a,", TokenComment},
		{"doc comment", "1 /** a,", "/** a,", TokenDocComment},
		{"block comment with one byte", "1 /*,", "/*,", TokenComment},
		{"doc opener with one byte", "1 /**,", "/**,", TokenComment},
		{"block comment ending in a star", "1 /* a *", "/* a *", TokenComment},
		{"empty block comment", "1 /*", "/*", TokenComment},
		{"doc opener alone", "1 /**", "/**", TokenComment},
		{"empty doc comment", "1 /** ", "/** ", TokenDocComment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tokens, errs := tokenizeFrom(tt.source, false)
			if len(tokens) != 3 || tokens[2].Kind != TokenEOF {
				t.Fatalf("tokens = %#v, want the number, the comment and the end of input", tokens)
			}
			if got := tokens[1]; got.Kind != tt.kind || got.Value != tt.comment {
				t.Fatalf("last token = %#v, want %q", got, tt.comment)
			}
			if len(errs) != 1 {
				t.Fatalf("errors = %#v, want one unterminated comment", errs)
			}
		})
	}
}

func TestBlockCommentAdvancesPosition(t *testing.T) {
	tokens, _ := tokenizeFrom("/* a\nb */ $x", false)
	if len(tokens) != 3 || tokens[1].Value != "$x" || tokens[1].Line != 1 || tokens[1].Column != 5 {
		t.Fatalf("tokens = %#v, want $x at 1:5", tokens)
	}
}

func TestDocParamDefaultInUnterminatedComment(t *testing.T) {
	code, rest := docParamCode("1 /* a,")
	if code != "1 /* a," || rest != "" {
		t.Fatalf("docParamCode = %q, %q, want the whole default and nothing after it", code, rest)
	}
}

func TestBlockCommentKindsMatchPHP(t *testing.T) {
	tests := []struct {
		source     string
		values     []string
		docComment []bool
		errors     int
	}{
		{"/**/", []string{"/**/"}, []bool{false}, 0},
		{"/***/", []string{"/***/"}, []bool{false}, 0},
		{"/****/", []string{"/****/"}, []bool{false}, 0},
		{"/** */", []string{"/** */"}, []bool{true}, 0},
		{"/**\t*/", []string{"/**\t*/"}, []bool{true}, 0},
		{"/**\r*/", []string{"/**\r*/"}, []bool{true}, 0},
		{"/**\n*/", []string{"/**\n*/"}, []bool{true}, 0},
		{"/*** */", []string{"/*** */"}, []bool{false}, 0},
		{"/**x*/", []string{"/**x*/"}, []bool{false}, 0},
		{"/**@var Foo $x */", []string{"/**@var Foo $x */"}, []bool{false}, 0},
		{"/**/ $a; /* b */", []string{"/**/", "$a", ";", "/* b */"}, []bool{false, false, false, false}, 0},
		{"/**/x", []string{"/**/", "x"}, []bool{false, false}, 0},
		{"/**/*/", []string{"/**/", "*", "/"}, []bool{false, false, false}, 0},
		{"/*/ $a;", []string{"/*/ $a;"}, []bool{false}, 1},
		{"/*/", []string{"/*/"}, []bool{false}, 1},
		{"/**", []string{"/**"}, []bool{false}, 1},
		{"/** ", []string{"/** "}, []bool{true}, 1},
	}
	for _, tt := range tests {
		t.Run(tt.source, func(t *testing.T) {
			tokens, errs := tokenizeFrom(tt.source, false)
			if len(tokens) != len(tt.values)+1 || tokens[len(tokens)-1].Kind != TokenEOF {
				t.Fatalf("tokens = %#v, want %q and the end of input", tokens, tt.values)
			}
			for i, want := range tt.values {
				got := tokens[i]
				if got.Value != want || (got.Kind == TokenDocComment) != tt.docComment[i] {
					t.Errorf("token %d = %#v, want %q (doc comment: %v)", i, got, want, tt.docComment[i])
				}
			}
			if len(errs) != tt.errors {
				t.Errorf("errors = %#v, want %d", errs, tt.errors)
			}
		})
	}
}
