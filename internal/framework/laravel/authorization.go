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
// the project's PHP files: app/Providers, bootstrap/app.php and
// app/Http/Kernel.php always, any other file when its tokens show a Gate or
// middleware registration, and bootstrap/providers.php and config/app.php
// for the providers they list. Laravel's own defaults, the policy namespaces
// and what the packages' providers register are read again after Composer
// changes. Policy methods come from the symbol index.
//
// It also knows whether its lists are complete: a name registered in a way
// it can't read (a computed name, Gate::before(), a package's provider)
// makes the list it belongs to open.
type Authorization struct {
	rootPath string
	index    *symbols.Index
	grammars func() phpparser.GrammarRange

	mu                sync.Mutex
	files             map[string]authFile
	framework         frameworkAuth
	packages          authOpen
	policyDirs        []string
	projectNamespaces []string
	generation        uint64
	names             *authNames
}

type frameworkAuth struct {
	middleware []Middleware
	resource   []resourceAbility
	defaults   bool
}

type authOpen struct {
	abilities, middleware bool
}

func (o *authOpen) add(other authOpen) {
	o.abilities = o.abilities || other.abilities
	o.middleware = o.middleware || other.middleware
}

type authNames struct {
	indexGeneration uint64
	abilities       map[string][]Ability
	abilityNames    []string
	middleware      map[string][]Middleware
	middlewareNames []string
	policies        map[string]string
	open            authOpen
}

// NewAuthorization returns the authorization names of the Laravel app at
// rootPath. They are empty until ScanWorkspace.
func NewAuthorization(rootPath string, index *symbols.Index, grammars func() phpparser.GrammarRange) *Authorization {
	return &Authorization{rootPath: rootPath, index: index, grammars: grammars, files: map[string]authFile{}}
}

// ScanWorkspace reads what Composer installed, then the registrations of
// the project's PHP files, given by path, from disk.
func (a *Authorization) ScanWorkspace(autoload []composer.AutoloadEntry, projectFiles []string) {
	a.ComposerChanged(autoload)
	for _, path := range projectFiles {
		if data, err := os.ReadFile(path); err == nil {
			a.IndexFile(symbols.PathToURI(path), string(data))
		}
	}
}

// ComposerChanged reads again what depends on the installed packages and the
// autoload map: Laravel's default middleware, the abilities Gate::resource()
// defines, the namespaces policies are discovered in and what the packages'
// providers register.
func (a *Authorization) ComposerChanged(autoload []composer.AutoloadEntry) {
	grammars := a.grammars()
	framework := readFrameworkAuth(a.rootPath, grammars)
	dirs := policyNamespaces(a.rootPath, autoload)
	packages := readPackageProviders(a.rootPath, autoload, grammars)
	a.mu.Lock()
	defer a.mu.Unlock()
	a.framework, a.policyDirs, a.packages, a.projectNamespaces = framework, dirs, packages, projectNamespaces(autoload)
	a.changedLocked()
}

// IndexFile reads what a project file registers, and forgets it when
// source is empty. Vendor files and Blade views register nothing.
func (a *Authorization) IndexFile(uri, source string) {
	if a == nil {
		return
	}
	rel, ok := a.projectPath(uri)
	if !ok {
		return
	}
	var file authFile
	grammars := a.grammars()
	if source != "" && (alwaysReads(rel) || mayRegisterNames(source, grammars.Newest())) {
		a.mu.Lock()
		namespaces := a.projectNamespaces
		a.mu.Unlock()
		file = readAuthFile(uri, source, rel, grammars, namespaces)
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

func (a *Authorization) projectPath(uri string) (string, bool) {
	rel, err := filepath.Rel(a.rootPath, symbols.URIToPath(uri))
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	if !strings.HasSuffix(rel, ".php") || strings.HasSuffix(rel, ".blade.php") || strings.HasPrefix(rel, "../") || strings.HasPrefix(rel, "vendor/") {
		return "", false
	}
	return rel, true
}

const (
	bootstrapApp       = "bootstrap/app.php"
	bootstrapProviders = "bootstrap/providers.php"
	configApp          = "config/app.php"
	httpKernel         = "app/Http/Kernel.php"
)

func alwaysReads(rel string) bool {
	switch rel {
	case bootstrapApp, bootstrapProviders, configApp, httpKernel:
		return true
	}
	return isProvider(rel)
}

func isProvider(rel string) bool {
	return path.Dir(rel) == "app/Providers"
}

func projectNamespaces(autoload []composer.AutoloadEntry) []string {
	var namespaces []string
	for _, entry := range autoload {
		if !entry.IsVendor && !entry.IsFile && entry.Namespace != "" {
			namespaces = append(namespaces, strings.Trim(entry.Namespace, `\`)+`\`)
		}
	}
	return namespaces
}

// UnknownAbility reports whether name is surely no ability of the app: its
// abilities are all known and none of them, nor a method of the policy of
// model, is called name.
func (a *Authorization) UnknownAbility(name string, model ModelArg) bool {
	names := a.current()
	if names.open.abilities || name == "" {
		return false
	}
	if model.Given {
		if model.Class == "" {
			return false
		}
		if policy := a.policyFor(names, model.Class); policy != "" {
			if a.policyMethod(policy, name) != nil || !a.policyKnown(policy) {
				return false
			}
		}
	}
	return len(names.abilities[name]) == 0
}

// UnknownMiddleware reports whether name is surely no middleware alias or
// group of the app. A class name is never reported.
func (a *Authorization) UnknownMiddleware(name string) bool {
	names := a.current()
	if names.open.middleware || name == "" || strings.Contains(name, `\`) {
		return false
	}
	return len(names.middleware[name]) == 0
}

// ModelArg is the argument an ability is checked against: Given when the
// call passes one, with its class when that is known.
type ModelArg struct {
	Given bool
	Class string
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
	framework, dirs, packages := a.framework, a.policyDirs, a.packages
	a.mu.Unlock()
	names := a.build(files, framework, dirs)
	names.open.add(packages)
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
	kernelAliases := false
	for _, file := range files {
		names.open.add(file.open)
		kernelAliases = kernelAliases || file.kernelAliases
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
	if !framework.defaults && !kernelAliases {
		names.open.middleware = true
	}
	for _, file := range files {
		for _, res := range file.resources {
			if res.abilities == nil && framework.resource == nil {
				names.open.abilities = true
			}
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
		if !a.policyKnown(policy) {
			names.open.abilities = true
		}
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

func (a *Authorization) policyKnown(policy string) bool {
	sym := a.index.Lookup(policy)
	if sym == nil || len(a.index.ClassMembersNamed(policy, "__call")) > 0 {
		return false
	}
	for _, parent := range a.index.GetInheritanceChain(policy) {
		if a.index.Lookup(parent) == nil {
			return false
		}
	}
	for _, trait := range sym.Traits {
		if a.index.Lookup(trait) == nil {
			return false
		}
	}
	return true
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
