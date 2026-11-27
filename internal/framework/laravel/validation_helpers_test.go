package laravel

import (
	"testing"

	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const helperFixture = `<?php
namespace App\Services;

use App\Rules\PostRules;
use Illuminate\Support\Facades\Validator;

class Importer
{
    use \App\Concerns\NameRules;

    public function run(array $data, $request)
    {
        Validator::make($data, ['title' => self::titleRules(), 'body' => PostRules::body(), ...$this->rules()], ['title.required' => $this->messages()]);
        $request->validate(['slug' => static::slugRules()]);
        $this->validate($request, $this->authorRules());
        validator($data, $this->all());
    }

    private static function titleRules() { return 'required|max:5'; }
    private static function slugRules() { return ['alpha_dash']; }
    public function rules() { return ['name' => $this->nameRules()]; }
    public function authorRules() { return ['author' => $this->authorField()]; }
    public function authorField() { return ['string']; }
    public function all() { return [...$this->rules(), 'extra' => 'string']; }
    public function messages() { return ['x' => 'y']; }
    public function unused() { return ['required']; }
}

class PostRequest extends \Illuminate\Foundation\Http\FormRequest
{
    public function rules() { return ['tags' => $this->tagRules()]; }
    public function tagRules() { return ['array']; }
}

class NotARequest
{
    public function rules() { return ['tags' => $this->tagRules()]; }
    public function tagRules() { return ['array']; }
}
`

func TestValidationHelpers(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///vendor/FormRequest.php", "<?php\nnamespace Illuminate\\Foundation\\Http;\nclass FormRequest {}\n", symbols.SourceVendor)
	idx.IndexFileWithSource("file:///app/NameRules.php", "<?php\nnamespace App\\Concerns;\ntrait NameRules { public function nameRules() { return ['string']; } }\n", symbols.SourceProject)
	idx.IndexFileWithSource("file:///app/Importer.php", helperFixture, symbols.SourceProject)
	h := NewValidationHelpers()
	h.IndexFile("file:///app/Importer.php", helperFixture)
	for _, tc := range []struct {
		class, method string
		want          RulesForm
	}{
		{`App\Services\Importer`, "titleRules", FieldRules},
		{`App\Services\Importer`, "slugRules", FieldRules},
		{`App\Rules\PostRules`, "body", FieldRules},
		{`App\Services\Importer`, "rules", RulesArray},
		{`App\Services\Importer`, "authorRules", RulesArray},
		{`App\Services\Importer`, "authorField", FieldRules},
		{`App\Services\Importer`, "all", RulesArray},
		{`App\Concerns\NameRules`, "nameRules", FieldRules},
		{`App\Services\PostRequest`, "tagRules", FieldRules},
		{`App\Services\Importer`, "messages", 0},
		{`App\Services\Importer`, "unused", 0},
		{`App\Services\NotARequest`, "tagRules", 0},
		{`App\Services\Other`, "titleRules", 0},
	} {
		form, ok := h.Form(idx, tc.class, tc.method)
		if ok != (tc.want != 0) || form != tc.want {
			t.Errorf("%s::%s: %v %v, want %v", tc.class, tc.method, form, ok, tc.want)
		}
	}
	h.IndexFile("file:///app/Importer.php", "")
	if _, ok := h.Form(idx, `App\Services\Importer`, "titleRules"); ok {
		t.Error("a removed file's helpers go")
	}
	var none *ValidationHelpers
	if _, ok := none.Form(idx, `App\Services\Importer`, "titleRules"); ok {
		t.Error("a nil set has none")
	}
}
