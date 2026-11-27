package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writeFixture(t *testing.T, root, rel, content string) string {
	t.Helper()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func translationLabels(r *TranslationResolver, partial string) []string {
	var labels []string
	for _, item := range r.Complete(&TranslationStringContext{Partial: partial}) {
		labels = append(labels, item.Label)
	}
	return labels
}

func TestTranslationKeysAreReadOnceUntilTheirFilesChange(t *testing.T) {
	root := setupTranslationFixture(t)
	r := NewTranslationResolver(root)
	if labels := translationLabels(r, "messages."); !slices.Contains(labels, "messages.welcome") {
		t.Fatalf("the fixture's keys are completed: %v", labels)
	}

	en := writeFixture(t, root, "lang/en/messages.php", "<?php\nreturn [\n    'welcome' => 'Welcome',\n    'goodbye' => 'Goodbye',\n];\n")
	if labels := translationLabels(r, "messages."); slices.Contains(labels, "messages.goodbye") {
		t.Fatalf("keys are kept between requests, not read again: %v", labels)
	}
	if loc := r.Definition("messages.goodbye"); loc != nil {
		t.Fatalf("definition reads the kept keys too: %v", loc)
	}

	r.FilesChanged([]string{filepath.Join(root, "app", "Models", "User.php"), filepath.Join(root, "resources", "views", "home.blade.php")})
	if labels := translationLabels(r, "messages."); slices.Contains(labels, "messages.goodbye") {
		t.Fatalf("a change outside the translations keeps the keys: %v", labels)
	}

	r.FilesChanged([]string{en})
	if labels := translationLabels(r, "messages."); !slices.Contains(labels, "messages.goodbye") {
		t.Fatalf("a changed translation file is read again: %v", labels)
	}
	if loc := r.Definition("messages.goodbye"); loc == nil {
		t.Fatal("a new key is found by definition")
	}
}

func TestTranslationKeysFollowANewJSONFileAndALocaleChange(t *testing.T) {
	root := setupTranslationFixture(t)
	r := NewTranslationResolver(root)
	if loc := r.Definition("messages.welcome"); loc == nil || filepath.Base(filepath.Dir(loc.URI)) != "es" {
		t.Fatalf("the configured locale es is preferred: %v", loc)
	}

	fr := writeFixture(t, root, "resources/lang/fr.json", "{\n  \"Bonjour\": \"Bonjour\"\n}\n")
	r.FilesChanged([]string{fr})
	if labels := translationLabels(r, "Bon"); !slices.Contains(labels, "Bonjour") {
		t.Fatalf("a JSON file created under resources/lang is read: %v", labels)
	}

	app := writeFixture(t, root, "config/app.php", "<?php\nreturn [\n    'locale' => 'en',\n];\n")
	r.FilesChanged([]string{app})
	if loc := r.Definition("messages.welcome"); loc == nil || filepath.Base(filepath.Dir(loc.URI)) != "en" {
		t.Fatalf("a new locale in config/app.php is preferred: %v", loc)
	}
}

func TestTranslationKeysAreForgottenWhenTheirDirectoryGoes(t *testing.T) {
	root := setupTranslationFixture(t)
	r := NewTranslationResolver(root)
	if labels := translationLabels(r, "messages."); len(labels) == 0 {
		t.Fatal("the fixture's keys are completed")
	}
	if err := os.RemoveAll(filepath.Join(root, "lang")); err != nil {
		t.Fatal(err)
	}
	r.FilesChanged([]string{root})
	if labels := translationLabels(r, ""); len(labels) != 0 {
		t.Fatalf("a deleted directory holding lang/ takes its keys along: %v", labels)
	}
}

func TestViewNamesFollowViewsCreatedAndDeleted(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "resources/views/welcome.blade.php", "<h1>Hi</h1>")
	v := NewViews(root)
	if names := v.ViewNames(""); !slices.Equal(names, []string{"welcome"}) {
		t.Fatalf("the fixture's views are listed: %v", names)
	}

	created := writeFixture(t, root, "resources/views/orders/show.blade.php", "<p>Order</p>")
	if names := v.ViewNames(""); slices.Contains(names, "orders.show") {
		t.Fatalf("views are listed once, not on every request: %v", names)
	}
	v.FilesChanged([]string{filepath.Join(root, "app", "Http", "Kernel.php")})
	if names := v.ViewNames(""); slices.Contains(names, "orders.show") {
		t.Fatalf("a change outside the views keeps the list: %v", names)
	}
	v.FilesChanged([]string{created})
	if names := v.ViewNames(""); !slices.Contains(names, "orders.show") {
		t.Fatalf("a created view is listed: %v", names)
	}

	if err := os.RemoveAll(filepath.Join(root, "resources", "views", "orders")); err != nil {
		t.Fatal(err)
	}
	v.FilesChanged([]string{filepath.Join(root, "resources", "views", "orders")})
	if names := v.ViewNames(""); slices.Contains(names, "orders.show") {
		t.Fatalf("a deleted view directory takes its views along: %v", names)
	}
	if _, ok := v.ViewURI("orders.show"); ok {
		t.Fatal("a deleted view has no file")
	}
}

func TestViewsKnownFollowsTheViewConfig(t *testing.T) {
	root := t.TempDir()
	writeFixture(t, root, "resources/views/welcome.blade.php", "<h1>Hi</h1>")
	v := NewViews(root)
	if !v.ViewsKnown() {
		t.Fatal("an app with only resources/views knows all its views")
	}
	cfg := writeFixture(t, root, "config/view.php", "<?php\nreturn ['paths' => [resource_path('views'), base_path('themes')]];\n")
	v.FilesChanged([]string{cfg})
	if v.ViewsKnown() {
		t.Fatal("a view path added in config/view.php is followed")
	}
}
