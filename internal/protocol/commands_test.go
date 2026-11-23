package protocol

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"
)

func declaredConstants(t *testing.T, typeName string) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "types.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value := spec.(*ast.ValueSpec)
			if ident, ok := value.Type.(*ast.Ident); ok && ident.Name == typeName {
				names = append(names, value.Names[0].Name)
			}
		}
	}
	return names
}

func TestEveryCodeActionKindIsAdvertised(t *testing.T) {
	declared := declaredConstants(t, "CodeActionKind")
	if len(declared) != len(CodeActionKinds) {
		t.Errorf("types.go declares %d code action kinds (%v), CodeActionKinds advertises %d", len(declared), declared, len(CodeActionKinds))
	}
}

func TestEveryCommandIsAdvertised(t *testing.T) {
	declared := declaredConstants(t, "CommandName")
	if len(declared) != len(Commands) {
		t.Errorf("types.go declares %d commands (%v), Commands advertises %d", len(declared), declared, len(Commands))
	}
	for i, c := range Commands {
		if slices.Contains(Commands[i+1:], c) {
			t.Errorf("%s is advertised twice", c)
		}
	}
}

func TestCodeActionKindIn(t *testing.T) {
	tests := []struct {
		name string
		only []CodeActionKind
		kind CodeActionKind
		want bool
	}{
		{name: "empty only allows all", kind: CodeActionQuickFix, want: true},
		{name: "exact match", only: []CodeActionKind{CodeActionQuickFix}, kind: CodeActionQuickFix, want: true},
		{name: "parent kind matches child", only: []CodeActionKind{CodeActionSource}, kind: CodeActionSourceOrganizeImports, want: true},
		{name: "nested parent matches child", only: []CodeActionKind{CodeActionRefactor}, kind: CodeActionRefactorExtract, want: true},
		{name: "child does not match parent", only: []CodeActionKind{CodeActionRefactorExtract}, kind: CodeActionRefactor, want: false},
		{name: "different branch rejected", only: []CodeActionKind{CodeActionSource}, kind: CodeActionQuickFix, want: false},
		{name: "a prefix is not a parent", only: []CodeActionKind{"source.organize"}, kind: CodeActionSourceOrganizeImports, want: false},
	}
	for _, tt := range tests {
		if got := tt.kind.In(tt.only); got != tt.want {
			t.Errorf("%s: %q.In(%v) = %v, want %v", tt.name, tt.kind, tt.only, got, tt.want)
		}
	}
}
