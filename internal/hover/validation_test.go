package hover

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

func TestHoverValidationRule(t *testing.T) {
	root := t.TempDir()
	files := map[string]string{
		"vendor/Request.php": "<?php\nnamespace Illuminate\\Http;\nclass Request {}\n",
		"vendor/ValidatesAttributes.php": `<?php
namespace Illuminate\Validation\Concerns;
trait ValidatesAttributes
{
    /**
     * Validate the size of an attribute is less than or equal to a maximum value.
     *
     * @param  array<int, int|string>  $parameters
     */
    public function validateMax($attribute, $value, $parameters) { $this->requireParameterCount(1, $parameters, 'max'); }
    public function validateRequired($attribute, $value) {}
}
`,
		"app/Providers/AppServiceProvider.php": `<?php
namespace App\Providers;
use Illuminate\Support\Facades\Validator;
class AppServiceProvider
{
    public function boot(): void
    {
        Validator::extend('phone_number', fn () => true);
    }
}
`,
	}
	idx := symbols.NewIndex()
	ext := frameworklaravel.NewValidationExtensions()
	for rel, src := range files {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		idx.IndexFileWithSource(symbols.PathToURI(path), src, symbols.SourceProject)
		ext.IndexFile(symbols.PathToURI(path), src)
	}
	idx.MarkReady()
	read := func(uri string) string {
		data, err := os.ReadFile(symbols.URIToPath(uri))
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		return string(data)
	}
	p := NewProvider(idx, nil, "laravel")
	p.SetValidationRules(frameworklaravel.NewValidationRules(idx, ext, read))

	src := `<?php
use Illuminate\Http\Request;

function store(Request $request)
{
    $request->validate(['title' => 'required|max:255', 'phone' => ['phone_number'], 'x' => 'unknwn']);
}
`
	at := func(needle string) protocol.Position {
		i := strings.Index(src, needle)
		return protocol.Position{Line: strings.Count(src[:i], "\n"), Character: i - strings.LastIndex(src[:i], "\n") - 1 + 1}
	}
	for needle, want := range map[string]string{
		"max:":         "```php\n(validation rule) max:value\n```\n\nValidate the size of an attribute is less than or equal to a maximum value.",
		"255":          "```php\n(validation rule) max:value\n```\n\nValidate the size of an attribute is less than or equal to a maximum value.",
		"required|":    "```php\n(validation rule) required\n```",
		"phone_number": "```php\n(validation rule) phone_number\n```\n\nCustom rule, registered in AppServiceProvider.php:8",
	} {
		h := p.GetHover("file:///store.php", src, at(needle))
		if h == nil || h.Contents.Value != want {
			t.Errorf("hover on %q: %+v\nwant %q", needle, h, want)
		}
	}
	if h := p.GetHover("file:///store.php", src, at("max:")); h == nil || h.Range == nil || h.Range.End.Character-h.Range.Start.Character != len("max") {
		t.Errorf("hover range covers the rule's name: %+v", h)
	}
	if h := p.GetHover("file:///store.php", src, at("unknwn")); h != nil {
		t.Errorf("no hover on an unknown rule: %+v", h)
	}
	if h := p.GetHover("file:///store.php", src, at("'title'")); h != nil && strings.Contains(h.Contents.Value, "validation rule") {
		t.Errorf("a rules array key is no rule: %+v", h)
	}
	p.framework = "none"
	if h := p.GetHover("file:///store.php", src, at("max:")); h != nil && strings.Contains(h.Contents.Value, "validation rule") {
		t.Errorf("outside a Laravel app a rule string is a string: %+v", h)
	}
}
