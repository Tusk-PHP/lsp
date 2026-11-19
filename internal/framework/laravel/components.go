package laravel

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/composer"
	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// classComponentNamespace is where Laravel looks for the class of
// `<x-forms.input>`: App\View\Components\Forms\Input.
const classComponentNamespace = `App\View\Components\`

// Components finds the Blade components of a Laravel app: classes under
// App\View\Components and anonymous views under resources/views/components.
// It implements blade.Components.
type Components struct {
	rootPath string
	index    *symbols.Index
	// read returns a file's source, open in the editor or on disk.
	read func(uri string) string

	mu     sync.Mutex
	names  map[string]bool // every component name found, once listed
	listed bool
	// aliases are the components providers register under a literal name
	// with Blade::component, name to class.
	aliases map[string]string
	// known is set once the providers have been read and register no
	// component the server can't name.
	known, checked bool
	// dirs are the directories anonymous components are looked for in.
	dirs []string
}

// NewComponents returns the components of the Laravel app at rootPath.
func NewComponents(rootPath string, index *symbols.Index, read func(uri string) string) *Components {
	return &Components{rootPath: rootPath, index: index, read: read}
}

// Invalidate forgets what was listed and read from providers, after a
// component or a provider changed.
func (c *Components) Invalidate() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.names, c.listed, c.aliases, c.known, c.checked, c.dirs = nil, false, nil, false, false, nil
	c.mu.Unlock()
}

// ComponentNames implements blade.Components.
func (c *Components) ComponentNames(prefix string) []string {
	if c == nil {
		return nil
	}
	c.list()
	c.mu.Lock()
	var out []string
	for name := range c.names {
		if strings.HasPrefix(name, prefix) {
			out = append(out, name)
		}
	}
	c.mu.Unlock()
	sort.Strings(out)
	return out
}

// Component implements blade.Components. As Laravel does, a class is looked
// for first, then an anonymous view.
func (c *Components) Component(name string) (blade.Component, bool) {
	if c == nil || name == "" || strings.Contains(name, "..") {
		return blade.Component{}, false
	}
	c.readProviders()
	c.mu.Lock()
	fqn := c.aliases[name]
	c.mu.Unlock()
	if fqn == "" {
		fqn = classComponentNamespace + studlyPath(name)
	}
	view := c.anonymousView(name)
	if comp, ok := c.classComponent(name, fqn, view); ok {
		return comp, true
	}
	if view == "" {
		return blade.Component{}, false
	}
	comp := blade.Component{Name: name, URI: view, ViewURI: view}
	if c.read != nil {
		comp.Attributes = blade.Props(c.read(view))
	}
	return comp, true
}

func (c *Components) classComponent(name, fqn, view string) (blade.Component, bool) {
	comp := blade.Component{Name: name, ClassFQN: fqn, ViewURI: view}
	if sym := c.lookup(fqn); sym != nil {
		comp.URI = sym.URI
		comp.Line, comp.Col = sym.Range.Start.Line, sym.Range.Start.Character
		comp.Attributes = c.constructorAttributes(fqn)
		return comp, true
	}
	// A class written since the index was built.
	if strings.HasPrefix(fqn, classComponentNamespace) {
		rel := strings.ReplaceAll(strings.TrimPrefix(fqn, classComponentNamespace), `\`, "/")
		path := filepath.Join(c.rootPath, "app", "View", "Components", filepath.FromSlash(rel)+".php")
		if isFile(path) {
			comp.URI = "file://" + filepath.ToSlash(path)
			return comp, true
		}
	}
	return blade.Component{}, false
}

func (c *Components) lookup(fqn string) *symbols.Symbol {
	if c.index == nil {
		return nil
	}
	if sym := c.index.Lookup(fqn); sym != nil && sym.Kind == symbols.KindClass {
		return sym
	}
	return nil
}

// constructorAttributes returns a class component's constructor parameters,
// which its tag passes as attributes.
func (c *Components) constructorAttributes(fqn string) []blade.Attribute {
	for _, m := range c.index.GetClassMembers(fqn) {
		if m.Kind != symbols.KindMethod || !strings.EqualFold(m.Name, "__construct") {
			continue
		}
		attrs := make([]blade.Attribute, 0, len(m.Params))
		for _, p := range m.Params {
			attrs = append(attrs, blade.Attribute{Name: blade.Kebab(strings.TrimPrefix(p.Name, "$")), Type: p.Type, Default: p.DefaultValue})
		}
		return attrs
	}
	return nil
}

// anonymousView returns the view of an anonymous component:
// components/name.blade.php, components/name/index.blade.php or
// components/name/name.blade.php, in the order Laravel tries them.
func (c *Components) anonymousView(name string) string {
	rel := filepath.FromSlash(strings.ReplaceAll(name, ".", "/"))
	for _, dir := range c.anonymousDirs() {
		base := filepath.Join(dir, rel)
		for _, path := range []string{base + ".blade.php", filepath.Join(base, "index.blade.php"), filepath.Join(base, filepath.Base(base)+".blade.php")} {
			if isFile(path) {
				return "file://" + filepath.ToSlash(path)
			}
		}
	}
	return ""
}

// anonymousDirs are the directories anonymous components are looked for in:
// resources/views/components, then the locations Livewire 4 adds.
func (c *Components) anonymousDirs() []string {
	c.mu.Lock()
	dirs := c.dirs
	c.mu.Unlock()
	if dirs != nil {
		return dirs
	}
	dirs = []string{filepath.Join(c.rootPath, "resources", "views", "components")}
	extra, _ := livewireLocations(c.rootPath)
	for _, d := range extra {
		if d != dirs[0] {
			dirs = append(dirs, d)
		}
	}
	c.mu.Lock()
	c.dirs = dirs
	c.mu.Unlock()
	return dirs
}

// ComponentsKnown implements blade.Components. Components are known when
// the app has the usual layout (App\ in app/, views in resources/views) and
// neither its providers nor its packages' register components the server
// can't name. Until the index is ready the providers can't be found, and
// nothing is known.
func (c *Components) ComponentsKnown() bool {
	if c == nil || c.index == nil || !c.index.Ready() {
		return false
	}
	if !isDir(filepath.Join(c.rootPath, "resources", "views")) || !appInAppDir(c.rootPath) {
		return false
	}
	c.readProviders()
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.known
}

// list walks the component directories once, for completion.
func (c *Components) list() {
	c.mu.Lock()
	if c.listed {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	names := map[string]bool{}
	classes := filepath.Join(c.rootPath, "app", "View", "Components")
	_ = filepath.Walk(classes, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".php") {
			return nil
		}
		rel, _ := filepath.Rel(classes, strings.TrimSuffix(path, ".php"))
		parts := strings.Split(filepath.ToSlash(rel), "/")
		for i, p := range parts {
			parts[i] = blade.Kebab(p)
		}
		names[strings.Join(parts, ".")] = true
		return nil
	})
	for _, views := range c.anonymousDirs() {
		_ = filepath.Walk(views, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".blade.php") {
				return nil
			}
			rel, _ := filepath.Rel(views, strings.TrimSuffix(path, ".blade.php"))
			rel = filepath.ToSlash(rel)
			dir, file := filepath.Split(rel)
			dir = strings.TrimSuffix(dir, "/")
			if dir != "" && (file == "index" || file == filepath.Base(dir)) {
				rel = dir
			}
			names[strings.ReplaceAll(rel, "/", ".")] = true
			return nil
		})
	}

	c.mu.Lock()
	for alias := range c.aliases {
		names[alias] = true
	}
	c.names, c.listed = names, true
	c.mu.Unlock()
}

var (
	// componentRegistration finds the calls that register Blade components.
	componentRegistration = regexp.MustCompile(`(?:Blade::|->)(component|components|anonymousComponentPath|loadViewComponentsAs|componentNamespace|anonymousComponentNamespace)\s*\(`)
	// literalComponent is Blade::component('alias', X::class) or
	// Blade::component(X::class, 'alias'), with no prefix.
	literalComponentAliasFirst = regexp.MustCompile(`^\s*['"]([\w.-]+)['"]\s*,\s*\\?([\w\\]+)::class\s*\)`)
	literalComponentClassFirst = regexp.MustCompile(`^\s*\\?([\w\\]+)::class\s*,\s*['"]([\w.-]+)['"]\s*\)`)
)

// readProviders reads the app's service providers and those its packages
// register, once, for the components they register.
func (c *Components) readProviders() {
	c.mu.Lock()
	if c.checked || c.index == nil || !c.index.Ready() {
		c.mu.Unlock()
		return
	}
	c.mu.Unlock()

	var files []string
	if entries, err := os.ReadDir(filepath.Join(c.rootPath, "app", "Providers")); err == nil {
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".php") {
				files = append(files, filepath.Join(c.rootPath, "app", "Providers", e.Name()))
			}
		}
	}
	_, known := livewireLocations(c.rootPath)
	for _, fqn := range composer.LaravelPackageProviders(c.rootPath) {
		sym := c.lookup(fqn)
		if sym == nil {
			// A provider that can't be read may register anything.
			known = false
			continue
		}
		files = append(files, symbols.URIToPath(sym.URI))
	}
	aliases := map[string]string{}
	for _, path := range files {
		data, err := os.ReadFile(path)
		if err != nil {
			known = false
			continue
		}
		src := string(data)
		if !strings.Contains(src, "Blade") && !strings.Contains(src, "blade.compiler") {
			continue
		}
		livewire := strings.Contains(filepath.ToSlash(path), "/livewire/livewire/src/")
		for _, m := range componentRegistration.FindAllStringSubmatchIndex(src, -1) {
			name := src[m[2]:m[3]]
			if livewire && name == "anonymousComponentPath" {
				// Livewire's component locations, read from its config above.
				continue
			}
			if name == "anonymousComponentPath" && prefixedPath(src[m[1]:]) {
				// anonymousComponentPath($path, 'flux') is `<x-flux::name>`.
				continue
			}
			switch name {
			case "componentNamespace", "anonymousComponentNamespace":
				// Only names written `<x-pkg::name>`, which are never reported.
				continue
			case "component":
				rest := src[m[1]:]
				if g := literalComponentAliasFirst.FindStringSubmatch(rest); g != nil && !strings.Contains(g[1], `\`) {
					aliases[g[1]] = strings.TrimPrefix(g[2], `\`)
					continue
				}
				if g := literalComponentClassFirst.FindStringSubmatch(rest); g != nil {
					aliases[g[2]] = strings.TrimPrefix(g[1], `\`)
					continue
				}
			}
			known = false
		}
	}

	c.mu.Lock()
	c.aliases, c.known, c.checked = aliases, known, true
	for alias := range aliases {
		if c.names != nil {
			c.names[alias] = true
		}
	}
	c.mu.Unlock()
}

// prefixedPath reports whether the arguments of an anonymousComponentPath
// call give a literal prefix as second argument.
func prefixedPath(args string) bool {
	depth := 0
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case '(', '[':
			depth++
		case ')', ']':
			if depth == 0 {
				return false
			}
			depth--
		case ',':
			if depth == 0 {
				rest := strings.TrimSpace(args[i+1:])
				return len(rest) > 1 && (rest[0] == '\'' || rest[0] == '"')
			}
		}
	}
	return false
}

// appInAppDir reports whether composer.json maps App\ to app/, where class
// components are looked for.
func appInAppDir(root string) bool {
	data, err := os.ReadFile(filepath.Join(root, "composer.json"))
	if err != nil {
		return false
	}
	compact := strings.NewReplacer(" ", "", "\n", "", "\t", "", "\r", "").Replace(string(data))
	return strings.Contains(compact, `"App\\":"app/"`) || strings.Contains(compact, `"App\\":"app"`)
}

// studlyPath turns `forms.input-group` into `Forms\InputGroup`.
func studlyPath(name string) string {
	var b strings.Builder
	for i, seg := range strings.Split(name, ".") {
		if i > 0 {
			b.WriteByte('\\')
		}
		for _, word := range strings.FieldsFunc(seg, func(r rune) bool { return r == '-' || r == '_' }) {
			b.WriteString(strings.ToUpper(word[:1]) + word[1:])
		}
	}
	return b.String()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
