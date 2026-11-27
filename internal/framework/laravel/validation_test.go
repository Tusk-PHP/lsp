package laravel

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Tusk-PHP/lsp/internal/parser"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const validatesAttributesSource = `<?php
namespace Illuminate\Validation\Concerns;

trait ValidatesAttributes
{
    /**
     * Validate that a required attribute exists.
     */
    public function validateRequired($attribute, $value) { return true; }

    /**
     * Validate that an attribute exists when another attribute has a given value.
     *
     * @param  mixed  $parameters
     */
    public function validateRequiredIf($attribute, $value, $parameters)
    {
        $this->requireParameterCount(2, $parameters, 'required_if');
        return true;
    }

    public function validateMax($attribute, $value, $parameters)
    {
        $this->requireParameterCount(1, $parameters, 'max');
        return true;
    }

    public function validateIn($attribute, $value, $parameters) { return true; }

    public function validateMacAddress($attribute, $value) { return true; }

    /** "Break" on first validation fail. */
    public function validateBail() { return true; }

    public function requireParameterCount($count, $parameters, $rule) {}
}
`

const validationProvider = `<?php

namespace App\Providers;

use Illuminate\Support\Facades\Validator;
use Illuminate\Support\ServiceProvider;

class AppServiceProvider extends ServiceProvider
{
    public function boot(): void
    {
        Validator::extend('phone_number', fn () => true);
        Validator::extendImplicit('Postcode', fn () => true);
        $this->app['validator']->extend('iban', fn () => true);
        app('validator')->extendDependent('greater_than_field', fn () => true);
        $this->app->extend(Foo::class, fn ($foo) => $foo);
        Cache::extend('mongo', fn () => null);
        Validator::extend($name, fn () => true);
    }
}
`

func TestValidationRuleKey(t *testing.T) {
	for _, same := range [][2]string{
		{"required_if", "requiredIf"},
		{"required_if", " Required-If "},
		{"mac_address", "MacAddress"},
	} {
		if ValidationRuleKey(same[0]) != ValidationRuleKey(same[1]) {
			t.Errorf("%q and %q are the same rule", same[0], same[1])
		}
	}
	if ValidationRuleKey("required") == ValidationRuleKey("required_if") {
		t.Error("required is not required_if")
	}
}

func TestSnake(t *testing.T) {
	for in, want := range map[string]string{
		"MacAddress": "mac_address", "Ipv4": "ipv4", "RequiredIf": "required_if", "Max": "max",
		"FullName": "full_name", "ID": "i_d", "BlogPost": "blog_post", "Category": "category", "ÄrgerLog": "ärger_log",
	} {
		if got := Snake(in); got != want {
			t.Errorf("Snake(%q) = %q, want %q", in, got, want)
		}
	}
}

func extensionNames(e *ValidationExtensions) string {
	_, regs, _ := e.snapshot()
	names := make([]string, 0, len(regs))
	for _, r := range regs {
		names = append(names, r.Name)
	}
	return strings.Join(slices.Sorted(slices.Values(names)), " ")
}

func TestValidationExtensions(t *testing.T) {
	e := NewValidationExtensions()
	e.IndexFile("file:///app/Providers/AppServiceProvider.php", validationProvider)
	if got := extensionNames(e); got != "Postcode greater_than_field iban phone_number" {
		t.Errorf("validation rules: %q", got)
	}
	_, regs, _ := e.snapshot()
	for _, reg := range regs {
		if reg.Name == "phone_number" && (reg.Line != 11 || reg.URI != "file:///app/Providers/AppServiceProvider.php") {
			t.Errorf("phone_number registered at %+v", reg)
		}
	}

	e.IndexFile("file:///bootstrap/app.php", "<?php\nuse Illuminate\\Support\\Facades\\Validator as V;\nV::extend('slug', fn () => true);\n\\Illuminate\\Support\\Facades\\Validator::extendImplicit('vat_id', fn () => true);\n")
	if got := extensionNames(e); got != "Postcode greater_than_field iban phone_number slug vat_id" {
		t.Errorf("rules registered anywhere count: %q", got)
	}

	before := e.currentVersion()
	e.IndexFile("file:///bootstrap/app.php", "<?php\nuse Illuminate\\Support\\Facades\\Validator as V;\nV::extend('slug', fn () => true);\n\\Illuminate\\Support\\Facades\\Validator::extendImplicit('vat_id', fn () => true);\n")
	if e.currentVersion() != before {
		t.Error("indexing a file again unchanged keeps the catalog")
	}
	e.IndexFile("file:///bootstrap/app.php", "<?php\nValidator::extend('isbn', fn () => true);\n")
	if got := extensionNames(e); got != "Postcode greater_than_field iban isbn phone_number" || e.currentVersion() == before {
		t.Errorf("a file's rules are replaced when it changes: %q", got)
	}
	e.IndexFile("file:///bootstrap/app.php", "")
	if got := extensionNames(e); got != "Postcode greater_than_field iban phone_number" {
		t.Errorf("a removed file's rules go: %q", got)
	}

	e.IndexFile("file:///app/Other.php", `<?php
namespace App;
class Other
{
    public function boot($manager)
    {
        Validator::extend('in_namespace', fn () => true);
        $manager->extend('driver', fn () => null);
        // Validator::extend('commented', fn () => true);
        $x = "Validator::extend('in_a_string')";
    }
}
`)
	if got := extensionNames(e); got != "Postcode greater_than_field iban phone_number" {
		t.Errorf("App\\Validator, managers, comments and strings register nothing: %q", got)
	}
	var none *ValidationExtensions
	none.IndexFile("file:///x.php", validationProvider)
	if v, regs, unread := none.snapshot(); v != 0 || regs != nil || unread {
		t.Error("a nil set has nothing")
	}
}

func validationFixture(t *testing.T) *ValidationRules {
	t.Helper()
	root := t.TempDir()
	writeFiles(t, root, map[string]string{
		"composer.json":                        `{"autoload": {"psr-4": {"App\\": "app/"}}}`,
		"app/Providers/AppServiceProvider.php": validationProvider,
		"vendor/laravel/framework/src/Illuminate/Validation/Concerns/ValidatesAttributes.php": validatesAttributesSource,
	})
	idx := symbols.NewIndex()
	trait := filepath.Join(root, "vendor/laravel/framework/src/Illuminate/Validation/Concerns/ValidatesAttributes.php")
	idx.IndexFileWithSource(symbols.PathToURI(trait), validatesAttributesSource, symbols.SourceVendor)
	idx.MarkReady()
	read := func(uri string) string {
		data, err := os.ReadFile(symbols.URIToPath(uri))
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		return string(data)
	}
	ext := NewValidationExtensions()
	ext.IndexFile(symbols.PathToURI(filepath.Join(root, "app/Providers/AppServiceProvider.php")), validationProvider)
	return NewValidationRules(idx, ext, read)
}

func TestValidationRulesCatalog(t *testing.T) {
	v := validationFixture(t)
	var names []string
	for _, r := range v.All() {
		names = append(names, r.Name)
	}
	want := "Postcode bail greater_than_field iban in mac_address max phone_number required required_if"
	if got := strings.Join(names, " "); got != want {
		t.Errorf("rules:\n got %q\nwant %q", got, want)
	}
	ri, ok := v.Lookup("requiredIf")
	if !ok || ri.Name != "required_if" || ri.Required != 2 || !ri.TakesParameters ||
		ri.Signature() != "required_if:anotherfield,value,…" ||
		ri.Description() != "Validate that an attribute exists when another attribute has a given value." {
		t.Errorf("required_if: %+v", ri)
	}
	if r, _ := v.Lookup("required"); r.TakesParameters || r.Signature() != "required" || r.Required != 0 {
		t.Errorf("required: %+v", r)
	}
	if r, _ := v.Lookup("in"); r.Required != 0 || r.Signature() != "in:value,…" {
		t.Errorf("in: %+v", r)
	}
	if r, _ := v.Lookup("phone_number"); r.Registered == nil || r.Description() != "Custom rule, registered in AppServiceProvider.php:12" {
		t.Errorf("phone_number: %+v", r)
	}
	if _, ok := v.Lookup("requierd"); ok {
		t.Error("requierd is no rule")
	}
	if first, second := v.All(), v.All(); &first[0] != &second[0] {
		t.Error("the catalog is read once until its sources change")
	}
}

func TestValidationRulesCatalogFollowsTheTrait(t *testing.T) {
	v := validationFixture(t)
	if _, ok := v.Lookup("max"); !ok {
		t.Fatal("max")
	}
	trait := v.index.Lookup(validatesAttributes)
	v.index.IndexFileWithSource(trait.URI, strings.Replace(validatesAttributesSource, "validateMax(", "validateMaximum(", 1), symbols.SourceVendor)
	if _, ok := v.Lookup("max"); ok {
		t.Error("max is gone once the trait is indexed again")
	}
	if _, ok := v.Lookup("maximum"); !ok {
		t.Error("maximum comes with it")
	}
}

type ruleFixture struct {
	classes map[string]string
	exprs   map[string]string
	parents map[string][]string
}

func (f ruleFixture) syntax(src string) ValidationSyntax {
	return ValidationSyntax{
		ClassName: func(name string) string {
			if fqn, ok := f.classes[name]; ok {
				return fqn
			}
			return strings.TrimPrefix(name, `\`)
		},
		ExprClass: func(start, end uint32) string { return f.exprs[src[start:end]] },
		IsSubtype: func(child, parent string) bool {
			if child == parent {
				return true
			}
			for _, p := range f.parents[child] {
				if p == parent {
					return true
				}
			}
			return false
		},
	}
}

var rulesFixture = ruleFixture{
	classes: map[string]string{
		"Validator":   validatorFacade,
		"FormRequest": formRequest,
		"Request":     httpRequest,
		"Unrelated":   `App\Unrelated`,
	},
	exprs: map[string]string{
		"$request":  httpRequest,
		"request()": httpRequest,
		"$this":     `App\Http\Controllers\Controller`,
		"$other":    `App\Service`,
	},
	parents: map[string][]string{
		`App\Http\Controllers\Controller`: {validatesRequests},
	},
}

func ruleStrings(t *testing.T, src string) []string {
	t.Helper()
	var out []string
	ValidationRuleStrings(src, phpparser.Parse(src, token.Latest), rulesFixture.syntax(src), func(r RuleString) {
		var segs []string
		for _, s := range r.Segments(src) {
			segs = append(segs, s.Name(src)+"["+src[s.NameEnd:s.End]+"]")
		}
		form := "array"
		if r.Piped {
			form = "piped"
		}
		out = append(out, form+":"+strings.Join(segs, ","))
	})
	return out
}

func TestValidationRuleStringsInEachCallForm(t *testing.T) {
	for name, tc := range map[string]struct{ src, want string }{
		"request validate": {`$request->validate(['a' => 'required|max:255', 'b' => ['in:x,y', 'nullable']], ['a.required' => 'Needed']);`,
			"piped:required[],max[:255] | array:in[:x,y] | array:nullable[]"},
		"request helper":     {`request()->validate(['a' => 'email']);`, "piped:email[]"},
		"validateWithBag":    {`$request->validateWithBag('bag', ['a' => 'string']);`, "piped:string[]"},
		"controller":         {`$this->validate($request, ['a' => 'integer'], ['a.integer' => 'Whole numbers']);`, "piped:integer[]"},
		"controller bag":     {`$this->validateWithBag('bag', $request, ['a' => 'date']);`, "piped:date[]"},
		"named rules":        {`$this->validate(request: $request, rules: ['a' => 'url']);`, "piped:url[]"},
		"validator facade":   {`Validator::make($data, ['a' => 'required_if:b,1']);`, "piped:required_if[:b,1]"},
		"validator validate": {`\Illuminate\Support\Facades\Validator::validate($data, ['a' => 'size:2']);`, "piped:size[:2]"},
		"validator helper":   {`validator($data, ['a' => 'uuid']);`, "piped:uuid[]"},
		"regex in array":     {`$request->validate(['a' => ['regex:/^(a|b)$/', 'required']]);`, "array:regex[:/^(a|b)$/] | array:required[]"},
		"regex piped":        {`$request->validate(['a' => 'regex:/^(a|b)$/']);`, "piped:regex[:/^(a],b)$/[]"},
		"other receiver":     {`$other->validate(['a' => 'nope']);`, ""},
		"unknown receiver":   {`$unknown->validate(['a' => 'nope']);`, ""},
		"other facade":       {`Unrelated::make($data, ['a' => 'nope']);`, ""},
		"dynamic":            {`$request->validate(['a' => 'required|' . $max, 'b' => "max:$n", 'c' => $rules, ...$more]);`, ""},
		"rules variable":     {`$request->validate($rules, ['a' => 'Message']);`, ""},
		"list is no rules":   {`$request->validate(['required']);`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			got := strings.Join(ruleStrings(t, "<?php\n"+tc.src), " | ")
			if got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestValidationRuleStringsInFormRequestRules(t *testing.T) {
	src := `<?php
class StorePost extends FormRequest
{
    public function rules(): array
    {
        $messages = fn () => ['x' => 'not a rule'];
        if ($this->isMethod('post')) {
            return ['title' => 'required|string'];
        }
        return ['title' => ['sometimes']];
    }

    public function messages(): array
    {
        return ['title.required' => 'A title'];
    }
}

class NotARequest extends Unrelated
{
    public function rules(): array
    {
        return ['title' => 'nope'];
    }
}
`
	got := strings.Join(ruleStrings(t, src), " | ")
	if want := "piped:required[],string[] | array:sometimes[]"; got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestValidationRuleAt(t *testing.T) {
	src := "<?php\n$request->validate(['a' => 'required|max:255', 'b' => ['nullable']]);"
	file := phpparser.Parse(src, token.Latest)
	syn := rulesFixture.syntax(src)
	for _, tc := range []struct {
		at   string
		want string
		ok   bool
	}{
		{"'required", "required", true},
		{"'requ", "required", true},
		{"required|", "max", true},
		{"max:2", "max", true},
		{"['nullable", "nullable", true},
		{"'a'", "", false},
		{"validate", "", false},
	} {
		offset := strings.Index(src, tc.at) + len(tc.at)
		_, seg, ok := ValidationRuleAt(src, file, syn, uint32(offset))
		if ok != tc.ok || (ok && seg.Name(src) != tc.want) {
			t.Errorf("after %q: %q %v, want %q %v", tc.at, seg.Name(src), ok, tc.want, tc.ok)
		}
	}
}

func TestMayValidate(t *testing.T) {
	for src, want := range map[string]bool{
		"<?php $request->validate([]);":         true,
		"<?php function rules() {}":             true,
		"<?php Validator::make($a, $b);":        true,
		"<?php $x = 'validate'; // validate":    false,
		"<?php $user->save(['name' => 'x|y']);": false,
	} {
		if got := MayValidate(parser.Tokenize(src)); got != want {
			t.Errorf("MayValidate(%q) = %v", src, got)
		}
	}
}

func TestValidationExtensionsUnread(t *testing.T) {
	e := NewValidationExtensions()
	unread := func() bool { _, _, u := e.snapshot(); return u }
	for name, src := range map[string]string{
		"computed name":       "<?php\nIlluminate\\Support\\Facades\\Validator::extend($name, fn () => true);\n",
		"custom validator":    "<?php\nIlluminate\\Support\\Facades\\Validator::resolver(fn () => null);\n",
		"resolving callback":  "<?php\n$app->resolving('validator', fn ($v) => $v->extend('x', fn () => true));\n",
		"after resolving":     "<?php\nuse Illuminate\\Contracts\\Validation\\Factory;\n$this->app->afterResolving(Factory::class, fn ($v) => null);\n",
		"validator->resolver": "<?php\napp('validator')->resolver(fn () => null);\n",
	} {
		e.IndexFile("file:///x.php", src)
		if !unread() {
			t.Errorf("%s: the file's rules can't be read", name)
		}
		e.IndexFile("file:///x.php", "")
		if unread() {
			t.Errorf("%s: removing the file forgets it", name)
		}
	}
	e.IndexFile("file:///y.php", "<?php\n$app->resolving(Foo::class, fn () => null);\nCache::resolver(fn () => null);\n")
	if unread() {
		t.Error("other services' callbacks register no rules")
	}
}
