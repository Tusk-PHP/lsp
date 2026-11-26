package symbols

import "testing"

func TestFunctionAvailabilityComesFromPHPsBuilds(t *testing.T) {
	cases := []struct {
		name string
		want BuiltinAvailability
	}{
		{"str_contains", BuiltinAvailability{Since: "8.0"}},
		{"array_is_list", BuiltinAvailability{Since: "8.1"}},
		{"mb_str_pad", BuiltinAvailability{Since: "8.3"}},
		{"json_validate", BuiltinAvailability{Since: "8.3"}},
		{"apcu_fetch", BuiltinAvailability{Extension: "apcu"}},
		{"brotli_compress", BuiltinAvailability{Extension: "brotli"}},
	}
	for _, c := range cases {
		if got, ok := LookupAvailability(c.name); !ok || got != c.want {
			t.Errorf("%s: got %+v %v, want %+v", c.name, got, ok, c.want)
		}
	}
	for _, name := range []string{"strlen", "exit", "die", "opcache_get_status", "intl_get_error_code"} {
		if got, ok := LookupAvailability(name); ok {
			t.Errorf("%s needs no version or extension: %+v", name, got)
		}
	}
}

func TestRemovedFunctionsComeFromPHPsDefaultBuilds(t *testing.T) {
	for _, name := range []string{"each", "create_function", "money_format", "get_magic_quotes_gpc"} {
		got, ok := LookupAvailability(name)
		if !ok || got != (BuiltinAvailability{Removed: "8.0"}) {
			t.Errorf("%s: got %+v %v, want removed in 8.0", name, got, ok)
		}
		if !got.InVersion("7.4") || got.InVersion("8.0") || got.InVersion("8.4") {
			t.Errorf("%s is there on 7.4 only", name)
		}
	}
	for _, name := range []string{"chroot", "ftp_connect", "mb_strlen"} {
		if got, ok := LookupAvailability(name); ok {
			t.Errorf("%s is only missing from some builds, not removed: %+v", name, got)
		}
	}
	since := BuiltinAvailability{Since: "8.1"}
	if since.InVersion("8.0") || !since.InVersion("8.1") || since.RemovedBy("8.6") {
		t.Errorf("%+v is there from 8.1 on", since)
	}
}

func TestPECLFunctionsNeedTheirExtension(t *testing.T) {
	if newIndexForProfile("8.4").Lookup("brotli_compress") != nil {
		t.Error("brotli_compress needs ext-brotli")
	}
	if newIndexForProfile("8.4", "brotli").Lookup("brotli_compress") == nil {
		t.Error("brotli_compress is there with ext-brotli")
	}
	if newIndexForProfile("8.3").Lookup("array_find") != nil || newIndexForProfile("8.4").Lookup("array_find") == nil {
		t.Error("array_find comes with PHP 8.4")
	}
}

func newIndexForProfile(version string, extensions ...string) *Index {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: version, Extensions: extensions})
	return idx
}

// TestLookupAvailabilityUnknown verifies that a name not present in either map
// returns the zero value and ok=false.
func TestLookupAvailabilityUnknown(t *testing.T) {
	got, ok := LookupAvailability("definitely_not_a_php_function_xyz")
	if ok {
		t.Fatalf("LookupAvailability(unknown) returned ok=true, want false")
	}
	zero := BuiltinAvailability{}
	if got != zero {
		t.Errorf("LookupAvailability(unknown) = %+v, want zero value", got)
	}
}

func TestVersionDeltaAddsToEarlierClass(t *testing.T) {
	hasMember := func(version, member string) bool {
		idx := NewIndex()
		idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: version})
		for _, m := range idx.GetClassMembers("ReflectionClass") {
			if m.Name == member {
				return true
			}
		}
		return false
	}
	if !hasMember("8.4", "getName") || !hasMember("8.4", "newLazyGhost") {
		t.Error("PHP 8.4 should have ReflectionClass's earlier members and its lazy object ones")
	}
	if hasMember("8.3", "newLazyGhost") {
		t.Error("PHP 8.3 shouldn't have lazy object members")
	}
}
