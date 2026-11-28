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

const helperEdgeFixture = `<?php
namespace App {
use Illuminate\Support\Facades\Validator;

Validator::make($data, ['top' => Rules::topRules(), 'self' => self::nothing()]);

trait SharedRules
{
    public function sharedRules() { return ['string']; }
    abstract public function declared();
}

class Form
{
    use SharedRules;

    protected $rules = ['title' => 'required'];

    public function save($request, $field)
    {
        $this->validateWithBag('bag', $request, ['bagged' => $this->baggedRules()]);
        $this->validateOnly($field, ['only' => $this->onlyRules()]);
        $this->validate(['livewire' => $this->livewireRules(), 'shared' => $this->sharedRules()], $this->messages());
        $this->validate($request, ['controller' => $this->controllerRules(), 'self' => self::selfRules()]);
        validator($data, ['helper' => $this->validatorRules()]);
        $request->validate($this->dynamicRules);
        $request->validate([, 'a' => 'required', 'b' => $other->rules(), 'c' => $class::rules(), 'd' => parent::rules()]);
        $request->validate(...$arguments);
        $request->validate(['alias' => $this->aliasRules(), 'list' => $this->listRules()]);
        $request->validate($this->keyedListRules());
        $request->validate([...$this->spreadRules(), 'x' => 'string']);
        $fn = function () use ($request) {
            $request->validate(['closure' => $this->closureRules()]);
            return ['ignored' => $this->ignoredRules()];
        };
        function inner($request) { $request->validate(['inner' => 'string']); }
    }

    public function rules() { return ['r' => $this->notARequestsRules()]; }

    public function baggedRules() { return ['string']; }
    public function onlyRules() { return ['string']; }
    public function livewireRules() { return ['string']; }
    public function controllerRules() { return ['string']; }
    public static function selfRules() { return ['string']; }
    public function validatorRules() { return ['string']; }
    public function closureRules() { return ['string']; }
    public function messages() { return []; }
    public function varRules() { return $rules; }

    public function aliasRules() { return $this->aliasedRules(); }
    public function aliasedRules() { return ['string']; }
    public function listRules() { return [, ...$this->listedRules()]; }
    public function listedRules() { return ['string']; }
    public function keyedListRules() { return ['k' => 'string', ...$this->keyedSpreadRules()]; }
    public function keyedSpreadRules() { return ['s' => 'string']; }
    public function spreadRules() { return ['t' => 'string']; }

    public function loopA() { return ['a' => $this->loopB()]; }
    public function loopB() { return ['b' => $this->loopA()]; }
}
}
`

func TestValidationHelpersFollowEveryCallForm(t *testing.T) {
	idx := symbols.NewIndex()
	idx.IndexFileWithSource("file:///app/Form.php", helperEdgeFixture, symbols.SourceProject)
	h := NewValidationHelpers()
	h.IndexFile("file:///app/Form.php", "<?php\nclass Form { public function rules() { return ['a' => $this->stale()]; } }\n")
	h.IndexFile("file:///app/Form.php", helperEdgeFixture)
	h.IndexFile("file:///app/Plain.php", "<?php\necho 1;\n")
	for _, tc := range []struct {
		class, method string
		want          RulesForm
	}{
		{`App\Rules`, "topRules", FieldRules},
		{`App\SharedRules`, "sharedRules", FieldRules},
		{`App\Form`, "baggedRules", FieldRules},
		{`App\Form`, "onlyRules", FieldRules},
		{`App\Form`, "livewireRules", FieldRules},
		{`App\Form`, "controllerRules", FieldRules},
		{`App\Form`, "selfRules", FieldRules},
		{`App\Form`, "validatorRules", FieldRules},
		{`App\Form`, "closureRules", FieldRules},
		{`App\Form`, "aliasRules", FieldRules},
		{`App\Form`, "aliasedRules", FieldRules},
		{`App\Form`, "listRules", FieldRules},
		{`App\Form`, "listedRules", FieldRules},
		{`App\Form`, "keyedListRules", RulesArray},
		{`App\Form`, "keyedSpreadRules", RulesArray},
		{`App\Form`, "spreadRules", RulesArray},
		{`App\Form`, "notARequestsRules", 0},
		{`App\Form`, "messages", 0},
		{`App\Form`, "varRules", 0},
		{`App\Form`, "ignoredRules", 0},
		{`App\Form`, "loopA", 0},
		{`App\Form`, "nothing", 0},
		{`App\Form`, "stale", 0},
		{`App\Other`, "baggedRules", 0},
		{`App\Other`, "aliasedRules", 0},
		{"", "livewireRules", 0},
	} {
		form, ok := h.Form(idx, tc.class, tc.method)
		if ok != (tc.want != 0) || form != tc.want {
			t.Errorf("%s::%s: %v %v, want %v", tc.class, tc.method, form, ok, tc.want)
		}
	}
	var none *ValidationHelpers
	none.IndexFile("file:///app/Form.php", helperEdgeFixture)
}
