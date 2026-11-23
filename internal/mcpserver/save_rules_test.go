package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// TestProjectDiagnosticsRunSaveRules checks that the diagnostics tool reports
// what the editor reports after a save, such as redundant-nullsafe.
func TestProjectDiagnosticsRunSaveRules(t *testing.T) {
	root := t.TempDir()
	src := "<?php\nnamespace App;\nclass Logger { public function info(): void {} }\n" +
		"class Service {\n    public function run(Logger $logger): void\n    {\n        $logger?->info();\n    }\n}\n"
	if err := os.MkdirAll(filepath.Join(root, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "src", "Service.php"), []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	out, err := svc.projectDiagnostics("src/Service.php")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range out.Files {
		for _, d := range f.Diagnostics {
			found = found || d.Code == "redundant-nullsafe"
		}
	}
	if !found {
		t.Errorf("no redundant-nullsafe in %+v", out)
	}
}
