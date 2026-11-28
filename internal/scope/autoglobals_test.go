package scope

import (
	"slices"
	"testing"
)

func TestAutoGlobals(t *testing.T) {
	want := []string{"$GLOBALS", "$_COOKIE", "$_ENV", "$_FILES", "$_GET", "$_POST", "$_REQUEST", "$_SERVER", "$_SESSION"}
	got := AutoGlobals()
	if !slices.Equal(got, want) {
		t.Fatalf("AutoGlobals() = %v, want %v", got, want)
	}
	got[0] = "$changed"
	for _, name := range want {
		if !IsAutoGlobal(name) {
			t.Errorf("IsAutoGlobal(%q) = false after a caller changed its copy", name)
		}
	}
	for _, name := range []string{"$changed", "$this", "$argv", "$_get", "GLOBALS"} {
		if IsAutoGlobal(name) {
			t.Errorf("IsAutoGlobal(%q) = true", name)
		}
	}
}
