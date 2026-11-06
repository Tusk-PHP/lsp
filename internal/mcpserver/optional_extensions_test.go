package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSuggestedExtensionsReachMCPIndex(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "composer.json"), []byte(`{"require":{"php":"^7.4"},"suggest":{"ext-imagick":"Images","ext-ssh2":"Remote files"}}`), 0644); err != nil {
		t.Fatal(err)
	}
	svc, err := New(context.Background(), root, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Imagick::readImage", "ssh2_connect"} {
		if svc.Workspace.Index.Lookup(name) == nil {
			t.Errorf("missing %s from MCP index", name)
		}
	}
	if svc.Workspace.Index.Lookup("Redis") != nil {
		t.Error("unselected Redis extension was indexed")
	}
}
