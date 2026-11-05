package completion

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestResolveEnumCaseNamedLikeAClass(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	idx.IndexFile("file:///e.php", "<?php\nnamespace App\\Enums;\nenum UserRole: string { case Admin = 'a'; case Client = 'c'; }\n")
	idx.IndexFile("file:///c.php", "<?php\nnamespace App\\Models;\nclass Client {}\n")
	src := "<?php\nnamespace App\\Models;\nuse App\\Enums\\UserRole;\nclass F { function d() {\n  $x = UserRole::Client->value;\n  $y = UserRole::Admin->value;\n} }\n"
	p := NewProvider(idx, container.NewContainerAnalyzer(idx, "/tmp", "none"), "none")
	f := parser.ParseFile(src)
	// The `Client` case must resolve to the enum, not the App\Models\Client class.
	for _, e := range []string{"UserRole::Client", "UserRole::Admin"} {
		if got := p.ResolveExpressionType(e, src, protocol.Position{Line: 4}, f); got != "App\\Enums\\UserRole" {
			t.Errorf("%s resolved to %q, want App\\Enums\\UserRole", e, got)
		}
		if got := p.ResolveExpressionTypeTyped(e, src, protocol.Position{Line: 4}, f).BaseFQN(); got != "App\\Enums\\UserRole" {
			t.Errorf("typed: %s resolved to %q", e, got)
		}
	}
}
