package laravel

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/framework/laravel/blade"
	"github.com/Tusk-PHP/lsp/internal/symbols"
)

// classComponentNamespace is where Laravel looks for the class of
// `<x-forms.input>`: App\View\Components\Forms\Input.
const classComponentNamespace = `App\View\Components\`

// Components finds the Blade components of a Laravel app: classes under
// App\View\Components, anonymous views under resources/views/components, and
// what its providers and packages register: aliases, class namespaces
// (`<x-pkg::name>`), anonymous component paths (`<flux:button>`) and view
// namespaces (`<x-filament::button>`). It implements blade.Components.
type Components struct {
	rootPath  string
	index     *symbols.Index
	providers *Providers
	// read returns a file's source, open in the editor or on disk.
	read func(uri string) string

	mu     sync.Mutex
	names  map[string]bool // every component name found, once listed
	listed bool
	regs   *Registrations // what names was listed from
}

// NewComponents returns the components of the Laravel app at rootPath.
func NewComponents(rootPath string, index *symbols.Index, providers *Providers, read func(uri string) string) *Components {
	return &Components{rootPath: rootPath, index: index, providers: providers, read: read}
}

// Invalidate forgets what was listed and read from providers, after a
// component or a provider changed.
func (c *Components) Invalidate() {
	if c == nil {
		return
	}
	c.providers.Invalidate()
	c.mu.Lock()
	c.names, c.listed, c.regs = nil, false, nil
	c.mu.Unlock()
}

func (c *Components) registrations() *Registrations {
	if regs := c.providers.Get(); regs != nil {
		return regs
	}
	// Until the providers are read: Livewire's locations, from its config.
	regs := &Registrations{}
	dirs, _ := livewireLocations(c.rootPath)
	for _, d := range dirs {
		regs.AnonymousPaths = append(regs.AnonymousPaths, AnonymousPath{Dir: d})
	}
	return regs
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
	regs := c.registrations()
	if prefix, rest, ok := strings.Cut(name, "::"); ok {
		return c.namespaced(regs, name, prefix, rest)
	}
	fqn := regs.ComponentAliases[name]
	if fqn == "" {
		fqn = classComponentNamespace + studlyPath(name)
	}
	view := c.anonymousView(name, c.anonymousDirs(regs, ""))
	if comp, ok := c.classComponent(name, fqn, view); ok {
		return comp, true
	}
	return c.anonymous(name, view)
}

// namespaced finds `<x-prefix::name>`: an alias registered with that name,
// a class in a componentNamespace(), a view in an anonymous component path
// with that prefix, or a view namespace's components/ directory, which
// resources/views/vendor/{prefix} overrides.
func (c *Components) namespaced(regs *Registrations, name, prefix, rest string) (blade.Component, bool) {
	if fqn := regs.ComponentAliases[name]; fqn != "" {
		return c.classComponent(name, fqn, "")
	}
	if ns, ok := regs.ComponentNamespaces[prefix]; ok {
		if comp, ok := c.classComponent(name, ns+`\`+studlyPath(rest), ""); ok {
			return comp, true
		}
	}
	if view := c.anonymousView(rest, c.anonymousDirs(regs, prefix)); view != "" {
		return c.anonymous(name, view)
	}
	var dirs []string
	dirs = append(dirs, filepath.Join(c.rootPath, "resources", "views", "vendor", prefix, "components"))
	for _, d := range regs.ViewNamespaces[prefix] {
		dirs = append(dirs, filepath.Join(d, "components"))
	}
	if view := c.anonymousView(rest, dirs); view != "" {
		return c.anonymous(name, view)
	}
	return blade.Component{}, false
}

func (c *Components) anonymous(name, view string) (blade.Component, bool) {
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
			comp.URI = symbols.PathToURI(path)
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

// anonymousView returns the view of an anonymous component in dirs:
// name.blade.php, name/index.blade.php or name/name.blade.php, in the order
// Laravel tries them.
func (c *Components) anonymousView(name string, dirs []string) string {
	rel := filepath.FromSlash(strings.ReplaceAll(name, ".", "/"))
	for _, dir := range dirs {
		base := filepath.Join(dir, rel)
		for _, path := range []string{base + ".blade.php", filepath.Join(base, "index.blade.php"), filepath.Join(base, filepath.Base(base)+".blade.php")} {
			if isFile(path) {
				return symbols.PathToURI(path)
			}
		}
	}
	return ""
}

// anonymousDirs are the directories anonymous components with a prefix are
// looked for in; for no prefix, resources/views/components first.
func (c *Components) anonymousDirs(regs *Registrations, prefix string) []string {
	var dirs []string
	if prefix == "" {
		dirs = append(dirs, filepath.Join(c.rootPath, "resources", "views", "components"))
	}
	for _, p := range regs.AnonymousPaths {
		if p.Prefix == prefix && (len(dirs) == 0 || p.Dir != dirs[0]) {
			dirs = append(dirs, p.Dir)
		}
	}
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
	regs := c.providers.Get()
	return regs != nil && !regs.UnknownComponents
}

// list walks the component directories once, for completion.
func (c *Components) list() {
	regs := c.registrations()
	c.mu.Lock()
	if c.listed && c.regs == regs {
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
	walkViews := func(dir, prefix string) {
		_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".blade.php") {
				return nil
			}
			rel, _ := filepath.Rel(dir, strings.TrimSuffix(path, ".blade.php"))
			rel = filepath.ToSlash(rel)
			d, file := filepath.Split(rel)
			d = strings.TrimSuffix(d, "/")
			if d != "" && (file == "index" || file == filepath.Base(d)) {
				rel = d
			}
			names[prefix+strings.ReplaceAll(rel, "/", ".")] = true
			return nil
		})
	}
	for _, dir := range c.anonymousDirs(regs, "") {
		walkViews(dir, "")
	}
	for _, p := range regs.AnonymousPaths {
		if p.Prefix != "" {
			walkViews(p.Dir, p.Prefix+"::")
		}
	}
	for ns, dirs := range regs.ViewNamespaces {
		for _, d := range dirs {
			walkViews(filepath.Join(d, "components"), ns+"::")
		}
	}
	for alias := range regs.ComponentAliases {
		names[alias] = true
	}

	c.mu.Lock()
	c.names, c.listed, c.regs = names, true, regs
	c.mu.Unlock()
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
