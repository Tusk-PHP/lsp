package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestOperatorsAreTypedFromTheirOperands(t *testing.T) {
	r := NewResolver(nil)
	ctx := ExprContext{Locals: map[string]ResolvedType{
		"$i":    {FQN: "int"},
		"$f":    {FQN: "float"},
		"$s":    {FQN: "non-empty-string"},
		"$pos":  {FQN: "positive-int"},
		"$ni":   {FQN: "int", Nullable: true},
		"$num":  ParseGenericType("int|float"),
		"$list": ParseGenericType("list<int>"),
		"$o":    {FQN: `App\Money`},
		"$mix":  {FQN: "mixed"},
	}}
	for expr, want := range map[string]string{
		"1 + $i":                   "int",
		"$pos * $ni":               "int",
		"$i + $f":                  "float",
		"$num - 1":                 "int|float",
		"$i / 2":                   "int|float",
		"$s + 1":                   "int|float",
		"$i % $f":                  "int",
		"$i ** 2":                  "int",
		"$i ** -2":                 "float",
		"$s & 'x'":                 "string",
		"$i | $s":                  "int",
		"-$i":                      "int",
		"-5":                       "int",
		"-'5'":                     "int|float",
		"~$f":                      "int",
		"$list + $list":            "array",
		"$o + 1":                   "",
		"$mix * 2":                 "",
		"$missing + 1":             "",
		"5 > $missing":             "bool",
		"!$o":                      "bool",
		"$o instanceof App\\Money": "bool",
		"$o <=> $mix":              "int",
		"$o . $mix":                "string",
		"@$o":                      `App\Money`,
		"@$i + 1":                  "int",
		"@$missing":                "",
		"(int) $o + 1.5":           "float",
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%s) = %q, want %q", expr, got, want)
		}
	}
}

func TestAssignedOperatorValuesAreTyped(t *testing.T) {
	src := `<?php
function f(array $items, $y, float $rate) {
    $p = 1 + count($items);
    echo $p; // sum with a call
    $q = 5 > $y;
    echo $q; // comparison
    $ratio = count($items) / 2;
    echo $ratio; // division
    $scaled = $rate * 2;
    echo $scaled; // float product
    $unknown = $y + 1;
    echo $unknown; // unknown operand
    $direct = file_get_contents('/tmp/x');
    echo $direct; // call
    $contents = @file_get_contents('/tmp/x');
    echo $contents; // silenced call
}
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	for _, c := range []struct{ v, marker, want string }{
		{"$p", "sum with a call", "int"},
		{"$q", "comparison", "bool"},
		{"$ratio", "division", "int|float"},
		{"$scaled", "float product", "float"},
		{"$unknown", "unknown operand", ""},
		{"$direct", "call", "string|bool"},
		{"$contents", "silenced call", "string|bool"},
	} {
		line := -1
		for i, l := range lines {
			if strings.HasSuffix(l, "// "+c.marker) {
				line = i
			}
		}
		if line < 0 {
			t.Fatalf("no line marked %q", c.marker)
		}
		pos := textpos.Position{Line: line, Character: strings.Index(lines[line], c.v) + 2}
		if got := r.ResolveVariableTypeTyped(c.v, lines, pos, file).FullString(); got != c.want {
			t.Errorf("typed %s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
		if got := r.ResolveVariableType(c.v, lines, pos, file); got != c.want {
			t.Errorf("plain %s (%s) = %q, want %q", c.v, c.marker, got, c.want)
		}
	}
}

func BenchmarkOperatorExpressionType(b *testing.B) {
	r := NewResolver(nil)
	ctx := ExprContext{Locals: map[string]ResolvedType{"$i": {FQN: "int"}, "$f": {FQN: "float"}, "$s": {FQN: "string"}}}
	exprs := []string{"1 + $i * 2", "$i / $f - 1", "$s . $i", "$i ** 2 % 3", "-$i << 1", "$i > $f && !$s", "$s & 'x'"}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		r.ExpressionType(exprs[i%len(exprs)], ctx)
	}
}
