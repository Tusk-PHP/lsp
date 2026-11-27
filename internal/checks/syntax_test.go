package checks

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestSyntaxErrorRuleRanges(t *testing.T) {
	source := "<?php\n$a = 1\n\n/* open\nend"
	file := &parser.FileNode{Errors: []parser.ParseError{
		{Message: "expected ';'", Line: 1, Column: 6},
		{Message: "unexpected end", Line: 2, Column: 0},
		{Message: "unterminated comment", Line: 3, Column: 0},
		{Message: "unterminated doc comment", Line: 4, Column: 1},
		{Message: "past the end", Line: 9, Column: 40},
	}}
	got := (&SyntaxErrorRule{}).Check(file, source, nil)
	want := [][4]int{{1, 5, 1, 6}, {2, 0, 2, 1}, {3, 0, 3, 2}, {4, 1, 4, 3}, {4, 2, 4, 3}}
	if len(got) != len(want) {
		t.Fatalf("got %d findings: %+v", len(got), got)
	}
	for i, f := range got {
		if r := [4]int{f.StartLine, f.StartCol, f.EndLine, f.EndCol}; r != want[i] || f.Code != rule.SyntaxError || f.Severity != rule.SeverityError {
			t.Errorf("finding %d: %v %s %v, want range %v", i, r, f.Code, f.Severity, want[i])
		}
	}
	if (&SyntaxErrorRule{}).Check(&parser.FileNode{}, source, nil) != nil {
		t.Error("a file without errors has no syntax findings")
	}
}

func TestAbstractInConcreteRule(t *testing.T) {
	source := "<?php\nclass A {\n    abstract function run();\n}\nabstract class B {\n    abstract function run();\n}\n"
	got := (&AbstractInConcreteRule{}).Check(parser.ParseFile(source, token.Latest), source, symbols.NewIndex())
	if len(got) != 1 || got[0].StartLine != 2 || got[0].Code != rule.AbstractInConcrete {
		t.Errorf("got %+v", got)
	}
	if (&AbstractInConcreteRule{}).Check(nil, "", nil) != nil {
		t.Error("no file, no findings")
	}
}

func TestUnknownClassInCatchAndImplements(t *testing.T) {
	source := "<?php\nclass A implements Known, Missing {\n    function f() {\n        try {} catch (\\RuntimeException | Gone $e) {}\n    }\n}\n"
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///Known.php", "<?php\ninterface Known {}\n")
	var names []string
	for _, f := range (&UnknownClassRule{}).Check(parser.ParseFile(source, token.Latest), source, idx) {
		names = append(names, f.Message)
	}
	want := []string{"Unknown class 'Missing'", "Unknown class 'Gone'"}
	if len(names) != len(want) || names[0] != want[0] || names[1] != want[1] {
		t.Errorf("got %v, want %v", names, want)
	}
}
