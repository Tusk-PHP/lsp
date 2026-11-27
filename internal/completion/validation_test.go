package completion

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/protocol"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var validationStubs = map[string]string{
	"vendor/Http/Request.php": `<?php
namespace Illuminate\Http;
class Request {}
`,
	"vendor/Http/FormRequest.php": `<?php
namespace Illuminate\Foundation\Http;
use Illuminate\Http\Request;
class FormRequest extends Request {}
`,
	"vendor/Validation/ValidatesRequests.php": `<?php
namespace Illuminate\Foundation\Validation;
trait ValidatesRequests {}
`,
	"vendor/Facades/Validator.php": `<?php
namespace Illuminate\Support\Facades;
class Validator {}
`,
	"vendor/helpers.php": `<?php
function request(): \Illuminate\Http\Request {}
function validator(array $data = [], array $rules = []) {}
`,
	"vendor/Validation/ValidatesAttributes.php": `<?php
namespace Illuminate\Validation\Concerns;
trait ValidatesAttributes
{
    /** Validate that a required attribute exists. */
    public function validateRequired($attribute, $value) {}
    /** Validate that an attribute exists when another attribute has a given value. */
    public function validateRequiredIf($attribute, $value, $parameters) { $this->requireParameterCount(2, $parameters, 'required_if'); }
    /** Validate the size of an attribute is less than or equal to a maximum value. */
    public function validateMax($attribute, $value, $parameters) { $this->requireParameterCount(1, $parameters, 'max'); }
    public function validateIn($attribute, $value, $parameters) {}
    public function validateString($attribute, $value) {}
    public function validateNullable() {}
}
`,
	"app/Http/Controllers/Controller.php": `<?php
namespace App\Http\Controllers;
use Illuminate\Foundation\Validation\ValidatesRequests;
abstract class Controller { use ValidatesRequests; }
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

func validationProvider(t testing.TB) *Provider {
	t.Helper()
	root := t.TempDir()
	idx := symbols.NewIndex()
	idx.RegisterBuiltins()
	for rel, src := range validationStubs {
		path := filepath.Join(root, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		idx.IndexFileWithSource(symbols.PathToURI(path), src, symbols.SourceProject)
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
	p.SetValidationRules(frameworklaravel.NewValidationRules(idx, frameworklaravel.NewProviders(root, idx), read))
	return p
}

func completeAtMarker(t testing.TB, p *Provider, src string) []protocol.CompletionItem {
	t.Helper()
	i := strings.Index(src, "|^")
	if i < 0 {
		t.Fatal("no |^ marker")
	}
	src = src[:i] + src[i+2:]
	line := strings.Count(src[:i], "\n")
	col := i - strings.LastIndex(src[:i], "\n") - 1
	const uri = "file:///app/Http/Controllers/PostController.php"
	p.index.IndexFile(uri, src)
	return p.GetCompletions(uri, src, protocol.Position{Line: line, Character: col})
}

func labels(items []protocol.CompletionItem) string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Label)
	}
	return strings.Join(out, " ")
}

const validationController = `<?php
namespace App\Http\Controllers;

use Illuminate\Http\Request;
use Illuminate\Support\Facades\Validator;

class PostController extends Controller
{
    public function store(Request $request)
    {
        CALL
    }
}
`

func TestValidationRuleCompletionInEachCallForm(t *testing.T) {
	p := validationProvider(t)
	for name, tc := range map[string]struct{ call, want string }{
		"request validate":   {`$request->validate(['title' => 'requ|^']);`, "required required_if"},
		"after a pipe":       {`$request->validate(['title' => 'required|m|^']);`, "max"},
		"empty segment":      {`$request->validate(['title' => 'string||^']);`, "in max nullable phone_number required required_if string"},
		"array form":         {`$request->validate(['title' => ['nul|^', 'max:255']]);`, "nullable"},
		"request helper":     {`request()->validate(['title' => 'str|^']);`, "string"},
		"controller":         {`$this->validate($request, ['title' => 'nu|^']);`, "nullable"},
		"validateWithBag":    {`$request->validateWithBag('post', ['title' => 'nu|^']);`, "nullable"},
		"validator facade":   {`Validator::make($request->all(), ['title' => 'requ|^']);`, "required required_if"},
		"validator validate": {`Validator::validate($request->all(), ['title' => 'i|^']);`, "in"},
		"validator helper":   {`validator($request->all(), ['title' => 'i|^']);`, "in"},
		"custom rule":        {`$request->validate(['phone' => 'pho|^']);`, "phone_number"},
		"camel case":         {`$request->validate(['title' => 'requiredI|^']);`, "required_if"},
		"unclosed call":      {`$request->validate(['title' => 'nu|^'`, "nullable"},
		"unclosed array":     {`$request->validate(['title' => 'string', 'body' => ['nu|^'`, "nullable"},
	} {
		t.Run(name, func(t *testing.T) {
			items := completeAtMarker(t, p, strings.Replace(validationController, "CALL", tc.call, 1))
			if got := labels(items); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestValidationRuleCompletionInFormRequestRules(t *testing.T) {
	p := validationProvider(t)
	src := `<?php
namespace App\Http\Requests;

use Illuminate\Foundation\Http\FormRequest;

class StorePostRequest extends FormRequest
{
    public function rules(): array
    {
        return ['title' => 'required|ma|^'];
    }
}
`
	if got := labels(completeAtMarker(t, p, src)); got != "max" {
		t.Errorf("got %q", got)
	}
}

func TestValidationRuleCompletionEdits(t *testing.T) {
	p := validationProvider(t)
	items := completeAtMarker(t, p, strings.Replace(validationController, "CALL", `$request->validate(['title' => 'string|requ|^']);`, 1))
	byLabel := map[string]protocol.CompletionItem{}
	for _, it := range items {
		byLabel[it.Label] = it
	}
	req, ok := byLabel["required_if"]
	if !ok || req.TextEdit == nil {
		t.Fatalf("required_if: %+v", items)
	}
	if req.TextEdit.NewText != "required_if:${1:anotherfield},${2:value}" || req.InsertTextFormat != 2 {
		t.Errorf("required_if inserts %q (format %d)", req.TextEdit.NewText, req.InsertTextFormat)
	}
	if r := req.TextEdit.Range; r.Start.Line != 10 || r.End.Character-r.Start.Character != len("requ") {
		t.Errorf("required_if replaces %+v", r)
	}
	if req.Detail != "required_if:anotherfield,value,…" || !strings.HasPrefix(req.Documentation, "Validate that an attribute exists") {
		t.Errorf("required_if detail %q, documentation %q", req.Detail, req.Documentation)
	}
	if plain := byLabel["required"]; plain.TextEdit.NewText != "required" || plain.InsertTextFormat != 0 {
		t.Errorf("required inserts %q", plain.TextEdit.NewText)
	}

	items = completeAtMarker(t, p, strings.Replace(validationController, "CALL", `$request->validate(['title' => 'm|^:255']);`, 1))
	if len(items) != 1 || items[0].TextEdit.NewText != "max" {
		t.Errorf("a rule with its parameters written gets only its name: %+v", items)
	}
}

func TestValidationRuleCompletionStaysOut(t *testing.T) {
	p := validationProvider(t)
	for name, call := range map[string]string{
		"parameters":    `$request->validate(['title' => 'max:2|^']);`,
		"messages":      `$request->validate(['title' => 'required'], ['title.required' => 'req|^']);`,
		"field name":    `$request->validate(['tit|^' => 'required']);`,
		"other method":  `$request->input('requ|^');`,
		"concatenation": `$request->validate(['title' => 'requ|^' . $more]);`,
	} {
		t.Run(name, func(t *testing.T) {
			items := completeAtMarker(t, p, strings.Replace(validationController, "CALL", call, 1))
			for _, it := range items {
				if it.Label == "required_if" || it.Label == "nullable" {
					t.Errorf("rule %q offered", it.Label)
				}
			}
		})
	}
}

func BenchmarkCompleteValidationRule(b *testing.B) {
	p := validationProvider(b)
	src := strings.Replace(validationController, "CALL", `$request->validate(['title' => 'string|requ|^']);`, 1)
	b.ReportAllocs()
	for b.Loop() {
		if len(completeAtMarker(b, p, src)) == 0 {
			b.Fatal("no completions")
		}
	}
}

func TestValidationRulesFromTheInstalledFramework(t *testing.T) {
	requireLaravelVendor(t)
	root, err := filepath.Abs(filepath.Join("..", "..", "testdata", "laravel"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "vendor/laravel/framework/src/Illuminate/Validation/Concerns/ValidatesAttributes.php")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	idx := symbols.NewIndex()
	idx.IndexFileWithSource(symbols.PathToURI(path), string(data), symbols.SourceVendor)
	read := func(uri string) string {
		data, err := os.ReadFile(symbols.URIToPath(uri))
		if err != nil {
			t.Fatalf("read %s: %v", uri, err)
		}
		return string(data)
	}
	rules := frameworklaravel.NewValidationRules(idx, frameworklaravel.NewProviders(root, idx), read)
	if n := len(rules.All()); n < 100 {
		t.Errorf("%d built-in rules", n)
	}
	for name, signature := range map[string]string{
		"required_if":      "required_if:anotherfield,value,…",
		"exists":           "exists:table,column",
		"date_format":      "date_format:format,…",
		"regex":            "regex:pattern",
		"current_password": "current_password:guard",
		"bail":             "bail",
		"mac_address":      "mac_address",
	} {
		r, ok := rules.Lookup(name)
		if !ok || r.Signature() != signature || r.Summary == "" {
			t.Errorf("%s: %+v", name, r)
		}
	}
	if r, _ := rules.Lookup("required_if"); r.Required != 2 {
		t.Errorf("required_if requires %d parameters", r.Required)
	}
	for _, r := range rules.All() {
		if r.TakesParameters && len(r.Parameters) == 0 {
			t.Errorf("%s takes parameters Laravel's documentation doesn't name here", r.Name)
		}
	}
}
