package laravel

import (
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/composer"
	phpparser "github.com/Tusk-PHP/lsp/internal/php/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
	"github.com/Tusk-PHP/lsp/internal/textpos"
)

// Ability is one way a Gate ability name is answered: a Gate::define()
// call, or a method of a policy, registered with Gate::resource(),
// Gate::policy(), a provider's $policies or found by Laravel's naming
// convention.
type Ability struct {
	Name   string
	URI    string
	Range  textpos.Range
	Policy string
	Method string
}

// MiddlewareKind says whether a middleware name is an alias of one
// middleware class or the name of a group.
type MiddlewareKind uint8

const (
	MiddlewareAlias MiddlewareKind = iota + 1
	MiddlewareGroup
)

// Middleware is a middleware name the app can use on a route: an alias or a
// group, from bootstrap/app.php, the HTTP kernel or Laravel's defaults.
type Middleware struct {
	Name  string
	Kind  MiddlewareKind
	Class string
	URI   string
	Range textpos.Range
}

// Authorization knows the Gate abilities and middleware names of a Laravel
// app. The app's registrations are read per file as the workspace indexes
// app/Providers, bootstrap/app.php and app/Http/Kernel.php; Laravel's own
// defaults and the policy namespaces are read again after Composer changes.
// Policy methods come from the symbol index.
type Authorization struct {
	rootPath string
	index    *symbols.Index
	grammars func() phpparser.GrammarRange

	mu         sync.Mutex
	files      map[string]authFile
	framework  frameworkAuth
	policyDirs []string
	generation uint64
	names      *authNames
}

type frameworkAuth struct {
	middleware []Middleware
	resource   []resourceAbility
}

type authNames struct {
	indexGeneration uint64
	abilities       map[string][]Ability
	abilityNames    []string
	middleware      map[string][]Middleware
	middlewareNames []string
	policies        map[string]string
}

// NewAuthorization returns the authorization names of the Laravel app at
// rootPath. They are empty until ScanWorkspace.
func NewAuthorization(rootPath string, index *symbols.Index, grammars func() phpparser.GrammarRange) *Authorization {
	return &Authorization{rootPath: rootPath, index: index, grammars: grammars, files: map[string]authFile{}}
}

// ScanWorkspace reads the app's registrations from disk, then what Composer
// installed.
func (a *Authorization) ScanWorkspace(autoload []composer.AutoloadEntry) {
	var paths []string
	if entries, err := os.ReadDir(filepath.Join(a.rootPath, "app", "Providers")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".php") {
				paths = append(paths, filepath.Join(a.rootPath, "app", "Providers", e.Name()))
			}
		}
	}
	paths = append(paths, filepath.Join(a.rootPath, "bootstrap", "app.php"), filepath.Join(a.rootPath, "app", "Http", "Kernel.php"))
	for _, path := range paths {
		if data, err := os.ReadFile(path); err == nil {
			a.IndexFile(symbols.PathToURI(path), string(data))
		}
	}
	a.ComposerChanged(autoload)
}

// ComposerChanged reads again what depends on the installed packages and the
// autoload map: Laravel's default middleware, the abilities Gate::resource()
// defines and the namespaces policies are discovered in.
func (a *Authorization) ComposerChanged(autoload []composer.AutoloadEntry) {
	framework := readFrameworkAuth(a.rootPath, a.grammars())
	dirs := policyNamespaces(a.rootPath, autoload)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.framework, a.policyDirs = framework, dirs
	a.changedLocked()
}

// IndexFile reads what source registers when uri is one of the files
// registrations are read from, and forgets it when source is empty.
func (a *Authorization) IndexFile(uri, source string) {
	if a == nil || !a.reads(uri) {
		return
	}
	var file authFile
	if source != "" {
		file = readAuthFile(uri, source, a.grammars())
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if file.empty() {
		if _, had := a.files[uri]; !had {
			return
		}
		delete(a.files, uri)
	} else {
		a.files[uri] = file
	}
	a.changedLocked()
}

func (a *Authorization) changedLocked() {
	a.generation++
	a.names = nil
}

func (a *Authorization) reads(uri string) bool {
	rel, err := filepath.Rel(a.rootPath, symbols.URIToPath(uri))
	if err != nil {
		return false
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(rel, ".php") {
		return false
	}
	return path.Dir(rel) == "app/Providers" || rel == "bootstrap/app.php" || rel == "app/Http/Kernel.php"
}

// AbilityNames lists every ability name, sorted.
func (a *Authorization) AbilityNames() []string {
	return a.current().abilityNames
}

// Abilities returns what answers an ability name.
func (a *Authorization) Abilities(name string) []Ability {
	return a.current().abilities[name]
}

// AbilityFor returns the definition of an ability checked against model,
// a class name: that model's policy method when it has one, else the first
// definition.
func (a *Authorization) AbilityFor(name, model string) (Ability, bool) {
	names := a.current()
	abilities := names.abilities[name]
	if len(abilities) == 0 {
		return Ability{}, false
	}
	if policy := a.policyFor(names, model); policy != "" {
		for _, ability := range abilities {
			if strings.EqualFold(ability.Policy, policy) {
				return ability, true
			}
		}
	}
	return abilities[0], true
}

// MiddlewareNames lists every middleware alias and group, sorted.
func (a *Authorization) MiddlewareNames() []string {
	return a.current().middlewareNames
}

// Middleware returns the aliases and groups registered under name, the app's
// before Laravel's.
func (a *Authorization) Middleware(name string) []Middleware {
	return a.current().middleware[name]
}

func (a *Authorization) policyFor(names *authNames, model string) string {
	if model == "" {
		return ""
	}
	model = strings.TrimPrefix(model, `\`)
	if policy, ok := names.policies[strings.ToLower(model)]; ok {
		return policy
	}
	for _, candidate := range guessPolicyNames(model) {
		if a.index.Lookup(candidate) != nil {
			return candidate
		}
	}
	return ""
}

func guessPolicyNames(model string) []string {
	cut := strings.LastIndexByte(model, '\\')
	if cut < 0 {
		return nil
	}
	dir, policy := model[:cut], model[cut+1:]+"Policy"
	segments := strings.Split(dir, `\`)
	candidates := make([]string, 0, len(segments)+2)
	for i := 1; i <= len(segments); i++ {
		candidates = append(candidates, strings.Join(segments[:i], `\`)+`\Policies\`+policy)
	}
	if strings.Contains(dir, `\Models\`) {
		candidates = append(candidates,
			strings.ReplaceAll(dir, `\Models\`, `\Policies\`)+`\`+policy,
			strings.ReplaceAll(dir, `\Models\`, `\Models\Policies\`)+`\`+policy)
	}
	slices.Reverse(candidates)
	return candidates
}

func policyNamespaces(rootPath string, autoload []composer.AutoloadEntry) []string {
	var namespaces []string
	for _, dir := range []string{filepath.Join("app", "Policies"), filepath.Join("app", "Models", "Policies")} {
		if ns := composer.PathToNamespace(filepath.Join(rootPath, dir, "Policy.php"), autoload); ns != "" {
			namespaces = append(namespaces, strings.Trim(ns, `\`))
		}
	}
	return namespaces
}

func (a *Authorization) current() *authNames {
	if a == nil {
		return &authNames{}
	}
	indexGeneration := a.index.Generation()
	a.mu.Lock()
	if a.names != nil && a.names.indexGeneration == indexGeneration {
		names := a.names
		a.mu.Unlock()
		return names
	}
	generation := a.generation
	uris := sortedKeys(a.files)
	files := make([]authFile, len(uris))
	for i, uri := range uris {
		files[i] = a.files[uri]
	}
	framework, dirs := a.framework, a.policyDirs
	a.mu.Unlock()
	names := a.build(files, framework, dirs)
	names.indexGeneration = indexGeneration
	a.mu.Lock()
	if a.generation == generation {
		a.names = names
	}
	a.mu.Unlock()
	return names
}

func (a *Authorization) build(files []authFile, framework frameworkAuth, policyDirs []string) *authNames {
	names := &authNames{abilities: map[string][]Ability{}, middleware: map[string][]Middleware{}, policies: map[string]string{}}
	addAbility := func(ability Ability) {
		names.abilities[ability.Name] = append(names.abilities[ability.Name], ability)
	}
	var policies []string
	for _, file := range files {
		for _, ability := range file.abilities {
			addAbility(ability)
		}
		for _, reg := range file.policies {
			names.policies[strings.ToLower(reg.model)] = reg.policy
			policies = append(policies, reg.policy)
		}
		for _, m := range file.middleware {
			names.middleware[m.Name] = append(names.middleware[m.Name], m)
		}
	}
	for _, m := range framework.middleware {
		names.middleware[m.Name] = append(names.middleware[m.Name], m)
	}
	for _, file := range files {
		for _, res := range file.resources {
			for _, ability := range a.resourceAbilities(res, framework.resource) {
				addAbility(ability)
			}
		}
	}
	for _, ns := range policyDirs {
		for _, sym := range a.index.GetNamespaceMembers(ns) {
			if sym.Kind == symbols.KindClass && strings.HasSuffix(sym.Name, "Policy") {
				policies = append(policies, sym.FQN)
			}
		}
	}
	slices.Sort(policies)
	for _, policy := range slices.Compact(policies) {
		for _, ability := range a.policyAbilities(policy) {
			addAbility(ability)
		}
	}
	names.abilityNames = sortedKeys(names.abilities)
	names.middlewareNames = sortedKeys(names.middleware)
	return names
}

func (a *Authorization) resourceAbilities(res resourceRegistration, defaults []resourceAbility) []Ability {
	abilities := res.abilities
	if abilities == nil {
		abilities = defaults
	}
	out := make([]Ability, 0, len(abilities))
	for _, ra := range abilities {
		ability := Ability{Name: res.name + "." + ra.ability, URI: res.uri, Range: res.rng, Policy: res.policy, Method: ra.method}
		if method := a.policyMethod(res.policy, ra.method); method != nil {
			ability.URI, ability.Range = method.URI, method.Range
		}
		out = append(out, ability)
	}
	return out
}

func (a *Authorization) policyMethod(policy, method string) *symbols.Symbol {
	for _, m := range a.index.ClassMembersNamed(policy, method) {
		if m.Kind == symbols.KindMethod {
			return m
		}
	}
	return nil
}

func (a *Authorization) policyAbilities(policy string) []Ability {
	var out []Ability
	for _, m := range a.index.GetClassMembers(policy) {
		if !isPolicyAbility(m) {
			continue
		}
		out = append(out, Ability{Name: m.Name, URI: m.URI, Range: m.Range, Policy: policy, Method: m.Name})
	}
	return out
}

func isPolicyAbility(m *symbols.Symbol) bool {
	if m.Kind != symbols.KindMethod || m.IsStatic || m.Visibility != "public" && m.Visibility != "" {
		return false
	}
	return !strings.HasPrefix(m.Name, "__") && !strings.EqualFold(m.Name, "before") && !strings.EqualFold(m.Name, "after")
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
