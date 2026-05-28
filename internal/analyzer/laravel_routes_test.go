package analyzer

import (
	"path/filepath"
	"strings"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
)

func setupLaravelRouteAnalyzer(t *testing.T) *Analyzer {
	t.Helper()

	a := setupLaravelAnalyzer(t)
	routes := frameworklaravel.NewRouteIndex(filepath.Join("..", "..", "testdata", "laravel"))
	if err := routes.ScanWorkspace(); err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}
	a.SetLaravelRouteIndex(routes)
	return a
}

func TestDefinitionLaravelRouteNameHelper(t *testing.T) {
	a := setupLaravelRouteAnalyzer(t)

	source := `<?php
return route('dashboard');
`
	loc := a.FindDefinition("file:///test.php", source, protocol.Position{Line: 1, Character: 15})
	if loc == nil {
		t.Fatal("expected definition for route('dashboard')")
	}
	if !strings.Contains(loc.URI, "/routes/web.php") {
		t.Fatalf("expected web.php definition, got %s", loc.URI)
	}
}

func TestDefinitionLaravelRouteNameRouteIs(t *testing.T) {
	a := setupLaravelRouteAnalyzer(t)

	source := `<?php
$active = request()->routeIs('profile.edit');
`
	loc := a.FindDefinition("file:///test.php", source, protocol.Position{Line: 1, Character: 31})
	if loc == nil {
		t.Fatal("expected definition for routeIs('profile.edit')")
	}
	if !strings.Contains(loc.URI, "/routes/settings.php") {
		t.Fatalf("expected settings.php definition, got %s", loc.URI)
	}
}
