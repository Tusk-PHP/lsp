package diagnostics

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestMemoizeTypeResolverAsksOncePerRun(t *testing.T) {
	calls := 0
	resolve := MemoizeTypeResolver(func(file *parser.FileNode, e ast.Expr) string {
		calls++
		return e.Bounds().Text(file.Source) + "@" + file.Source[len("<?php\n"):len("<?php\n")+2]
	})
	v1 := parser.ParseFile("<?php\nv1($a, $b);\n", token.Latest)
	args := v1.Tree.Stmts[0].(*ast.ExprStmt).Expr.(*ast.Call).Args.Args
	for i := 0; i < 3; i++ {
		if got := resolve(v1, args[0].Value); got != "$a@v1" {
			t.Fatalf("got %q", got)
		}
	}
	resolve(v1, args[1].Value)
	if calls != 2 {
		t.Fatalf("calls = %d, want 2", calls)
	}
	v2 := parser.ParseFile("<?php\nv2($a, $b);\n", token.Latest)
	if got := resolve(v2, v2.Tree.Stmts[0].(*ast.ExprStmt).Expr.(*ast.Call).Args.Args[0].Value); got != "$a@v2" || calls != 3 {
		t.Fatalf("got %q after %d calls", got, calls)
	}
	if MemoizeTypeResolver(nil) != nil {
		t.Fatal("nil resolver should stay nil")
	}
}
