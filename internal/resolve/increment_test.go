package resolve

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestIncrementAndDecrementFollowPHP(t *testing.T) {
	r := NewResolver(nil)
	str := func(literal string) ResolvedType { return ResolvedType{FQN: "string", Literal: literal} }
	ctx := ExprContext{Locals: map[string]ResolvedType{
		"$i":       {FQN: "int", Literal: "0"},
		"$pos":     {FQN: "positive-int"},
		"$f":       {FQN: "float"},
		"$b":       {FQN: "bool", Literal: "true"},
		"$n":       {FQN: "null"},
		"$ni":      {FQN: "int", Nullable: true},
		"$num":     {FQN: "numeric-string"},
		"$s":       {FQN: "string"},
		"$ne":      {FQN: "non-empty-string"},
		"$word":    str("'a'"),
		"$dq":      str(`"Az"`),
		"$five":    str("'5'"),
		"$spaced":  str("' 5'"),
		"$trail":   str("'5 '"),
		"$half":    str("'5.5'"),
		"$exp":     str(`"1e3"`),
		"$huge":    str("'99999999999999999999'"),
		"$signed":  str("'+5'"),
		"$expsign": str(`"1e+3"`),
		"$badexp":  str("'1e'"),
		"$empty":   str("''"),
		"$escaped": str(`'a\'b'`),
		"$interp":  str(`"$x"`),
		"$a":       {FQN: "array"},
		"$o":       {FQN: `App\Counter`},
	}}
	for expr, want := range map[string]string{
		"$i++":       "int",
		"$i--":       "int",
		"$o++":       `App\Counter`,
		"$s++":       "string",
		"$missing++": "",
		"++$i":       "int",
		"--$i":       "int",
		"++$pos":     "int",
		"++$f":       "float",
		"--$f":       "float",
		"++$b":       "bool",
		"++$n":       "int",
		"--$n":       "null",
		"++$ni":      "int",
		"--$ni":      "?int",
		"++$num":     "int|float",
		"++$s":       "",
		"--$s":       "",
		"++$ne":      "",
		"++$word":    "string",
		"++$signed":  "int",
		"++$expsign": "float",
		"++$badexp":  "string",
		"--$word":    "string",
		"++$dq":      "string",
		"++$five":    "int",
		"--$spaced":  "int",
		"++$trail":   "",
		"++$half":    "float",
		"++$exp":     "float",
		"++$huge":    "",
		"++$empty":   "string",
		"--$empty":   "int",
		"++$escaped": "",
		"++$interp":  "",
		"++$a":       "",
		"++$o":       "",
		"++$missing": "",
	} {
		if got := r.ExpressionType(expr, ctx).FullString(); got != want {
			t.Errorf("ExpressionType(%s) = %q, want %q", expr, got, want)
		}
	}
}

func TestAssignedIncrementsAreTyped(t *testing.T) {
	src := `<?php
function f() {
    $i = 0;
    $j = $i++;
    echo $j; // postfix
    $k = ++$i;
    echo $k; // prefix
}
`
	idx := symbols.NewIndex()
	idx.IndexFile("file:///a.php", src)
	r := NewResolver(idx)
	file := parser.ParseFile(src, token.Latest)
	lines := SplitLines(src)
	for _, c := range []struct{ v, marker string }{{"$j", "postfix"}, {"$k", "prefix"}} {
		for i, l := range lines {
			if !strings.HasSuffix(l, "// "+c.marker) {
				continue
			}
			pos := textpos.Position{Line: i, Character: strings.Index(l, c.v) + 2}
			if got := r.ResolveVariableTypeTyped(c.v, lines, pos, file).FQN; got != "int" {
				t.Errorf("typed %s (%s) = %q, want int", c.v, c.marker, got)
			}
			if got := r.ResolveVariableType(c.v, lines, pos, file); got != "int" {
				t.Errorf("plain %s (%s) = %q, want int", c.v, c.marker, got)
			}
		}
	}
}
