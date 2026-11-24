package composer

import (
	"path/filepath"
	"slices"
	"testing"
)

func TestClassFilesLongestPrefixFirst(t *testing.T) {
	entries := []AutoloadEntry{
		{Namespace: "App", Path: "/p/app"},
		{Namespace: "App\\Models", Path: "/p/models"},
		{Namespace: "Vendor\\Pkg", Path: "/v/pkg/src", IsVendor: true},
		{Path: "/v/legacy", IsVendor: true},
		{Namespace: "App", Path: "/p/helpers.php", IsFile: true},
	}
	got := ClassFiles("\\App\\Models\\User", entries)
	var paths []string
	for _, f := range got {
		paths = append(paths, f.Path())
	}
	want := []string{filepath.FromSlash("/p/models/User.php"), filepath.FromSlash("/p/app/Models/User.php")}
	if !slices.Equal(paths, want) {
		t.Errorf("got %v, want %v", paths, want)
	}
	if !slices.Equal(got[1].Parts, []string{"Models", "User.php"}) || got[1].Root != "/p/app" {
		t.Errorf("a candidate keeps its autoload root apart from the parts the class name gives: %+v", got[1])
	}
	if got := ClassFiles("Vendor\\Pkg\\Thing", entries); len(got) != 1 || !got[0].IsVendor {
		t.Errorf("vendor: %+v", got)
	}
	if got := ClassFiles("Appendix\\X", entries); len(got) != 0 {
		t.Errorf("a namespace prefix must end at a separator: %+v", got)
	}
}
