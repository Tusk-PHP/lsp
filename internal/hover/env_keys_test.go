package hover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverOnAnEnvKeyNamesItsFilesWithoutItsValue(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".env":         "DB_PASSWORD=\"hunter2\"\n",
		".env.example": "DB_PASSWORD=example-secret\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := symbols.NewIndex()
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, root, "laravel"), "laravel")
	p.SetEnvKeys(frameworklaravel.NewEnvKeys(root))

	line := "    'password' => env('DB_PASSWORD', ''),"
	source := "<?php\nreturn [\n" + line + "\n];\n"
	start := strings.Index(line, "DB_PASSWORD")
	h := p.GetHover("file:///config/database.php", source, protocol.Position{Line: 2, Character: start + 3})
	if h == nil {
		t.Fatal("no hover on the key")
	}
	if !strings.Contains(h.Contents.Value, "DB_PASSWORD") || !strings.Contains(h.Contents.Value, "`.env`, `.env.example`") {
		t.Errorf("hover names the key and its files: %q", h.Contents.Value)
	}
	if strings.Contains(h.Contents.Value, "hunter2") || strings.Contains(h.Contents.Value, "example-secret") {
		t.Errorf("hover shows no value: %q", h.Contents.Value)
	}
	want := protocol.Range{Start: protocol.Position{Line: 2, Character: start}, End: protocol.Position{Line: 2, Character: start + len("DB_PASSWORD")}}
	if h.Range == nil || *h.Range != want {
		t.Errorf("hover range %+v, want %+v", h.Range, want)
	}
}
