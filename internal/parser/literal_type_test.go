package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestLiteralTypesAreReadFromTheNode(t *testing.T) {
	for expr, want := range map[string][2]string{
		"'hello'":                          {"'hello'", "string"},
		`"hello"`:                          {`"hello"`, "string"},
		`"a $b"`:                           {"string", "string"},
		"<<<EOT\nx\nEOT":                   {"string", "string"},
		"TRUE":                             {"true", "bool"},
		"\\false":                          {"false", "bool"},
		"null":                             {"null", "null"},
		"PHP_EOL":                          {"", ""},
		"42":                               {"42", "int"},
		"-5":                               {"-5", "int"},
		"+5":                               {"5", "int"},
		"0x1F":                             {"int", "int"},
		"1_000":                            {"int", "int"},
		"3.14":                             {"3.14", "float"},
		"-1e10":                            {"float", "float"},
		"-$x":                              {"", ""},
		"!true":                            {"", ""},
		"(int) $x":                         {"int", "int"},
		"(object) []":                      {"object", "object"},
		"(float) $x":                       {"float", "float"},
		"(string) $x":                      {"string", "string"},
		"(bool) $x":                        {"bool", "bool"},
		"(array) $x":                       {"array", "array"},
		"(void) $x":                        {"", ""},
		"[1, , 2]":                         {"array<int, int>", "array"},
		"self::class":                      {"string", "string"},
		"\\App\\Foo::class":                {"string", "string"},
		"Foo::CLASS_NAME":                  {"", ""},
		"$foo::class":                      {"", ""},
		"$var":                             {"", ""},
		"[]":                               {"array", "array"},
		"array(1)":                         {"array<int, int>", "array"},
		"['id' => 1, 'name' => 'desk']":    {"array{id: int, name: string}", "array"},
		"[1, 2, 3]":                        {"array<int, int>", "array"},
		"[['id' => 1], ['id' => 2]]":       {"array<int, array{id: int}>", "array"},
		"[[1], [2]]":                       {"array<int, array>", "array"},
		"[1, 'x']":                         {"array", "array"},
		"[$a, $b]":                         {"array", "array"},
		"[...$a]":                          {"array", "array"},
		"['a,b' => [1, 2], 2 => foo(1)]":   {"array{a,b: array, 2: mixed}", "array"},
		"['k' => ['x' => true], 'n' => 1]": {"array{k: array{x: bool}, n: int}", "array"},
	} {
		src := "return " + expr + ";"
		e := phpparser.ParseCode(src, token.Latest).Stmts[0].(*ast.Return).Value
		if got := LiteralExprType(e, src); got != want[0] {
			t.Errorf("LiteralExprType(%s) = %q, want %q", expr, got, want[0])
		}
		if got := LiteralValueType(e, src); got != want[1] {
			t.Errorf("LiteralValueType(%s) = %q, want %q", expr, got, want[1])
		}
	}
}

func TestCastTypeOfCastsWithoutAValue(t *testing.T) {
	if got := CastType(token.UnsetCast); got != "null" {
		t.Errorf("(unset) = %q, want null", got)
	}
	if got := CastType(token.VoidCast); got != "" {
		t.Errorf("(void) = %q, want no type", got)
	}
}
