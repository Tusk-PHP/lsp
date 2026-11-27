package hover

import (
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const explodeSource = `<?php
function templateOf(string $id) {
    $parts = explode('//', $id);
    foreach ($parts as $part) {
        echo $part;
    }
    echo $parts;
}
`

func hoverEcho(t *testing.T, version, variable string) string {
	t.Helper()
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: version})
	p := NewProvider(idx, nil, "")
	for i, line := range strings.Split(explodeSource, "\n") {
		if c := strings.Index(line, "echo "+variable+";"); c >= 0 {
			h := p.GetHover("file:///app/template.php", explodeSource, protocol.Position{Line: i, Character: c + 6})
			if h == nil {
				t.Fatalf("no hover on %s", variable)
			}
			return h.Contents.Value
		}
	}
	t.Fatalf("no echo of %s", variable)
	return ""
}

func TestHoverTypesBuiltinResultsFromTheirPHPDoc(t *testing.T) {
	if got := hoverEcho(t, "8.0", "$part"); !strings.Contains(got, "string $part") {
		t.Errorf("an element of explode() should be a string, got:\n%s", got)
	}
	if got := hoverEcho(t, "8.0", "$parts"); !strings.Contains(got, "list<string> $parts") {
		t.Errorf("explode() on PHP 8 should give list<string>, got:\n%s", got)
	}
	if got := hoverEcho(t, "7.4", "$parts"); !strings.Contains(got, "list<string>|") {
		t.Errorf("explode() on PHP 7.4 should give list<string> or false, got:\n%s", got)
	}
}
