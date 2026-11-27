package laravel

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/parser"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// Registration is where a provider registers something, for definition and
// hover.
type Registration struct {
	Name      string // as registered
	URI       string
	Line, Col int
}

// AnonymousPath is a directory of anonymous components, with the prefix
// they are written with: `<x-flux::button>` for prefix "flux". An empty
// prefix is `<x-name>`.
type AnonymousPath struct {
	Prefix string
	Dir    string
}

// Registrations are what an app's service providers, and those of the
// packages it installs, register with Blade and the view factory.
type Registrations struct {
	// Directives are Blade::directive() names, Conditionals Blade::if()
	// names, both lower-cased.
	Directives   map[string]Registration
	Conditionals map[string]Registration
	// ComponentAliases are Blade::component() and loadViewComponentsAs()
	// names, to their class.
	ComponentAliases map[string]string
	// ComponentNamespaces are componentNamespace() prefixes, to the PHP
	// namespace of their classes.
	ComponentNamespaces map[string]string
	// AnonymousPaths are anonymousComponentPath() and
	// anonymousComponentNamespace() directories.
	AnonymousPaths []AnonymousPath
	// ViewNamespaces are loadViewsFrom() and View::addNamespace() namespaces,
	// to their directories.
	ViewNamespaces map[string][]string
	// UnknownComponents is set when a provider registers components the
	// server can't name: `<x-name>` components may then come from anywhere.
	UnknownComponents bool
	// UnknownNamespaces holds view and component namespaces registered
	// with a directory the server can't read.
	UnknownNamespaces map[string]bool
	// ValidationRules are the rules registered with the validator's
	// extend(), extendImplicit() and extendDependent(), by ValidationRuleKey.
	ValidationRules map[string]Registration
	// UnknownValidationRules is set when a provider registers a validation
	// rule under a name the server can't read.
	UnknownValidationRules bool
}

// Custom returns the directives and conditionals Blade should know.
func (r *Registrations) Custom() *blade.Custom {
	if r == nil || len(r.Directives)+len(r.Conditionals) == 0 {
		return nil
	}
	c := &blade.Custom{Directives: map[string]bool{}, Conditionals: map[string]bool{}, Where: map[string]blade.Location{}, Names: map[string]string{}}
	for name, reg := range r.Directives {
		c.Directives[name] = true
		c.Names[name] = reg.Name
		c.Where[name] = blade.Location{URI: reg.URI, Line: reg.Line, Col: reg.Col, EndCol: reg.Col}
	}
	for name, reg := range r.Conditionals {
		c.Conditionals[name] = true
		c.Names[name] = reg.Name
		c.Where[name] = blade.Location{URI: reg.URI, Line: reg.Line, Col: reg.Col, EndCol: reg.Col}
	}
	return c
}

// Providers reads an app's registrations once the index is ready, and again
// after Invalidate.
type Providers struct {
	rootPath string
	index    *symbols.Index

	mu     sync.Mutex
	regs   *Registrations
	custom *blade.Custom
}

// Custom returns the directives and conditionals Blade should know, the
// same value until the registrations are read again.
func (p *Providers) Custom() *blade.Custom {
	regs := p.Get()
	if regs == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.custom == nil && p.regs == regs {
		p.custom = regs.Custom()
	}
	return p.custom
}

// NewProviders returns the provider reader of the Laravel app at rootPath.
func NewProviders(rootPath string, index *symbols.Index) *Providers {
	return &Providers{rootPath: rootPath, index: index}
}

// Invalidate forgets what was read, after a provider changed.
func (p *Providers) Invalidate() {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.regs, p.custom = nil, nil
	p.mu.Unlock()
}

// Get returns the registrations, or nil until the index is ready and the
// providers can be found.
func (p *Providers) Get() *Registrations {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	regs := p.regs
	p.mu.Unlock()
	if regs != nil {
		return regs
	}
	if p.index == nil || !p.index.Ready() {
		return nil
	}
	regs = p.read()
	p.mu.Lock()
	p.regs = regs
	p.mu.Unlock()
	return regs
}

// read reads the app's providers in app/Providers and the providers its
// packages register through package discovery.
func (p *Providers) read() *Registrations {
	r := &Registrations{
		Directives: map[string]Registration{}, Conditionals: map[string]Registration{},
		ComponentAliases: map[string]string{}, ComponentNamespaces: map[string]string{},
		ViewNamespaces: map[string][]string{}, UnknownNamespaces: map[string]bool{},
		ValidationRules: map[string]Registration{},
	}
	var files []string
	if entries, err := os.ReadDir(filepath.Join(p.rootPath, "app", "Providers")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".php") {
				files = append(files, filepath.Join(p.rootPath, "app", "Providers", e.Name()))
			}
		}
	}
	for _, fqn := range composer.LaravelPackageProviders(p.rootPath) {
		sym := p.index.Lookup(fqn)
		if sym == nil {
			// A provider that can't be read may register anything.
			r.UnknownComponents = true
			continue
		}
		files = append(files, symbols.URIToPath(sym.URI))
	}
	// Livewire 4's component locations are anonymous component paths.
	dirs, known := livewireLocations(p.rootPath)
	if !known {
		r.UnknownComponents = true
	}
	for _, d := range dirs {
		r.AnonymousPaths = append(r.AnonymousPaths, AnonymousPath{Dir: d})
	}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			r.UnknownComponents = true
			continue
		}
		src := string(data)
		if !strings.Contains(src, "Blade") && !strings.Contains(src, "blade.compiler") && !strings.Contains(src, "View") &&
			!strings.Contains(src, "loadView") && !strings.Contains(src, "hasViews") && !strings.Contains(src, "extend") {
			continue
		}
		(&providerScan{root: p.rootPath, path: path, regs: r}).scan(src)
	}
	return r
}

// providerScan reads one provider file.
type providerScan struct {
	root, path string
	regs       *Registrations
	scan_      *viewScan
}

func (ps *providerScan) scan(src string) {
	uri := "file://" + filepath.ToSlash(ps.path)
	all := parser.Tokenize(src)
	toks := make([]parser.Token, 0, len(all))
	for _, t := range all {
		if t.Kind != parser.TokenComment && t.Kind != parser.TokenDocComment {
			toks = append(toks, t)
		}
	}
	vs := &viewScan{uri: uri, source: src, toks: toks, imports: map[string]string{}}
	vs.declarations()
	ps.scan_ = vs
	livewire := strings.Contains(filepath.ToSlash(ps.path), "/livewire/livewire/src/")
	packageName := ""
	for i := 0; i+1 < len(toks); i++ {
		t := toks[i]
		if t.Kind != parser.TokenIdentifier || toks[i+1].Kind != parser.TokenOpenParen || i == 0 {
			continue
		}
		prev := toks[i-1].Kind
		if prev != parser.TokenArrow && prev != parser.TokenDoubleColon {
			continue
		}
		args, close := splitTokenArgs(toks, i+1)
		if close < 0 {
			continue
		}
		reg := Registration{URI: uri, Line: t.Line, Col: t.Column}
		if name, ok := argString(toks, args, 0); ok {
			reg.Name = name
		}
		switch t.Value {
		case "directive":
			if name, ok := argString(toks, args, 0); ok {
				ps.regs.Directives[strings.ToLower(name)] = reg
			}
		case "if":
			if name, ok := argString(toks, args, 0); ok && prev == parser.TokenDoubleColon {
				ps.regs.Conditionals[strings.ToLower(name)] = reg
			}
		case "component":
			ps.component(toks, args)
		case "components":
			ps.components(toks, args)
		case "componentNamespace":
			ns, ok1 := argString(toks, args, 0)
			prefix, ok2 := argString(toks, args, 1)
			if ok1 && ok2 {
				ps.regs.ComponentNamespaces[prefix] = strings.Trim(ns, `\`)
			} else {
				ps.regs.UnknownComponents = true
			}
		case "anonymousComponentPath":
			if livewire {
				continue // Livewire's component locations, read from its config
			}
			dir, ok := ps.path_(toks, args, 0)
			prefix := ""
			if len(args) > 1 {
				var okp bool
				if prefix, okp = argString(toks, args, 1); !okp {
					ps.regs.UnknownComponents = true
					continue
				}
			}
			if !ok {
				if prefix == "" {
					ps.regs.UnknownComponents = true
				} else {
					ps.regs.UnknownNamespaces[prefix] = true
				}
				continue
			}
			ps.regs.AnonymousPaths = append(ps.regs.AnonymousPaths, AnonymousPath{Prefix: prefix, Dir: dir})
		case "anonymousComponentNamespace":
			rel, ok1 := argString(toks, args, 0)
			prefix, ok2 := argString(toks, args, 1)
			if !ok1 || !ok2 {
				ps.regs.UnknownComponents = true
				continue
			}
			dir := filepath.Join(ps.root, "resources", "views", filepath.FromSlash(strings.ReplaceAll(rel, ".", "/")))
			ps.regs.AnonymousPaths = append(ps.regs.AnonymousPaths, AnonymousPath{Prefix: prefix, Dir: dir})
		case "loadViewComponentsAs":
			ps.viewComponentsAs(toks, args)
		case "extend", "extendImplicit", "extendDependent":
			if !ps.validatorExtension(toks, i) {
				continue
			}
			if name, ok := argString(toks, args, 0); ok {
				ps.regs.ValidationRules[ValidationRuleKey(name)] = reg
			} else {
				ps.regs.UnknownValidationRules = true
			}
		case "loadViewsFrom", "addNamespace":
			// loadViewsFrom($path, 'ns'); View::addNamespace('ns', $path).
			nsArg, pathArg := 1, 0
			if t.Value == "addNamespace" {
				nsArg, pathArg = 0, 1
			}
			ns, ok := argString(toks, args, nsArg)
			if !ok {
				continue
			}
			if dir, ok := ps.path_(toks, args, pathArg); ok {
				ps.regs.ViewNamespaces[ns] = append(ps.regs.ViewNamespaces[ns], dir)
			} else {
				ps.regs.UnknownNamespaces[ns] = true
			}
		case "name":
			// Spatie's package tools: ->name('filament')->hasViews().
			if name, ok := argString(toks, args, 0); ok && packageName == "" {
				packageName = name
			}
		case "hasViews":
			ns, ok := argString(toks, args, 0)
			if !ok {
				ns = strings.TrimPrefix(packageName, "laravel-")
			}
			if ns == "" {
				continue
			}
			// The package's resources/views, above the provider's src/.
			dir := filepath.Join(filepath.Dir(filepath.Dir(ps.path)), "resources", "views")
			ps.regs.ViewNamespaces[ns] = append(ps.regs.ViewNamespaces[ns], dir)
		}
	}
}

// component reads Blade::component(X::class, 'alias'), the reverse, or
// either with a prefix.
func (ps *providerScan) component(toks []parser.Token, args []tokenSpan) {
	if len(args) < 2 {
		ps.regs.UnknownComponents = true // an alias guessed from the class
		return
	}
	class, alias := "", ""
	if s, ok := literalString(toks, args[0]); ok && !strings.Contains(s, `\`) {
		alias, class = s, ps.classArg(toks, args[1])
	} else {
		class = ps.classArg(toks, args[0])
		alias, _ = literalString(toks, args[1])
	}
	if len(args) > 2 {
		prefix, ok := literalString(toks, args[2])
		if !ok {
			ps.regs.UnknownComponents = true
			return
		}
		if prefix != "" {
			alias = prefix + "-" + alias
		}
	}
	if class == "" || alias == "" {
		ps.regs.UnknownComponents = true
		return
	}
	ps.regs.ComponentAliases[alias] = class
}

// components reads Blade::components([X::class => 'alias', ...]).
func (ps *providerScan) components(toks []parser.Token, args []tokenSpan) {
	if len(args) == 0 || toks[args[0].start].Kind != parser.TokenOpenBracket {
		ps.regs.UnknownComponents = true
		return
	}
	for _, item := range splitItems(toks, tokenSpan{args[0].start + 1, args[0].end - 1}) {
		arrow := -1
		for k := item.start; k < item.end; k++ {
			if toks[k].Kind == parser.TokenDoubleArrow {
				arrow = k
				break
			}
		}
		if arrow < 0 {
			ps.regs.UnknownComponents = true
			continue
		}
		class := ps.classArg(toks, tokenSpan{item.start, arrow})
		alias, ok := literalString(toks, tokenSpan{arrow + 1, item.end})
		if class == "" || !ok {
			ps.regs.UnknownComponents = true
			continue
		}
		ps.regs.ComponentAliases[alias] = class
	}
}

// viewComponentsAs reads loadViewComponentsAs('prefix', [X::class, ...]):
// `<x-prefix-x>`.
func (ps *providerScan) viewComponentsAs(toks []parser.Token, args []tokenSpan) {
	prefix, ok := argString(toks, args, 0)
	if !ok || len(args) < 2 || toks[args[1].start].Kind != parser.TokenOpenBracket {
		ps.regs.UnknownComponents = true
		return
	}
	for _, item := range splitItems(toks, tokenSpan{args[1].start + 1, args[1].end - 1}) {
		class := ps.classArg(toks, item)
		if class == "" {
			ps.regs.UnknownComponents = true
			continue
		}
		base := class[strings.LastIndexByte(class, '\\')+1:]
		ps.regs.ComponentAliases[prefix+"-"+blade.Kebab(base)] = class
	}
}

const validatorFacade = `Illuminate\Support\Facades\Validator`

func (ps *providerScan) validatorExtension(toks []parser.Token, i int) bool {
	if i < 2 {
		return false
	}
	if toks[i-1].Kind == parser.TokenDoubleColon {
		class := ps.scan_.resolveClass(nameEndingAt(toks, i-2))
		return class == validatorFacade || class == "Validator"
	}
	if toks[i].Value != "extend" {
		return true
	}
	receiver := toks[i-2]
	switch {
	case receiver.Kind == parser.TokenVariable:
		return receiver.Value == "$validator"
	case receiver.Kind == parser.TokenCloseBracket && i >= 4:
		name, ok := literalString(toks, tokenSpan{i - 3, i - 2})
		return ok && name == "validator" && toks[i-4].Kind == parser.TokenOpenBracket
	case receiver.Kind == parser.TokenCloseParen && i >= 5:
		name, ok := literalString(toks, tokenSpan{i - 3, i - 2})
		return ok && name == "validator" && toks[i-4].Kind == parser.TokenOpenParen && toks[i-5].Value == "app"
	}
	return false
}

func nameEndingAt(toks []parser.Token, end int) string {
	start := end
	for start > 0 && (toks[start-1].Kind == parser.TokenIdentifier || toks[start-1].Kind == parser.TokenBackslash) {
		start--
	}
	name, _ := qualifiedName(toks, start)
	if toks[start].Kind == parser.TokenBackslash {
		return `\` + name
	}
	return name
}

// classArg reads `X::class` or a class name string.
func (ps *providerScan) classArg(toks []parser.Token, a tokenSpan) string {
	if a.end-a.start >= 3 && toks[a.end-1].Value == "class" && toks[a.end-2].Kind == parser.TokenDoubleColon {
		name, _ := qualifiedName(toks, a.start)
		return ps.scan_.resolveClass(name)
	}
	if s, ok := literalString(toks, a); ok && strings.Contains(s, `\`) {
		return strings.TrimPrefix(unescapePHP(s), `\`)
	}
	return ""
}

func argString(toks []parser.Token, args []tokenSpan, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	s, ok := literalString(toks, args[i])
	return unescapePHP(s), ok
}

// unescapePHP reads the escapes a string literal can hold for a name or a
// path: `\\` and the quotes.
func unescapePHP(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	return strings.NewReplacer(`\\`, `\`, `\'`, `'`, `\"`, `"`).Replace(s)
}

// path_ evaluates a directory argument made of string literals, __DIR__,
// dirname(__DIR__, n) and resource_path(), base_path() or app_path() with a
// literal, joined with `.`.
func (ps *providerScan) path_(toks []parser.Token, args []tokenSpan, i int) (string, bool) {
	if i >= len(args) {
		return "", false
	}
	a := args[i]
	var b strings.Builder
	k := a.start
	for k < a.end {
		t := toks[k]
		switch {
		case t.Kind == parser.TokenStringLiteral:
			s, ok := literalString(toks, tokenSpan{k, k + 1})
			if !ok {
				return "", false
			}
			b.WriteString(s)
			k++
		case t.Kind == parser.TokenIdentifier && t.Value == "__DIR__":
			b.WriteString(filepath.Dir(ps.path))
			k++
		case t.Kind == parser.TokenIdentifier && k+1 < a.end && toks[k+1].Kind == parser.TokenOpenParen:
			fargs, close := splitTokenArgs(toks, k+1)
			if close < 0 || close >= a.end {
				return "", false
			}
			switch t.Value {
			case "dirname":
				if len(fargs) == 0 || fargs[0].end-fargs[0].start != 1 || toks[fargs[0].start].Value != "__DIR__" {
					return "", false
				}
				levels := 1
				if len(fargs) > 1 {
					n, err := strconv.Atoi(toks[fargs[1].start].Value)
					if err != nil {
						return "", false
					}
					levels = n
				}
				dir := filepath.Dir(ps.path)
				for ; levels > 0; levels-- {
					dir = filepath.Dir(dir)
				}
				b.WriteString(dir)
			case "resource_path", "base_path", "app_path":
				base := map[string]string{"resource_path": "resources", "base_path": "", "app_path": "app"}[t.Value]
				rel := ""
				if len(fargs) > 0 {
					s, ok := literalString(toks, fargs[0])
					if !ok {
						return "", false
					}
					rel = s
				}
				b.WriteString(filepath.Join(ps.root, base, rel))
			default:
				return "", false
			}
			k = close + 1
		case t.Kind == parser.TokenUnknown && t.Value == ".":
			k++
		default:
			return "", false
		}
	}
	if b.Len() == 0 {
		return "", false
	}
	return filepath.Clean(b.String()), true
}
