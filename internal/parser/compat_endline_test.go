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
