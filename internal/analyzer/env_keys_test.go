package analyzer

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

func TestDefinitionOfAnEnvKeyGoesToItsLine(t *testing.T) {
	root := t.TempDir()
	for name, content := range map[string]string{
		".env":         "APP_NAME=Shop\n\nexport APP_DEBUG=true\n",
		".env.example": "APP_NAME=Laravel\nAPP_DEBUG=false\nMAIL_HOST=smtp\n",
	} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	idx := symbols.NewIndex()
	a := NewAnalyzer(idx, container.NewContainerAnalyzer(idx, root, "laravel"))
	a.SetEnvKeys(frameworklaravel.NewEnvKeys(root))
	env := symbols.PathToURI(filepath.Join(root, ".env"))
	example := symbols.PathToURI(filepath.Join(root, ".env.example"))
	cases := []struct {
		key  string
		want *protocol.Location
	}{
		{"APP_DEBUG", &protocol.Location{URI: env, Range: protocol.Range{Start: protocol.Position{Line: 2, Character: 7}, End: protocol.Position{Line: 2, Character: 16}}}},
		{"MAIL_HOST", &protocol.Location{URI: example, Range: protocol.Range{Start: protocol.Position{Line: 2, Character: 0}, End: protocol.Position{Line: 2, Character: 9}}}},
		{"MISSING", nil},
	}
	for _, tc := range cases {
		line := "$value = env('" + tc.key + "', 'default');"
		source := "<?php\n" + line + "\n"
		loc := a.FindDefinition("file:///config/app.php", source, protocol.Position{Line: 1, Character: strings.Index(line, tc.key) + 2})
		if (loc == nil) != (tc.want == nil) || loc != nil && *loc != *tc.want {
			t.Errorf("%s: definition %+v, want %+v", tc.key, loc, tc.want)
		}
	}
}
