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
	want := []ClassFile{
		{Path: filepath.FromSlash("/p/models/User.php")},
		{Path: filepath.FromSlash("/p/app/Models/User.php")},
	}
	if !slices.Equal(got, want) {
		t.Errorf("got %+v, want %+v", got, want)
	}
	if got := ClassFiles("Vendor\\Pkg\\Thing", entries); len(got) != 1 || !got[0].IsVendor {
		t.Errorf("vendor: %+v", got)
	}
	if got := ClassFiles("Appendix\\X", entries); len(got) != 0 {
		t.Errorf("a namespace prefix must end at a separator: %+v", got)
	}
}
