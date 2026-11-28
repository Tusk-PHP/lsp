package symbols

import (
	"slices"
	"testing"
)

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

func TestOnlyPECLExtensionConstantsNeedTheExtension(t *testing.T) {
	if builtinIndex("8.4").Lookup("BROTLI_COMPRESS_LEVEL_DEFAULT") != nil {
		t.Error("BROTLI_COMPRESS_LEVEL_DEFAULT needs ext-brotli, a PECL extension")
	}
	if builtinIndex("8.4", "brotli").Lookup("BROTLI_COMPRESS_LEVEL_DEFAULT") == nil {
		t.Error("BROTLI_COMPRESS_LEVEL_DEFAULT is present with ext-brotli")
	}
	if sym := builtinIndex("8.4").Lookup("IMG_PNG"); sym == nil || sym.Value != "4" {
		t.Errorf("gd ships with PHP's source, so IMG_PNG needs no extension in the profile: %+v", sym)
	}
	if builtinIndex("8.4").Lookup("CURLOPT_URL") == nil {
		t.Error("CURLOPT_URL is in PHP's default build, so it needs no extension in the profile")
	}
}

func TestABuiltinConstantWhoseTypeChangedHasEveryType(t *testing.T) {
	for _, name := range []string{"PHP_DEBUG", "PHP_ZTS"} {
		sym := builtinIndex("8.4").Lookup(name)
		if sym == nil || sym.Type != "bool|int" || sym.Value != "" {
			t.Errorf("%s is an int before PHP 8.4 and a bool since, and its value depends on the build: %+v", name, sym)
		}
	}
}

func TestTheBuiltinConstantTableIsSortedForItsLookups(t *testing.T) {
	for i := 1; i < len(generatedBuiltinConstants); i++ {
		if a, b := generatedBuiltinConstants[i-1].Name, generatedBuiltinConstants[i].Name; compareBuiltinConstantNames(a, b) >= 0 {
			t.Fatalf("%s sorts after %s, so lookups by binary search miss them", a, b)
		}
	}
}

func TestBuiltinConstantsAreFoundByPrefix(t *testing.T) {
	idx := builtinIndex("8.4")
	var eol bool
	for _, sym := range idx.SearchByPrefix("php_eo") {
		eol = eol || sym.FQN == "PHP_EOL" && sym.Kind == KindConstant
	}
	if !eol {
		t.Error("PHP_EOL is found by its prefix, ignoring case as name search does")
	}
	if got := idx.SearchByPrefix(""); !slices.ContainsFunc(got, func(s *Symbol) bool { return s.FQN == "SORT_STRING" }) {
		t.Error("an empty prefix lists the builtin constants too")
	}
}

func TestAnIndexWithoutBuiltinsHasNoBuiltinConstants(t *testing.T) {
	if sym := NewIndex().Lookup("PHP_EOL"); sym != nil {
		t.Errorf("PHP_EOL without registered builtins: %+v", sym)
	}
}

func TestAUserConstantShadowsTheBuiltinOne(t *testing.T) {
	idx := builtinIndex("8.4")
	idx.IndexFile("file:///polyfill.php", "<?php\nconst SORT_STRING = 2;\n")
	defs := idx.Definitions("SORT_STRING")
	if len(defs) != 2 || defs[0].URI != "file:///polyfill.php" || defs[1].Source != SourceBuiltin {
		t.Fatalf("the user's SORT_STRING comes first, then the builtin it shadows: %+v", defs)
	}
	idx.RemoveFile("file:///polyfill.php")
	if sym := idx.Lookup("SORT_STRING"); sym == nil || sym.Source != SourceBuiltin {
		t.Errorf("removing the user's SORT_STRING brings back the builtin: %+v", sym)
	}
}

func TestReplacingBuiltinsLooksConstantsUpForTheNewProfile(t *testing.T) {
	idx := builtinIndex("8.4")
	if idx.Lookup("DATE_ISO8601_EXPANDED") == nil {
		t.Fatal("DATE_ISO8601_EXPANDED is present on 8.4")
	}
	if err := idx.ReplaceBuiltins(BuiltinProfile{PHPVersion: "8.1"}); err != nil {
		t.Fatal(err)
	}
	if sym := idx.Lookup("DATE_ISO8601_EXPANDED"); sym != nil {
		t.Errorf("DATE_ISO8601_EXPANDED, added in PHP 8.2, is gone on 8.1: %+v", sym)
	}
}
