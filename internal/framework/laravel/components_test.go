package laravel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func writeFiles(t *testing.T, root string, files map[string]string) {
	t.Helper()
	for rel, text := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestAnonymousComponents(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"resources/views/components/alert.blade.php":               "@props(['type' => 'info'])",
		"resources/views/components/card/index.blade.php":          "",
		"resources/views/components/accordion/accordion.blade.php": "",
		"resources/views/components/forms/input.blade.php":         "",
		"vendor/livewire/livewire/config/livewire.php":             "<?php return ['component_locations' => [\n resource_path('views/components'),\n resource_path('views/livewire'),\n]];",
		"resources/views/livewire/counter.blade.php":               "",
	})
	read := func(uri string) string {
		data, _ := os.ReadFile(symbols.URIToPath(uri))
		return string(data)
	}
	c := NewComponents(root, nil, nil, read)
	if got := strings.Join(c.ComponentNames(""), " "); got != "accordion alert card counter forms.input" {
		t.Fatalf("names: %q", got)
	}
	for _, name := range []string{"alert", "card", "accordion", "forms.input", "counter"} {
		if _, ok := c.Component(name); !ok {
			t.Errorf("%s not found", name)
		}
	}
	if comp, _ := c.Component("alert"); len(comp.Attributes) != 1 || comp.Attributes[0].Name != "type" {
		t.Errorf("alert's @props: %+v", comp.Attributes)
	}
	if _, ok := c.Component("missing"); ok {
		t.Error("missing found")
	}

	v := NewViews(root)
	if _, ok := v.ViewURI("counter"); !ok {
		t.Error("a view in Livewire's locations should be found")
	}
}

func TestLivewireLocationsUnreadable(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"vendor/livewire/livewire/config/livewire.php": "<?php return ['component_locations' => [env('X')]];",
		"config/livewire.php":                          "<?php return ['component_locations' => [resource_path('views/x'), $somewhere]];",
	})
	if _, known := livewireLocations(root); known {
		t.Fatal("a location the server can't read should make locations unknown")
	}
}

func TestStudlyPath(t *testing.T) {
	if got := studlyPath("forms.input-group"); got != `Forms\InputGroup` {
		t.Fatalf("studlyPath: %s", got)
	}
}
