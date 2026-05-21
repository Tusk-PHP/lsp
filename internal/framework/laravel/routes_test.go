package laravel

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestRouteIndexScanWorkspaceFindsNamedRoutes(t *testing.T) {
	root := filepath.Join("..", "..", "..", "testdata", "laravel")
	idx := NewRouteIndex(root)

	if err := idx.ScanWorkspace(); err != nil {
		t.Fatalf("ScanWorkspace() error = %v", err)
	}

	want := []string{"appearance.edit", "dashboard", "home", "profile.edit", "security.edit"}
	names := idx.Names()
	for _, name := range want {
		found := false
		for _, got := range names {
			if got == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("expected route %q in %v", name, names)
		}
	}

	security := idx.Find("security.edit")
	if security == nil {
		t.Fatal("expected definition for security.edit")
	}
	if !strings.Contains(security.URI, "/routes/settings.php") {
		t.Fatalf("expected settings route URI, got %s", security.URI)
	}
	if security.Range.Start.Line != 23 {
		t.Fatalf("expected security.edit on line 23, got %d", security.Range.Start.Line)
	}
}

func TestRouteIndexReplacesEntriesOnReindex(t *testing.T) {
	idx := NewRouteIndex("/tmp/project")
	uri := "file:///tmp/project/routes/web.php"

	idx.IndexFile(uri, "<?php\nRoute::get('/')->name('home');\n")
	if got := idx.Find("home"); got == nil {
		t.Fatal("expected home after initial index")
	}

	idx.IndexFile(uri, "<?php\nRoute::get('/')->name('dashboard');\n")
	if got := idx.Find("home"); got != nil {
		t.Fatal("expected home to be removed after reindex")
	}
	if got := idx.Find("dashboard"); got == nil {
		t.Fatal("expected dashboard after reindex")
	}
}

func TestExtractRouteNameArgContext(t *testing.T) {
	tests := []struct {
		name      string
		trimmed   string
		wantOK    bool
		wantPart  string
		wantQuote string
	}{
		{"route helper", "route('da", true, "da", "'"},
		{"blade binding", ":href=\"route('pro", true, "pro", "'"},
		{"routeIs", "request()->routeIs('sec", true, "sec", "'"},
		{"to_route helper", "to_route(\"ho", true, "ho", "\""},
		{"past closing paren", "route('home')", false, "", ""},
		{"not a route call", "config('app", false, "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			part, quote, ok := ExtractRouteNameArgContext(tt.trimmed)
			if ok != tt.wantOK {
				t.Fatalf("ok = %v, want %v", ok, tt.wantOK)
			}
			if part != tt.wantPart {
				t.Fatalf("part = %q, want %q", part, tt.wantPart)
			}
			if quote != tt.wantQuote {
				t.Fatalf("quote = %q, want %q", quote, tt.wantQuote)
			}
		})
	}
}

func TestFindRouteNameReference(t *testing.T) {
	line := `<a href="{{ route('dashboard') }}" :current="request()->routeIs('profile.edit')">`

	if name, ok := FindRouteNameReference(line, 20); !ok || name != "dashboard" {
		t.Fatalf("dashboard reference = (%q, %v)", name, ok)
	}
	if name, ok := FindRouteNameReference(line, 67); !ok || name != "profile.edit" {
		t.Fatalf("profile.edit reference = (%q, %v)", name, ok)
	}
}
