package main

import (
	"bytes"
	"os"
	"testing"
)

func TestChildrenIsGeneratedFromTheNodes(t *testing.T) {
	want, err := Generate("../ast.go")
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile("../children_generated.go")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Error("children_generated.go is out of date: run go generate ./internal/php/ast")
	}
}
