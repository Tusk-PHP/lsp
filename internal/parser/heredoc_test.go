package parser

import "testing"

func TestTokenizeHeredocAndNowdocAsSingleString(t *testing.T) {
	src := "<?php\nfunction f() {\n    $a = <<<SQL\n    insert into t (a) values\n    {$values};\n    SQL;\n    $b = <<<'TXT'\n}\nTXT;\n    $c = <<<\"X\"\n    X . 'y';\n}\n"
	var strs []Token
	for _, tok := range New().Parse(src).Tokens {
		if tok.Kind == TokenStringLiteral {
			strs = append(strs, tok)
		}
	}
	want := []string{
		"<<<SQL\n    insert into t (a) values\n    {$values};\n    SQL",
		"<<<'TXT'\n}\nTXT",
		"<<<\"X\"\n    X",
		"'y'",
	}
	if len(strs) != len(want) {
		t.Fatalf("got %d string tokens, want %d: %+v", len(strs), len(want), strs)
	}
	for i, w := range want {
		if strs[i].Value != w {
			t.Errorf("token %d = %q, want %q", i, strs[i].Value, w)
		}
	}
	if strs[3].Line != 10 {
		t.Errorf("line tracking after heredocs: 'y' on line %d, want 10", strs[3].Line)
	}
	// The `}` inside the nowdoc must not close the function.
	file := ParseFile(src)
	if len(file.Functions) != 1 || file.Functions[0].EndLine != 12 {
		t.Errorf("function bounds wrong: %+v", file.Functions)
	}
}

func TestShiftOperatorIsNotHeredoc(t *testing.T) {
	src := "<?php\n$a = 1 << 2;\n$b = $x <<<SQL;\n"
	for _, tok := range New().Parse(src).Tokens {
		if tok.Kind == TokenStringLiteral {
			t.Errorf("unexpected string token %q", tok.Value)
		}
	}
}
