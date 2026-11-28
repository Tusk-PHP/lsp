package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestProjectDiagnosticsThroughASymlinkedRoot(t *testing.T) {
	root := t.TempDir()
	src := "<?php\nnamespace App;\nclass Service {\n    public function run(): int\n    {\n        return $missing;\n    }\n}\n"
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "Service.php"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "app")
	if err := os.Symlink(root, link); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}

	diagnose := func(svcRoot, path string) DiagnosticsOutput {
		t.Helper()
		svc, err := New(context.Background(), svcRoot, nil)
		if err != nil {
			t.Fatal(err)
		}
		out, err := svc.projectDiagnostics(path)
		if err != nil {
			t.Fatalf("diagnostics of %q under %s: %v", path, svcRoot, err)
		}
		return out
	}
	want := diagnose(root, "")
	if len(want.Files) != 1 || want.Files[0].Path != "src/Service.php" || len(want.Files[0].Diagnostics) == 0 {
		t.Fatalf("through the real root: %+v", want)
	}
	for _, path := range []string{"", "src/Service.php", filepath.Join(link, "src", "Service.php")} {
		if got := diagnose(link, path); !reflect.DeepEqual(got, want) {
			t.Errorf("through the symlinked root, path %q: %+v, want %+v", path, got, want)
		}
	}
}

func TestProjectDiagnosticsOfAMissingPathIsAnError(t *testing.T) {
	svc, err := New(context.Background(), t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	if out, err := svc.projectDiagnostics("src/Gone.php"); err == nil {
		t.Errorf("diagnostics of a missing file = %+v, want an error", out)
	}
}
