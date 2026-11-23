package laravel

import "testing"

func TestPathHelperDir(t *testing.T) {
	if dir, ok := PathHelperDir("resource_path"); !ok || dir != "resources" {
		t.Errorf("resource_path: %q, %v", dir, ok)
	}
	if dir, ok := PathHelperDir("base_path"); !ok || dir != "" {
		t.Errorf("base_path: %q, %v", dir, ok)
	}
	if _, ok := PathHelperDir("asset"); ok {
		t.Error("asset is a URL helper, not a path helper")
	}
}
