package analyzer

import (
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	frameworksymfony "github.com/Tusk-PHP/lsp/internal/framework/symfony"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

func TestTranslationLocations(t *testing.T) {
	r := textpos.Range{Start: textpos.Position{Line: 3, Character: 4}, End: textpos.Position{Line: 3, Character: 9}}
	if loc := translationLocation(&frameworklaravel.TranslationEntry{URI: "file:///lang/en/messages.php", Range: r}); loc == nil || loc.URI != "file:///lang/en/messages.php" || loc.Range != r {
		t.Errorf("laravel: got %+v", loc)
	}
	if loc := symfonyTranslationLocation(&frameworksymfony.TranslationEntry{URI: "file:///translations/messages.en.yaml", Range: r}); loc == nil || loc.URI != "file:///translations/messages.en.yaml" || loc.Range != r {
		t.Errorf("symfony: got %+v", loc)
	}
	if translationLocation(nil) != nil || symfonyTranslationLocation(nil) != nil {
		t.Error("a missing entry has a location")
	}
}
