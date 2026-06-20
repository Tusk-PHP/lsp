package lsp

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/composer"
)

func testdataAutoload() []composer.AutoloadEntry {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "project")
	return composer.GetAutoloadPaths(root)
}

func testdataSrcDirURI() string {
	_, file, _, _ := runtime.Caller(0)
	root := filepath.Join(filepath.Dir(file), "..", "..", "testdata", "project")
	return "file://" + filepath.ToSlash(filepath.Join(root, "src"))
}

func TestBuildCreateObjectEdit_ClassInSubdir(t *testing.T) {
	autoload := testdataAutoload()
	dirURI := testdataSrcDirURI() + "/Models"

	edit := buildCreateObjectEdit(dirURI, "class", "User", autoload)
	if edit == nil {
		t.Fatal("expected non-nil WorkspaceEdit for class User in src/Models")
	}
	if len(edit.DocumentChanges) != 2 {
		t.Fatalf("expected 2 DocumentChanges, got %d", len(edit.DocumentChanges))
	}

	// First element: CreateFile op.
	createOp := edit.DocumentChanges[0].CreateFile
	if createOp == nil {
		t.Fatal("expected first DocumentChange to be a CreateFile")
	}
	if createOp.Kind != "create" {
		t.Errorf("CreateFile.Kind = %q, want %q", createOp.Kind, "create")
	}
	if !strings.HasSuffix(createOp.URI, "/User.php") {
		t.Errorf("CreateFile.URI = %q, expected suffix /User.php", createOp.URI)
	}
	if createOp.Options == nil || !createOp.Options.IgnoreIfExists {
		t.Errorf("expected CreateFile.Options.IgnoreIfExists = true")
	}

	// Second element: TextDocumentEdit with skeleton.
	tde := edit.DocumentChanges[1].TextDocumentEdit
	if tde == nil {
		t.Fatal("expected second DocumentChange to be a TextDocumentEdit")
	}
	if tde.TextDocument.URI != createOp.URI {
		t.Errorf("TextDocumentEdit URI = %q, want %q", tde.TextDocument.URI, createOp.URI)
	}
	if tde.TextDocument.Version != nil {
		t.Errorf("expected TextDocument.Version to be nil")
	}
	if len(tde.Edits) != 1 {
		t.Fatalf("expected 1 TextEdit, got %d", len(tde.Edits))
	}
	text := tde.Edits[0].NewText
	if !strings.HasPrefix(text, "<?php") {
		t.Errorf("skeleton should start with <?php, got: %q", text)
	}
	if !strings.Contains(text, "namespace App\\Models;") {
		t.Errorf("skeleton missing namespace App\\Models;, got:\n%s", text)
	}
	if !strings.Contains(text, "class User") {
		t.Errorf("skeleton missing 'class User', got:\n%s", text)
	}
	if !strings.Contains(text, "{\n}") {
		t.Errorf("skeleton missing empty body {\\n}, got:\n%s", text)
	}
}

func TestBuildCreateObjectEdit_InterfaceInRoot(t *testing.T) {
	autoload := testdataAutoload()
	dirURI := testdataSrcDirURI() // src/ is the PSR-4 root for App\

	edit := buildCreateObjectEdit(dirURI, "interface", "Repository", autoload)
	if edit == nil {
		t.Fatal("expected non-nil WorkspaceEdit for interface Repository in src/")
	}
	if len(edit.DocumentChanges) != 2 {
		t.Fatalf("expected 2 DocumentChanges, got %d", len(edit.DocumentChanges))
	}

	tde := edit.DocumentChanges[1].TextDocumentEdit
	if tde == nil {
		t.Fatal("expected second DocumentChange to be a TextDocumentEdit")
	}
	text := tde.Edits[0].NewText
	if !strings.Contains(text, "namespace App;") {
		t.Errorf("skeleton missing namespace App;, got:\n%s", text)
	}
	if !strings.Contains(text, "interface Repository") {
		t.Errorf("skeleton missing 'interface Repository', got:\n%s", text)
	}
}

func TestBuildCreateObjectEdit_Enum(t *testing.T) {
	autoload := testdataAutoload()
	dirURI := testdataSrcDirURI() + "/Models"

	edit := buildCreateObjectEdit(dirURI, "enum", "Status", autoload)
	if edit == nil {
		t.Fatal("expected non-nil WorkspaceEdit for enum Status")
	}

	tde := edit.DocumentChanges[1].TextDocumentEdit
	if tde == nil {
		t.Fatal("expected second DocumentChange to be a TextDocumentEdit")
	}
	text := tde.Edits[0].NewText
	if !strings.Contains(text, "enum Status") {
		t.Errorf("skeleton missing 'enum Status', got:\n%s", text)
	}
}

func TestBuildCreateObjectEdit_InvalidKind(t *testing.T) {
	autoload := testdataAutoload()
	dirURI := testdataSrcDirURI()

	edit := buildCreateObjectEdit(dirURI, "function", "helper", autoload)
	if edit != nil {
		t.Errorf("expected nil for invalid kind 'function', got non-nil")
	}
}

func TestBuildCreateObjectEdit_EmptyName(t *testing.T) {
	autoload := testdataAutoload()
	dirURI := testdataSrcDirURI()

	edit := buildCreateObjectEdit(dirURI, "class", "", autoload)
	if edit != nil {
		t.Errorf("expected nil for empty name, got non-nil")
	}
}

func TestBuildCreateObjectEdit_TrailingSlashDirURI(t *testing.T) {
	autoload := testdataAutoload()
	dirURI := testdataSrcDirURI() + "/Models/"

	edit := buildCreateObjectEdit(dirURI, "trait", "Auditable", autoload)
	if edit == nil {
		t.Fatal("expected non-nil WorkspaceEdit when dirURI has trailing slash")
	}
	createOp := edit.DocumentChanges[0].CreateFile
	// Ensure the path portion has no double slash (file:// prefix is normal — check
	// that the path segment after the scheme+host doesn't contain "//").
	pathPart := strings.TrimPrefix(createOp.URI, "file://")
	if strings.Contains(pathPart, "//") {
		t.Errorf("file URI path portion contains double slash: %q", createOp.URI)
	}
	if !strings.HasSuffix(createOp.URI, "/Auditable.php") {
		t.Errorf("expected URI to end with /Auditable.php, got: %q", createOp.URI)
	}
}

func TestBuildCreateObjectEdit_NoNamespaceOutsidePSR4(t *testing.T) {
	// A dirURI not under any PSR-4 root — namespace should be omitted.
	edit := buildCreateObjectEdit("file:///tmp/random_dir", "class", "Foo", nil)
	if edit == nil {
		t.Fatal("expected non-nil WorkspaceEdit even without PSR-4 autoload")
	}
	tde := edit.DocumentChanges[1].TextDocumentEdit
	text := tde.Edits[0].NewText
	if strings.Contains(text, "namespace") {
		t.Errorf("expected no namespace line when outside PSR-4 root, got:\n%s", text)
	}
	if !strings.HasPrefix(text, "<?php\n") {
		t.Errorf("skeleton should start with <?php\\n, got: %q", text)
	}
	if !strings.Contains(text, "class Foo") {
		t.Errorf("skeleton missing 'class Foo', got:\n%s", text)
	}
}
