package symbols

import "testing"

func builtinIndex(version string, extensions ...string) *Index {
	idx := NewIndex()
	idx.RegisterBuiltinsForProfile(BuiltinProfile{PHPVersion: version, Extensions: extensions})
	return idx
}

func TestPHPsGlobalConstantsAreBuiltinSymbols(t *testing.T) {
	idx := builtinIndex("8.4")
	eol := idx.Lookup("PHP_EOL")
	if eol == nil || eol.Kind != KindConstant || eol.Source != SourceBuiltin || eol.Type != "string" {
		t.Fatalf("PHP_EOL: %+v", eol)
	}
	if eol.Value != "" {
		t.Errorf("PHP_EOL's value depends on the platform, so none is given: %q", eol.Value)
	}
	if max := idx.Lookup("PHP_INT_MAX"); max == nil || max.Type != "int" || max.Value != "9223372036854775807" {
		t.Errorf("PHP_INT_MAX: %+v", max)
	}
	if got := idx.LookupByName("PHP_EOL"); len(got) != 1 {
		t.Errorf("PHP_EOL is found by name: %v", got)
	}
	if idx.LookupFold("php_eol", nil) != nil {
		t.Error("constants are case-sensitive, so they are not found ignoring case")
	}
	for _, name := range []string{"TRUE", "FALSE", "NULL"} {
		if idx.Lookup(name) != nil {
			t.Errorf("%s is part of PHP's grammar, not a constant to look up", name)
		}
	}
}

func TestBuiltinConstantsFollowThePHPVersion(t *testing.T) {
	if idx := builtinIndex("8.1"); idx.Lookup("DATE_ISO8601_EXPANDED") != nil {
		t.Error("DATE_ISO8601_EXPANDED, added in PHP 8.2, is absent on 8.1")
	}
	if idx := builtinIndex("8.2"); idx.Lookup("DATE_ISO8601_EXPANDED") == nil {
		t.Error("DATE_ISO8601_EXPANDED is present on 8.2")
	}
	if idx := builtinIndex("7.4"); idx.Lookup("PHP_FLOAT_EPSILON") == nil || idx.Lookup("JSON_THROW_ON_ERROR") == nil {
		t.Error("constants added before the 7.4 baseline are present on 7.4")
	}
}

func TestNamespacedBuiltinConstants(t *testing.T) {
	sym := builtinIndex("8.4").Lookup(`Dom\INDEX_SIZE_ERR`)
	if sym == nil || sym.Name != "INDEX_SIZE_ERR" || sym.Value != "1" {
		t.Fatalf(`Dom\INDEX_SIZE_ERR: %+v`, sym)
	}
	if builtinIndex("8.3").Lookup(`Dom\INDEX_SIZE_ERR`) != nil {
		t.Error(`Dom\INDEX_SIZE_ERR, added in PHP 8.4, is absent on 8.3`)
	}
}

func TestExtensionConstantsNeedTheExtension(t *testing.T) {
	if builtinIndex("8.4").Lookup("IMG_PNG") != nil {
		t.Error("IMG_PNG needs ext-gd")
	}
	if sym := builtinIndex("8.4", "gd").Lookup("IMG_PNG"); sym == nil || sym.Value != "4" {
		t.Errorf("IMG_PNG with ext-gd: %+v", sym)
	}
	if builtinIndex("8.4").Lookup("CURLOPT_URL") == nil {
		t.Error("CURLOPT_URL is in PHP's default build, so it needs no extension in the profile")
	}
}
