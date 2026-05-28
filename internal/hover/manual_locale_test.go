package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/config"
	"github.com/Tusk-PHP/lsp/internal/container"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// TestHoverBuiltinFunctionLocale verifies that when the provider's config
// carries PHPManualLocale = "es", the rendered hover for a builtin function
// contains a PHP Manual link using "/manual/es/" in the URL.
func TestHoverBuiltinFunctionLocale(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")

	cfg := config.DefaultConfig()
	cfg.PHPManualLocale = "es"
	p.SetConfig(cfg)

	// A minimal PHP file that calls strlen so the cursor lands on a builtin.
	source := `<?php
strlen("hello");
`
	pos := protocol.Position{Line: 1, Character: 2}
	h := p.GetHover("file:///test.php", source, pos)
	if h == nil {
		t.Fatal("expected hover result for builtin function")
	}
	val := h.Contents.Value
	if !strings.Contains(val, "/manual/es/") {
		t.Errorf("expected /manual/es/ in hover URL, got:\n%s", val)
	}
}

// TestHoverBuiltinFunctionNoLocale confirms that with no config set the
// hover falls back to the default "en" locale.
func TestHoverBuiltinFunctionNoLocale(t *testing.T) {
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	ca := container.NewContainerAnalyzer(idx, "/tmp", "none")
	p := NewProvider(idx, ca, "none")
	// No SetConfig call — cfg is nil, locale defaults to "".

	source := `<?php
strlen("hello");
`
	pos := protocol.Position{Line: 1, Character: 2}
	h := p.GetHover("file:///test.php", source, pos)
	if h == nil {
		t.Fatal("expected hover result for builtin function")
	}
	val := h.Contents.Value
	if !strings.Contains(val, "/manual/en/") {
		t.Errorf("expected /manual/en/ in hover URL, got:\n%s", val)
	}
}
