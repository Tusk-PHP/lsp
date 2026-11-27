package ast_test

import (
	"fmt"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestExprSpanning(t *testing.T) {
	src := "<?php\n$a = foo($b->c(), 1);\nfunction f() {}\n"
	file := parser.Parse(src, token.Latest)
	span := func(text string) ast.Span {
		start := uint32(strings.Index(src, text))
		return ast.Span{Start: start, End: start + uint32(len(text))}
	}
	for text, want := range map[string]string{
		"foo($b->c(), 1)":      "*ast.Call",
		"$b->c()":              "*ast.MethodCall",
		"$b":                   "*ast.Variable",
		"$a = foo($b->c(), 1)": "*ast.Assign",
	} {
		got := ast.ExprSpanning(file, span(text))
		if got == nil || got.Bounds() != span(text) || fmt.Sprintf("%T", got) != want {
			t.Errorf("%q: got %T", text, got)
		}
	}
	for _, text := range []string{"foo($b", "function f() {}", "b->c"} {
		if got := ast.ExprSpanning(file, span(text)); got != nil {
			t.Errorf("%q: got %T", text, got)
		}
	}
}
