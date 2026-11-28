package laravel

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
		data, _ := os.ReadFile(strings.TrimPrefix(uri, "file://"))
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

func TestLivewireLocationsFromTheReturnedArray(t *testing.T) {
	cases := []struct {
		name   string
		config string
		dirs   []string
		known  bool
	}{
		{"every helper", `<?php return ['component_locations' => [
    resource_path('views/components'), // [old, entries]
    \app_path("Livewire"),
    BASE_PATH('modules/ui'),
]];`, []string{"resources/views/components", "app/Livewire", "modules/ui"}, true},
		{"commented-out entries", `<?php return [
    'layout' => ['a', 'b'],
    'component_locations' => [
        /* resource_path('views/old'), */
        resource_path('views/livewire'),
        # app_path('Gone'),
    ],
];`, []string{"resources/views/livewire"}, true},
		{"a hole before the key", `<?php return ['x' => 1, , 'component_locations' => [resource_path('views/livewire')]];`, []string{"resources/views/livewire"}, true},
		{"a hole among the locations", `<?php return ['component_locations' => [resource_path('views/livewire'), , app_path('Livewire')]];`, nil, false},
		{"Livewire 3", `<?php
// 'component_locations' => [resource_path('views/x')],
return ['class_namespace' => 'App\\Livewire'];`, nil, true},
		{"empty", `<?php return ['component_locations' => []];`, nil, true},
		{"not an array", `<?php return ['component_locations' => LOCATIONS];`, nil, false},
		{"another function", `<?php return ['component_locations' => [storage_path('x')]];`, nil, false},
		{"a variable function", `<?php return ['component_locations' => [$path('x')]];`, nil, false},
		{"a method", `<?php return ['component_locations' => [$app->path('x')]];`, nil, false},
		{"interpolated", `<?php return ['component_locations' => [resource_path("views/$x")]];`, nil, false},
		{"two arguments", `<?php return ['component_locations' => [resource_path('a', 'b')]];`, nil, false},
		{"named argument", `<?php return ['component_locations' => [resource_path(path: 'a')]];`, nil, false},
		{"spread argument", `<?php return ['component_locations' => [resource_path(...$a)]];`, nil, false},
		{"keyed entry", `<?php return ['component_locations' => ['x' => resource_path('a')]];`, nil, false},
		{"spread entry", `<?php return ['component_locations' => [...resource_path('a')]];`, nil, false},
		{"no returned array", `<?php return require __DIR__ . '/other.php';`, nil, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			root := t.TempDir()
			writeFiles(t, root, map[string]string{
				"vendor/livewire/livewire/composer.json": "{}",
				"config/livewire.php":                    c.config,
			})
			dirs, known := livewireLocations(root)
			if known != c.known {
				t.Fatalf("known = %v, want %v", known, c.known)
			}
			var got []string
			for _, dir := range dirs {
				rel, err := filepath.Rel(root, dir)
				if err != nil {
					t.Fatal(err)
				}
				got = append(got, filepath.ToSlash(rel))
			}
			if strings.Join(got, ",") != strings.Join(c.dirs, ",") {
				t.Fatalf("dirs = %q, want %q", got, c.dirs)
			}
		})
	}
}

func TestLivewireLocationsWithoutAConfig(t *testing.T) {
	root := t.TempDir()
	writeFiles(t, root, map[string]string{"vendor/livewire/livewire/composer.json": "{}"})
	if dirs, known := livewireLocations(root); dirs != nil || !known {
		t.Fatalf("got %q, %v; want no locations, known", dirs, known)
	}
}

func TestStudlyPath(t *testing.T) {
	if got := studlyPath("forms.input-group"); got != `Forms\InputGroup` {
		t.Fatalf("studlyPath: %s", got)
	}
}
