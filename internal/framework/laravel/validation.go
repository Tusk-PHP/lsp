package laravel

import (
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/php/ast"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/php/token"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

const validatesAttributes = `Illuminate\Validation\Concerns\ValidatesAttributes`

// ValidationRule is a rule a validation rule string can name.
type ValidationRule struct {
	// Name is the rule as it is written: `required_if`.
	Name string
	// TakesParameters says whether the rule reads parameters after a `:`;
	// false for a custom rule, whose parameters the server can't know.
	TakesParameters bool
	// Parameters are the names of its parameters as Laravel's documentation
	// writes them, for the rules it documents; "…" stands for more.
	Parameters []string
	// Required is how many parameters the rule requires.
	Required int
	// Summary is the first line of the rule method's doc comment.
	Summary string
	// Registered is where a custom rule is registered; nil for a built-in.
	Registered *Registration
}

// Signature is the rule with its parameters: `required_if:anotherfield,value,…`.
func (r ValidationRule) Signature() string {
	if !r.TakesParameters {
		return r.Name
	}
	if len(r.Parameters) == 0 {
		return r.Name + ":…"
	}
	return r.Name + ":" + strings.Join(r.Parameters, ",")
}

// Description is the rule's summary, or where a custom rule is registered.
func (r ValidationRule) Description() string {
	if r.Registered == nil {
		return r.Summary
	}
	return "Custom rule, registered in " + filepath.Base(symbols.URIToPath(r.Registered.URI)) + ":" + strconv.Itoa(r.Registered.Line+1)
}

// ValidationRuleKey is the key Laravel finds a rule by: it calls
// `validate` + Str::studly(trim(name)), and PHP method names ignore case,
// so `required_if`, `requiredIf` and `Required-If` are the same rule.
func ValidationRuleKey(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	b.Grow(len(name))
	for i := 0; i < len(name); i++ {
		c := name[i]
		switch {
		case c == '_' || c == '-' || c == ' ':
		case 'A' <= c && c <= 'Z':
			b.WriteByte(c + 'a' - 'A')
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// Snake is Laravel's Str::snake: an underscore before each ASCII capital
// but the first, then lower case, so `MacAddress` becomes `mac_address`.
func Snake(s string) string {
	var b strings.Builder
	b.Grow(len(s) + 4)
	for i := 0; i < len(s); i++ {
		if c := s[i]; 'A' <= c && c <= 'Z' && i > 0 {
			b.WriteByte('_')
		}
		b.WriteByte(s[i])
	}
	return strings.ToLower(b.String())
}

// ValidationRules are the validation rules of a Laravel app: the built-in
// rules of its installed framework and the custom rules its files register.
// They are read again when the framework's rule trait is re-indexed or a
// file's registrations change.
type ValidationRules struct {
	index      *symbols.Index
	extensions *ValidationExtensions
	helpers    *ValidationHelpers
	read       func(uri string) string

	mu         sync.Mutex
	trait      *symbols.Symbol
	extVersion uint64
	catalog    *validationCatalog
}

type validationCatalog struct {
	rules []ValidationRule
	byKey map[string]int
}

// NewValidationRules returns the validation rules of the app whose index,
// custom rule registrations and rule helper methods are given; read
// returns a file's source by URI.
func NewValidationRules(index *symbols.Index, extensions *ValidationExtensions, helpers *ValidationHelpers, read func(uri string) string) *ValidationRules {
	return &ValidationRules{index: index, extensions: extensions, helpers: helpers, read: read}
}

// All returns every rule, sorted by name. The slice is shared: read only.
func (v *ValidationRules) All() []ValidationRule {
	return v.current().rules
}

// Lookup returns the rule a name in a rule string stands for.
func (v *ValidationRules) Lookup(name string) (ValidationRule, bool) {
	c := v.current()
	i, ok := c.byKey[ValidationRuleKey(name)]
	if !ok {
		return ValidationRule{}, false
	}
	return c.rules[i], true
}

func (v *ValidationRules) current() *validationCatalog {
	trait := v.index.Lookup(validatesAttributes)
	v.mu.Lock()
	if v.catalog != nil && v.trait == trait && v.extVersion == v.extensions.currentVersion() {
		c := v.catalog
		v.mu.Unlock()
		return c
	}
	v.mu.Unlock()
	version, custom := v.extensions.snapshot()
	c := v.build(trait, custom)
	v.mu.Lock()
	v.trait, v.extVersion, v.catalog = trait, version, c
	v.mu.Unlock()
	return c
}

func (v *ValidationRules) build(trait *symbols.Symbol, custom []Registration) *validationCatalog {
	c := &validationCatalog{byKey: map[string]int{}}
	if trait != nil {
		required := requiredParameters(v.read(trait.URI))
		for _, m := range trait.Children {
			suffix, ok := strings.CutPrefix(m.Name, "validate")
			if m.Kind != symbols.KindMethod || !ok || suffix == "" {
				continue
			}
			name := Snake(suffix)
			rule := ValidationRule{
				Name:            name,
				TakesParameters: len(m.Params) >= 3,
				Required:        required[name],
			}
			if doc := parser.ParseDocBlock(m.DocComment); doc != nil {
				rule.Summary = firstLine(doc.Summary)
			}
			if params, documented := validationParameters[name]; documented && rule.TakesParameters {
				rule.Parameters = params
				rule.TakesParameters = len(params) > 0
			}
			c.add(rule)
		}
	}
	for _, reg := range custom {
		if _, known := c.byKey[ValidationRuleKey(reg.Name)]; known {
			continue
		}
		c.add(ValidationRule{Name: reg.Name, Registered: &reg})
	}
	slices.SortFunc(c.rules, func(a, b ValidationRule) int { return strings.Compare(a.Name, b.Name) })
	for i, r := range c.rules {
		c.byKey[ValidationRuleKey(r.Name)] = i
	}
	return c
}

func (c *validationCatalog) add(r ValidationRule) {
	c.byKey[ValidationRuleKey(r.Name)] = len(c.rules)
	c.rules = append(c.rules, r)
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return strings.TrimSpace(line)
}

func requiredParameters(src string) map[string]int {
	if src == "" {
		return nil
	}
	required := map[string]int{}
	ast.Walk(phpparser.Parse(src, token.Latest), func(n ast.Node) bool {
		call, ok := n.(*ast.MethodCall)
		if !ok || len(call.Args.Args) != 3 {
			return true
		}
		if name, ok := call.Name.(*ast.IdentifierExpr); !ok || name.Text(src) != "requireParameterCount" {
			return true
		}
		count, ok := call.Args.Args[0].Value.(*ast.Literal)
		if !ok || count.Kind != token.IntegerLiteral {
			return true
		}
		rule, ok := constantString(src, call.Args.Args[2].Value)
		if !ok {
			return true
		}
		if n, err := strconv.Atoi(count.Text(src)); err == nil {
			required[rule] = n
		}
		return true
	})
	return required
}

func constantString(src string, e ast.Expr) (string, bool) {
	lit, ok := e.(*ast.Literal)
	if !ok || lit.Kind != token.ConstantString || lit.End-lit.Start < 2 {
		return "", false
	}
	return src[lit.Start+1 : lit.End-1], true
}

var validationParameters = map[string][]string{
	"accepted_if":            {"anotherfield", "value", "…"},
	"after":                  {"date"},
	"after_or_equal":         {"date"},
	"alpha":                  {"ascii"},
	"alpha_dash":             {"ascii"},
	"alpha_num":              {"ascii"},
	"array":                  {"key", "…"},
	"before":                 {"date"},
	"before_or_equal":        {"date"},
	"between":                {"min", "max"},
	"boolean":                {"strict"},
	"confirmed":              {"field"},
	"contains":               {"value", "…"},
	"current_password":       {"guard"},
	"date_equals":            {"date"},
	"date_format":            {"format", "…"},
	"decimal":                {"min", "max"},
	"declined_if":            {"anotherfield", "value", "…"},
	"different":              {"field"},
	"digits":                 {"value"},
	"digits_between":         {"min", "max"},
	"dimensions":             {"constraint=value", "…"},
	"distinct":               {"strict|ignore_case"},
	"doesnt_contain":         {"value", "…"},
	"doesnt_end_with":        {"value", "…"},
	"doesnt_start_with":      {"value", "…"},
	"email":                  {"validator", "…"},
	"encoding":               {"encoding_type"},
	"ends_with":              {"value", "…"},
	"exclude_if":             {"anotherfield", "value"},
	"exclude_unless":         {"anotherfield", "value"},
	"exclude_with":           {"anotherfield"},
	"exclude_without":        {"anotherfield"},
	"exists":                 {"table", "column"},
	"extensions":             {"extension", "…"},
	"gt":                     {"field"},
	"gte":                    {"field"},
	"image":                  {"allow_svg"},
	"in":                     {"value", "…"},
	"in_array":               {"anotherfield.*"},
	"in_array_keys":          {"value", "…"},
	"integer":                {"strict"},
	"lowercase":              {},
	"lt":                     {"field"},
	"lte":                    {"field"},
	"max":                    {"value"},
	"max_digits":             {"value"},
	"mimes":                  {"extension", "…"},
	"mimetypes":              {"type/subtype", "…"},
	"min":                    {"value"},
	"min_digits":             {"value"},
	"missing":                {},
	"missing_if":             {"anotherfield", "value", "…"},
	"missing_unless":         {"anotherfield", "value"},
	"missing_with":           {"field", "…"},
	"missing_with_all":       {"field", "…"},
	"multiple_of":            {"value"},
	"not_in":                 {"value", "…"},
	"not_regex":              {"pattern"},
	"numeric":                {"strict"},
	"present_if":             {"anotherfield", "value", "…"},
	"present_unless":         {"anotherfield", "value"},
	"present_with":           {"field", "…"},
	"present_with_all":       {"field", "…"},
	"prohibited_if":          {"anotherfield", "value", "…"},
	"prohibited_if_accepted": {"anotherfield", "…"},
	"prohibited_if_declined": {"anotherfield", "…"},
	"prohibited_unless":      {"anotherfield", "value", "…"},
	"prohibits":              {"anotherfield", "…"},
	"regex":                  {"pattern"},
	"required_array_keys":    {"key", "…"},
	"required_if":            {"anotherfield", "value", "…"},
	"required_if_accepted":   {"anotherfield", "…"},
	"required_if_declined":   {"anotherfield", "…"},
	"required_unless":        {"anotherfield", "value", "…"},
	"required_with":          {"field", "…"},
	"required_with_all":      {"field", "…"},
	"required_without":       {"field", "…"},
	"required_without_all":   {"field", "…"},
	"same":                   {"field"},
	"size":                   {"value"},
	"starts_with":            {"value", "…"},
	"timezone":               {"group", "country"},
	"unique":                 {"table", "column"},
	"uppercase":              {},
	"url":                    {"protocol", "…"},
	"uuid":                   {"version"},
}
