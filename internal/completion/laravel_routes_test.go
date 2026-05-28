package completion

import (
	"path/filepath"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func setupLaravelRouteCompletion(t *testing.T) *Provider {
	t.Helper()

	idx, ca := setupLaravelIndex(t)
	p := NewProvider(idx, ca, "laravel")

	routes := frameworklaravel.NewRouteIndex(filepath.Join("..", "..", "testdata", "laravel"))
	if err := routes.ScanWorkspace(); err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	p.SetLaravelRouteIndex(routes)

	return p
}

func TestLaravelRouteNameCompletionHelper(t *testing.T) {
	p := setupLaravelRouteCompletion(t)

	source := `<?php
return route('da');
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 16})
	labels := collectLabels(items)

	if !labels["dashboard"] {
		t.Fatalf("expected dashboard completion, got %v", labels)
	}
	if labels["profile.edit"] {
		t.Fatalf("did not expect profile.edit for 'da' prefix, got %v", labels)
	}
}

func TestLaravelRouteNameCompletionRouteIs(t *testing.T) {
	p := setupLaravelRouteCompletion(t)

	source := `<?php
$active = request()->routeIs('pro');
`
	items := p.GetCompletions("file:///test.php", source, protocol.Position{Line: 1, Character: 33})
	labels := collectLabels(items)

	if !labels["profile.edit"] {
		t.Fatalf("expected profile.edit completion, got %v", labels)
	}
}
