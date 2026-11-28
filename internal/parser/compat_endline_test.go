package parser

import (
	"testing"
)

// TestClassNodeEndLine verifies that ClassNode.EndLine is populated from
// ClassDef.EndLine when ParseFile converts the parser result through the
// compat layer (L23 — compat.go ClassDef→ClassNode mapping).
//
// The tokenizer uses 0-based line numbers. In the source below:
//
//	line 0: <?php
//	line 1: namespace App;
//	line 2: (blank)
//	line 3: class Example {
//	line 4:     public string $name;
//	line 5:     public function greet(): void {}
//	line 6: }
func TestClassNodeEndLine(t *testing.T) {
	source := `<?php
namespace App;

class Example {
    public string $name;
    public function greet(): void {}
}
`
	// Verify ClassDef values from the raw parser first so any failure is clear.
	result := New().Parse(source)
	if result == nil {
		t.Fatal("Parse returned nil")
	}
	if len(result.Classes) == 0 {
		t.Fatal("expected at least 1 class from parser")
	}
	classDef := result.Classes[0]
	if classDef.Name != "Example" {
		t.Fatalf("unexpected class name %q", classDef.Name)
	}
	if classDef.EndLine == 0 {
		t.Fatal("ClassDef.EndLine is 0 — parser is not recording end line; fix parser first")
	}

	file := ParseFile(source)
	if file == nil {
		t.Fatal("ParseFile returned nil")
	}
	if len(file.Classes) == 0 {
		t.Fatal("expected at least 1 class in FileNode.Classes")
	}

	cls := file.Classes[0]

	if cls.Name != "Example" {
		t.Errorf("Name = %q; want %q", cls.Name, "Example")
	}

	// StartLine must match ClassDef.Line (the 'class' keyword line).
	if cls.StartLine != classDef.Line {
		t.Errorf("StartLine = %d; want %d (ClassDef.Line)", cls.StartLine, classDef.Line)
	}

	// EndLine must match ClassDef.EndLine (the closing-brace line).
	if cls.EndLine != classDef.EndLine {
		t.Errorf("EndLine = %d; want %d (ClassDef.EndLine)", cls.EndLine, classDef.EndLine)
	}

	// EndLine must be strictly greater than StartLine for a multi-line class.
	if cls.EndLine <= cls.StartLine {
		t.Errorf("EndLine (%d) should be > StartLine (%d) for a multi-line class",
			cls.EndLine, cls.StartLine)
	}
}

// TestClassNodeEndLineMatchesClassDef verifies that ClassNode.EndLine equals
// the EndLine reported by the underlying parser (ClassDef.EndLine), confirming
// the compat layer does not silently zero it out.
func TestClassNodeEndLineMatchesClassDef(t *testing.T) {
	source := `<?php

class Alpha {
    public int $x = 1;
}

class Beta {
    public function run(): void {}
    public function stop(): void {}
}
`
	result := New().Parse(source)
	if result == nil {
		t.Fatal("Parse returned nil")
	}
	if len(result.Classes) != 2 {
		t.Fatalf("expected 2 classes from parser, got %d", len(result.Classes))
	}

	file := ParseFile(source)
	if file == nil {
		t.Fatal("ParseFile returned nil")
	}
	if len(file.Classes) != 2 {
		t.Fatalf("expected 2 classes in FileNode, got %d", len(file.Classes))
	}

	for i, classDef := range result.Classes {
		classNode := file.Classes[i]
		if classNode.EndLine != classDef.EndLine {
			t.Errorf("Classes[%d] %q: ClassNode.EndLine = %d; ClassDef.EndLine = %d (mismatch)",
				i, classDef.Name, classNode.EndLine, classDef.EndLine)
		}
		if classDef.EndLine == 0 {
			t.Errorf("Classes[%d] %q: ClassDef.EndLine is 0 — parser may not be recording it",
				i, classDef.Name)
		}
	}
}

// Interfaces, traits, enums and functions carry the parser's EndLine through
// the compat layer, as classes do.
func TestClassLikeNodesEndLine(t *testing.T) {
	source := `<?php
interface Shape
{
    public function area(): float;
}

trait Named
{
    public function name(): string { return ''; }
}

enum Suit
{
    case Hearts;
}

function helper(): void
{
}
`
	result := New().Parse(source)
	file := ParseFile(source)
	if len(result.Interfaces) != 1 || len(result.Traits) != 1 || len(result.Enums) != 1 || len(result.Functions) != 1 ||
		len(file.Interfaces) != 1 || len(file.Traits) != 1 || len(file.Enums) != 1 || len(file.Functions) != 1 {
		t.Fatalf("want one of each declaration, got %+v", file)
	}
	checks := []struct {
		name               string
		node, def, startAt int
	}{
		{"interface", file.Interfaces[0].EndLine, result.Interfaces[0].EndLine, file.Interfaces[0].StartLine},
		{"trait", file.Traits[0].EndLine, result.Traits[0].EndLine, file.Traits[0].StartLine},
		{"enum", file.Enums[0].EndLine, result.Enums[0].EndLine, file.Enums[0].StartLine},
		{"function", file.Functions[0].EndLine, result.Functions[0].EndLine, file.Functions[0].StartLine},
	}
	for _, c := range checks {
		if c.node != c.def {
			t.Errorf("%s: node EndLine = %d, parser EndLine = %d", c.name, c.node, c.def)
		}
		if c.node <= c.startAt {
			t.Errorf("%s: EndLine %d should be after StartLine %d", c.name, c.node, c.startAt)
		}
	}
}

func TestEndLineIsTheLineOfTheClosingBrace(t *testing.T) {
	src := "<?php\nclass A {\n    public function f() {\n    }\n    abstract function g();\n}\n\ninterface I {}\ntrait T {\n}\nenum E {\n}\nfunction h() {\n}\n"
	r := New().Parse(src)
	got := map[string]int{}
	for _, c := range r.Classes {
		got[c.Name] = c.EndLine
		for _, m := range c.Methods {
			got[m.Name] = m.EndLine
		}
	}
	for _, c := range r.Interfaces {
		got[c.Name] = c.EndLine
	}
	for _, c := range r.Traits {
		got[c.Name] = c.EndLine
	}
	for _, c := range r.Enums {
		got[c.Name] = c.EndLine
	}
	for _, f := range r.Functions {
		got[f.Name] = f.EndLine
	}
	want := map[string]int{"A": 5, "f": 3, "g": 4, "I": 7, "T": 9, "E": 11, "h": 13}
	for name, line := range want {
		if got[name] != line {
			t.Errorf("%s ends on line %d, want %d", name, got[name], line)
		}
	}
}
