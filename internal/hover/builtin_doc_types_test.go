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

func TestHoverTypesBuiltinMethodResultsFromTheirPHPDoc(t *testing.T) {
	source := `<?php
function zones(\Exception $e) {
    foreach (\DateTimeZone::listIdentifiers() as $zone) {
        echo $zone;
    }
    foreach ($e->getTrace() as $frame) {
        echo $frame;
    }
}
`
	idx := symbols.NewIndex()
	idx.RegisterBuiltinsForProfile(symbols.BuiltinProfile{PHPVersion: "8.4"})
	p := NewProvider(idx, nil, "")
	want := map[string]string{"$zone": "string $zone", "$frame": "array{function: string"}
	for i, line := range strings.Split(source, "\n") {
		for variable, typ := range want {
			c := strings.Index(line, "echo "+variable+";")
			if c < 0 {
				continue
			}
			h := p.GetHover("file:///app/zones.php", source, protocol.Position{Line: i, Character: c + 6})
			if h == nil || !strings.Contains(h.Contents.Value, typ) {
				t.Errorf("%s should show %q, got %+v", variable, typ, h)
			}
		}
	}
}
