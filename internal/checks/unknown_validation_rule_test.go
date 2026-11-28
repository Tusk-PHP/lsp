package checks

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	frameworklaravel "github.com/Tusk-PHP/lsp/internal/framework/laravel"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/rule"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

var validationRuleFixture = map[string]string{
	"vendor/Request.php": "<?php\nnamespace Illuminate\\Http;\nclass Request {}\n",
	"vendor/ValidatesAttributes.php": `<?php
namespace Illuminate\Validation\Concerns;
trait ValidatesAttributes
{
    public function validateRequired($attribute, $value) {}
    public function validateRequiredIf($attribute, $value, $parameters) {}
    public function validateEmail($attribute, $value, $parameters) {}
    public function validateInteger($attribute, $value, $parameters) {}
    public function validateBoolean($attribute, $value, $parameters) {}
    public function validateMax($attribute, $value, $parameters) {}
    public function validateRegex($attribute, $value, $parameters) {}
}
`,
	"vendor/ValidationRuleParser.php": `<?php
namespace Illuminate\Validation;
class ValidationRuleParser
{
    protected static function normalizeRule($rule)
    {
        return match ($rule) {
            'Int' => 'Integer',
            'Bool' => 'Boolean',
            default => $rule,
        };
    }
}
`,
}

const validationRuleProvider = `<?php
namespace App\Providers;
use Illuminate\Support\Facades\Validator;
class AppServiceProvider
{
    public function boot(): void
    {
        Validator::extend('phone_number', fn () => true);
        EXTRA
    }
}
`

type validationRuleCheck struct {
	rule  *UnknownValidationRuleRule
	index *symbols.Index
}

func newValidationRuleCheck(t testing.TB, providerExtra string, files map[string]string) validationRuleCheck {
	t.Helper()
	root := t.TempDir()
	all := map[string]string{"app/Providers/AppServiceProvider.php": strings.Replace(validationRuleProvider, "EXTRA", providerExtra, 1)}
	for rel, src := range files {
		all[rel] = src
	}
	idx := symbols.NewIndex()
	ext := frameworklaravel.NewValidationExtensions()
	for rel, src := range all {
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
	return validationRuleCheck{
		index: idx,
		rule: &UnknownValidationRuleRule{
			Rules: frameworklaravel.NewValidationRules(idx, ext, read),
			TypeResolver: func(expr, source string, line, col int, file *parser.FileNode) string {
				if expr == "$request" {
					return `\Illuminate\Http\Request`
				}
				return ""
			},
		},
	}
}

func (c validationRuleCheck) findings(t *testing.T, body string) string {
	t.Helper()
	src := "<?php\nnamespace App\\Http\\Controllers;\n\nuse Illuminate\\Http\\Request;\nuse Illuminate\\Support\\Facades\\Validator;\n\nfunction store(Request $request)\n{\n    " + body + "\n}\n"
	lines := strings.Split(src, "\n")
	var out []string
	for _, f := range c.rule.Check(parser.ParseFile(src), src, c.index) {
		if f.Code != rule.UnknownValidationRule || f.Severity != rule.SeverityWarning || f.StartLine != f.EndLine {
			t.Errorf("finding %+v", f)
		}
		out = append(out, lines[f.StartLine][f.StartCol:f.EndCol]+": "+f.Message)
	}
	return strings.Join(out, "; ")
}

func TestUnknownValidationRule(t *testing.T) {
	c := newValidationRuleCheck(t, "", validationRuleFixture)
	for name, tc := range map[string]struct{ body, want string }{
		"misspelled":       {`$request->validate(['email' => 'requierd|email']);`, "requierd: Unknown validation rule `requierd`"},
		"array form":       {`$request->validate(['email' => ['required', 'emial', 'max:255']]);`, "emial: Unknown validation rule `emial`"},
		"with parameters":  {`$request->validate(['n' => 'maxx:3']);`, "maxx: Unknown validation rule `maxx`"},
		"facade":           {`Validator::make($data, ['n' => 'nope']);`, "nope: Unknown validation rule `nope`"},
		"known":            {`$request->validate(['n' => 'required|requiredIf:a,1|Required_If:a,1|max:3', 'b' => ['email:rfc,dns']]);`, ""},
		"aliases":          {`$request->validate(['n' => 'int|bool']);`, ""},
		"custom rule":      {`$request->validate(['p' => 'phone_number|phoneNumber']);`, ""},
		"empty segments":   {`$request->validate(['n' => 'required||', 'm' => '']);`, ""},
		"not rule names":   {`$request->validate(['n' => 'my.rule|two words']);`, ""},
		"regex piped":      {`$request->validate(['n' => 'required|regex:/^(a|b)$/']);`, ""},
		"regex in array":   {`$request->validate(['n' => ['regex:/^(a|b)$/', 'nope']]);`, "nope: Unknown validation rule `nope`"},
		"messages":         {`$request->validate(['n' => 'required'], ['n.required' => 'Needed']);`, ""},
		"dynamic":          {`$request->validate(['n' => 'nope|' . $x, 'm' => "nope:$y", 'o' => $rules]);`, ""},
		"unknown receiver": {`$form->validate(['n' => 'nope']);`, ""},
	} {
		t.Run(name, func(t *testing.T) {
			if got := c.findings(t, tc.body); got != tc.want {
				t.Errorf("got  %q\nwant %q", got, tc.want)
			}
		})
	}
}

func TestUnknownValidationRuleTrustsRulesRegisteredAnywhere(t *testing.T) {
	files := map[string]string{
		"bootstrap/app.php":             "<?php\nuse Illuminate\\Support\\Facades\\Validator;\nValidator::extend('slug', fn () => true);\n",
		"app/Http/Middleware/Rules.php": "<?php\nnamespace App\\Http\\Middleware;\nclass Rules { public function handle() { app('validator')->extend('isbn', fn () => true); } }\n",
	}
	for rel, src := range validationRuleFixture {
		files[rel] = src
	}
	c := newValidationRuleCheck(t, "", files)
	if got := c.findings(t, `$request->validate(['s' => 'slug|isbn|requierd']);`); got != "requierd: Unknown validation rule `requierd`" {
		t.Errorf("got %q", got)
	}
}

func TestUnknownValidationRuleStaysQuietWithoutEveryRule(t *testing.T) {
	const body = `$request->validate(['email' => 'requierd']);`
	for name, extra := range map[string]string{
		"a rule under a variable name": `Validator::extend($name, fn () => true);`,
		"a custom validator":           `Validator::resolver(fn (...$args) => new CustomValidator(...$args));`,
		"rules added on resolving":     `$this->app->afterResolving('validator', fn ($v) => $v->extend('x', fn () => true));`,
	} {
		t.Run(name, func(t *testing.T) {
			if got := newValidationRuleCheck(t, extra, validationRuleFixture).findings(t, body); got != "" {
				t.Errorf("got %q", got)
			}
		})
	}
	t.Run("no framework rules", func(t *testing.T) {
		files := map[string]string{"vendor/Request.php": validationRuleFixture["vendor/Request.php"]}
		if got := newValidationRuleCheck(t, "", files).findings(t, body); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("until the index is built", func(t *testing.T) {
		c := newValidationRuleCheck(t, "", validationRuleFixture)
		c.index = symbols.NewIndex()
		c.rule.Rules = frameworklaravel.NewValidationRules(c.index, frameworklaravel.NewValidationExtensions(), func(string) string { return "" })
		if got := c.findings(t, body); got != "" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("container and manager extend calls are no rules", func(t *testing.T) {
		extra := `$this->app->extend(Foo::class, fn ($foo) => $foo); Cache::extend('mongo', fn () => null);`
		if got := newValidationRuleCheck(t, extra, validationRuleFixture).findings(t, body); got == "" {
			t.Error("requierd is still unknown")
		}
	})
}

func BenchmarkUnknownValidationRule(b *testing.B) {
	c := newValidationRuleCheck(b, "", validationRuleFixture)
	var body strings.Builder
	body.WriteString("<?php\nnamespace App\\Http\\Controllers;\n\nuse Illuminate\\Http\\Request;\n\nclass PostController\n{\n")
	for i := 0; i < 50; i++ {
		body.WriteString("    public function store" + strconv.Itoa(i) + "(Request $request)\n    {\n        $request->validate(['title' => 'required|max:255', 'tags' => ['required', 'email']]);\n        $x = $request->input('title') . 'suffix';\n    }\n")
	}
	body.WriteString("}\n")
	src := body.String()
	file := parser.ParseFile(src)
	b.ReportAllocs()
	for b.Loop() {
		if got := c.rule.Check(file, src, c.index); len(got) != 0 {
			b.Fatal(got)
		}
	}
}

func TestUnknownValidationRuleReportsItsCode(t *testing.T) {
	if codes := (&UnknownValidationRuleRule{}).Codes(); len(codes) != 1 || codes[0] != rule.UnknownValidationRule {
		t.Errorf("Codes() = %v", codes)
	}
}
