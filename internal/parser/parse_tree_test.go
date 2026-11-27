package parser

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/php/ast"
	"github.com/Tusk-PHP/lsp/internal/php/token"
)

func TestParseTreeReadsTheSelectedVersion(t *testing.T) {
	src := "<?php enum Suit {}"
	if tree := ParseTree(src, token.PHP80); len(tree.Errors) == 0 {
		t.Errorf("PHP 8.0 has no enums, want a syntax error, got none")
	}
	tree := ParseTree(src, token.PHP81)
	if len(tree.Errors) != 0 || tree.Version != token.PHP81 {
		t.Fatalf("PHP 8.1: errors %v, version %v", tree.Errors, tree.Version)
	}
	if len(tree.Stmts) != 1 {
		t.Fatalf("PHP 8.1: want one statement, got %d", len(tree.Stmts))
	}
	if _, ok := tree.Stmts[0].(*ast.EnumDecl); !ok {
		t.Errorf("PHP 8.1: want an enum declaration, got %T", tree.Stmts[0])
	}
}

func TestParseTreeReadsCodeWithoutAnOpenTag(t *testing.T) {
	src := "class C {}"
	tree := ParseTree(src, token.Latest)
	if len(tree.Errors) != 0 || len(tree.Stmts) != 1 {
		t.Fatalf("errors %v, %d statements", tree.Errors, len(tree.Stmts))
	}
	class, ok := tree.Stmts[0].(*ast.ClassDecl)
	if !ok || class.Name.Text(src) != "C" {
		t.Errorf("want class C, got %#v", tree.Stmts[0])
	}
}
