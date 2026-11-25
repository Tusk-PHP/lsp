package sourcectx

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func TestFunctionCallMarksOnlyANameCalledAsAFunction(t *testing.T) {
	cases := []struct {
		src  string
		want bool
	}{
		{"area|(1);", true},
		{"$x = ar|ea (1);", true},
		{"\\App\\ar|ea(1);", true},
		{"App\\ar|ea(1);", true},
		{"echo area|;", false},
		{"new Ar|ea(1);", false},
		{"new \\App\\Ar|ea(1);", false},
		{"function ar|ea() {}", false},
		{"$this->ar|ea();", false},
		{"Room::ar|ea();", false},
		{"#[Ar|ea(1)]\nfunction f() {}", false},
		{"#[Other, Ar|ea([1, 2])]\nfunction f() {}", false},
		{"$a = [ar|ea(1)];", true},
		{"f(ar|ea: 1);", false},
		{"/** ar|ea() */", false},
		{"$x = [f(1)[0], ar|ea(2)];", true},
		{"if (true) { ar|ea(1); }", true},
		{"#[Other([1], f(2)), Ar|ea(1)]\nfunction f() {}", false},
		{"$ar|ea(1);", false},
		{"echo ar|ea", false},
	}
	for _, tc := range cases {
		t.Run(tc.src, func(t *testing.T) {
			src := "<?php\n" + tc.src + "\n"
			offset := strings.Index(src, "|")
			src = src[:offset] + src[offset+1:]
			line := strings.Count(src[:offset], "\n")
			char := offset - strings.LastIndex(src[:offset], "\n") - 1
			ctx := Analyze("file:///a.php", src, protocol.Position{Line: line, Character: char})
			if ctx == nil {
				t.Fatal("expected cursor context")
			}
			if ctx.FunctionCall != tc.want {
				t.Fatalf("FunctionCall = %v, want %v (word %q)", ctx.FunctionCall, tc.want, ctx.SymbolText)
			}
		})
	}
}

func TestFunctionCallWithoutTokens(t *testing.T) {
	if isFunctionCall(nil, protocol.Range{End: protocol.Position{Line: 1, Character: 4}}) {
		t.Error("no tokens, no call")
	}
}
