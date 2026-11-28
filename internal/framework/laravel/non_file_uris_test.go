package laravel

import (
	"path/filepath"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestLaravelFileFeaturesAnswerNothingForADocumentThatIsNotAFile(t *testing.T) {
	if isLaravelRouteFile("untitled:/p/routes/web.php") {
		t.Error("a document that is not a file is no route file")
	}
	d := NewViewData("/p")
	if got := d.ViewName("untitled:/p/resources/views/home.blade.php"); got != "" {
		t.Errorf("a document that is not a file is no view, got %q", got)
	}
}

func TestViewTypesReadsAViewFromDiskWhenNothingElseHasIt(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "resources", "views", "home.blade.php")
	writeFile(t, path, "<p>{{ $title }}</p>")
	if got := (&ViewTypes{}).source(symbols.PathToURI(path)); got != "<p>{{ $title }}</p>" {
		t.Errorf("got %q", got)
	}
	if got := (&ViewTypes{}).source("untitled:home.blade.php"); got != "" {
		t.Errorf("a document that is not a file has nothing on disk, got %q", got)
	}
}

func TestAPackageProviderThatIsNotAFileMayRegisterAnyComponent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "vendor", "composer", "installed.json"), `{"packages": [
		{"name": "acme/scratch", "extra": {"laravel": {"providers": ["Acme\\ScratchServiceProvider", "Acme\\MoneyServiceProvider"]}}}
	]}`)
	money := filepath.Join(root, "vendor", "acme", "money", "MoneyServiceProvider.php")
	moneySource := "<?php\nnamespace Acme;\nuse Illuminate\\Support\\Facades\\Blade;\nclass MoneyServiceProvider {\n    public function boot(): void { Blade::directive('money', fn () => ); }\n}\n"
	writeFile(t, money, moneySource)
	idx := symbols.NewIndex()
	idx.IndexFile(symbols.PathToURI(money), moneySource)
	idx.IndexFile("untitled:ScratchServiceProvider.php", "<?php\nnamespace Acme;\nclass ScratchServiceProvider {}\n")
	r := NewProviders(root, idx).read()
	if !r.UnknownComponents {
		t.Error("a provider that can't be read from a file may register anything")
	}
	if _, ok := r.Directives["money"]; !ok {
		t.Errorf("a provider in a file is read: %v", r.Directives)
	}
}
