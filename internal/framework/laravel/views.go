package laravel

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/Tusk-PHP/lsp/internal/parser"
)

type View struct {
	Name string
	URI  string
}

type Views struct {
	rootPath string
	// providers registers view namespaces (`courier::mail`).
	providers *Providers
	nsMu      sync.Mutex
	nsNames   map[string][]string // namespace → its view names, once listed
	nsRegs    *Registrations      // what nsNames was listed from

	mu     sync.RWMutex
	loaded bool
	views  []View
	byName map[string]View

	knownOnce sync.Once
	known     bool

	dirsOnce sync.Once
	dirs     []string // where views are found: resources/views, then Livewire's
}

func NewViews(rootPath string) *Views {
	return &Views{rootPath: rootPath}
}

func (v *Views) Search(prefix string) []View {
	if v == nil {
		return nil
	}
	v.load()

	v.mu.RLock()
	defer v.mu.RUnlock()

	lowerPrefix := strings.ToLower(prefix)
	wantsNamespace := strings.Contains(prefix, "::")
	var matches []View
	for _, view := range v.views {
		if wantsNamespace != strings.Contains(view.Name, "::") {
			continue
		}
		if lowerPrefix == "" || strings.HasPrefix(strings.ToLower(view.Name), lowerPrefix) {
			matches = append(matches, view)
		}
	}
	return matches
}

func (v *Views) Find(name string) *View {
	if v == nil || name == "" {
		return nil
	}
	v.load()

	v.mu.RLock()
	defer v.mu.RUnlock()

	if view, ok := v.byName[name]; ok {
		copy := view
		return &copy
	}
	return nil
}

func (v *Views) load() {
	v.mu.RLock()
	if v.loaded {
		v.mu.RUnlock()
		return
	}
	v.mu.RUnlock()

	viewsPath := filepath.Join(v.rootPath, "resources", "views")
	entries := collectViews(viewsPath)
	byName := make(map[string]View, len(entries))
	for _, view := range entries {
		byName[view.Name] = view
	}

	v.mu.Lock()
	defer v.mu.Unlock()
	if v.loaded {
		return
	}
	v.views = entries
	v.byName = byName
	v.loaded = true
}

func collectViews(root string) []View {
	var views []View
	_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info == nil || info.IsDir() {
			return nil
		}
		name, ok := viewNameFromPath(root, path)
		if !ok {
			return nil
		}
		uri := "file://" + filepath.ToSlash(path)
		views = append(views, View{Name: name, URI: uri})
		if alias := aliasViewName(name); alias != "" {
			views = append(views, View{Name: alias, URI: uri})
		}
		return nil
	})

	sort.Slice(views, func(i, j int) bool {
		if views[i].Name != views[j].Name {
			return views[i].Name < views[j].Name
		}
		return views[i].URI < views[j].URI
	})
	return views
}

func viewNameFromPath(root, path string) (string, bool) {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return "", false
	}
	rel = filepath.ToSlash(rel)
	switch {
	case strings.HasSuffix(rel, ".blade.php"):
		rel = strings.TrimSuffix(rel, ".blade.php")
	case strings.HasSuffix(rel, ".php"):
		rel = strings.TrimSuffix(rel, ".php")
	default:
		return "", false
	}
	if rel == "" {
		return "", false
	}
	return strings.ReplaceAll(rel, "/", "."), true
}

func aliasViewName(name string) string {
	parts := strings.Split(name, ".")
	if len(parts) < 2 || parts[0] == "" {
		return ""
	}
	return parts[0] + "::" + strings.Join(parts[1:], ".")
}

type ViewContext struct {
	Name    string
	Partial string
	Quote   string
}

func ExtractViewCompletionContext(prefix string) (*ViewContext, bool) {
	call, ok := findViewCall(prefix)
	if !ok {
		return nil, false
	}
	scan := prefix[call.argsStart:]
	argIndex := 0
	depth := 0
	var quote byte
	quoteStart := -1
	for i := 0; i < len(scan); i++ {
		ch := scan[i]
		if quote != 0 {
			if ch == '\\' {
				i++
				continue
			}
			if ch == quote {
				quote = 0
				quoteStart = -1
			}
			continue
		}
		switch ch {
		case '\'', '"':
			quote = ch
			quoteStart = i
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, false
			}
			depth--
		case ',':
			if depth == 0 {
				argIndex++
			}
		}
	}
	if quote == 0 || quoteStart < 0 || argIndex != call.targetArg {
		return nil, false
	}
	return &ViewContext{
		Partial: scan[quoteStart+1:],
		Quote:   string(quote),
	}, true
}

func ExtractViewDefinitionContext(line string, char int) (*ViewContext, bool) {
	if char < 0 {
		return nil, false
	}
	if char > len(line) {
		char = len(line)
	}
	ctx, ok := ExtractViewCompletionContext(line[:char])
	if !ok {
		return nil, false
	}
	start := char
	for start > 0 && line[start-1] != ctx.Quote[0] {
		start--
	}
	if start == 0 {
		return nil, false
	}
	end := char
	for end < len(line) {
		ch := line[end]
		if ch == '\\' {
			end += 2
			continue
		}
		if ch == ctx.Quote[0] {
			break
		}
		end++
	}
	if end > len(line) {
		end = len(line)
	}
	name := line[start:end]
	if name == "" {
		return nil, false
	}
	ctx.Name = name
	return ctx, true
}

type viewCall struct {
	argsStart int
	targetArg int
}

func findViewCall(prefix string) (viewCall, bool) {
	const open = viewHelper + "("
	for idx := strings.LastIndex(prefix, open); idx >= 0; idx = strings.LastIndex(prefix[:idx], open) {
		if idx > 0 && parser.IsIdentByte(prefix[idx-1]) {
			continue
		}
		call := viewCall{argsStart: idx + len(open)}
		before := strings.TrimSpace(prefix[:idx])
		if strings.HasSuffix(before, "::") || strings.HasSuffix(before, "->") {
			call.targetArg = 1
		}
		return call, true
	}
	return viewCall{}, false
}

// ViewURI returns the file of a view by name. A view created since the views
// were listed is found on disk and added.
func (v *Views) ViewURI(name string) (string, bool) {
	if v == nil {
		return "", false
	}
	if view := v.Find(name); view != nil {
		return view.URI, true
	}
	if ns, rest, ok := strings.Cut(name, "::"); ok && !strings.Contains(rest, "..") {
		for _, dir := range v.namespaceDirs(ns) {
			if uri, ok := v.addFromDisk(name, filepath.Join(dir, filepath.FromSlash(strings.ReplaceAll(rest, ".", "/")))); ok {
				return uri, true
			}
		}
		return "", false
	}
	if strings.Contains(name, "::") || strings.Contains(name, "..") {
		return "", false
	}
	rel := filepath.FromSlash(strings.ReplaceAll(name, ".", "/"))
	v.dirsOnce.Do(func() {
		// Livewire 4 adds view locations after resources/views.
		extra, _ := livewireLocations(v.rootPath)
		v.dirs = append([]string{filepath.Join(v.rootPath, "resources", "views")}, extra...)
	})
	for _, dir := range v.dirs {
		if uri, ok := v.addFromDisk(name, filepath.Join(dir, rel)); ok {
			return uri, true
		}
	}
	return "", false
}

// addFromDisk finds a view's file at base plus its extension, and adds it.
func (v *Views) addFromDisk(name, base string) (string, bool) {
	for _, ext := range []string{".blade.php", ".php"} {
		if info, err := os.Stat(base + ext); err == nil && !info.IsDir() {
			view := View{Name: name, URI: "file://" + filepath.ToSlash(base+ext)}
			v.mu.Lock()
			if _, ok := v.byName[name]; ok {
				v.mu.Unlock()
				return view.URI, true
			}
			v.byName[name] = view
			v.views = append(v.views, view)
			sort.Slice(v.views, func(i, j int) bool { return v.views[i].Name < v.views[j].Name })
			v.mu.Unlock()
			return view.URI, true
		}
	}
	return "", false
}

// SetProviders lets the views find the namespaces providers register.
func (v *Views) SetProviders(p *Providers) {
	if v != nil {
		v.providers = p
	}
}

// namespaceDirs are where a namespace's views are looked for: the app's
// published copies in resources/views/vendor/{ns} first, as Laravel does,
// then the directories registered for it.
func (v *Views) namespaceDirs(ns string) []string {
	dirs := []string{filepath.Join(v.rootPath, "resources", "views", "vendor", ns)}
	if regs := v.providers.Get(); regs != nil {
		dirs = append(dirs, regs.ViewNamespaces[ns]...)
	}
	return dirs
}

// NamespaceKnown reports whether every directory of a view namespace is
// known, so a view not found in them doesn't exist.
func (v *Views) NamespaceKnown(ns string) bool {
	if v == nil {
		return false
	}
	regs := v.providers.Get()
	return regs != nil && len(regs.ViewNamespaces[ns]) > 0 && !regs.UnknownNamespaces[ns]
}

// namespaceNames lists a namespace's views once, for completion.
func (v *Views) namespaceNames(ns string) []string {
	regs := v.providers.Get()
	v.nsMu.Lock()
	defer v.nsMu.Unlock()
	if v.nsRegs != regs || v.nsNames == nil {
		v.nsNames, v.nsRegs = map[string][]string{}, regs
	}
	if names, ok := v.nsNames[ns]; ok {
		return names
	}
	seen := map[string]bool{}
	var names []string
	for _, dir := range v.namespaceDirs(ns) {
		for _, view := range collectViews(dir) {
			if !strings.Contains(view.Name, "::") && !seen[view.Name] {
				seen[view.Name] = true
				names = append(names, ns+"::"+view.Name)
			}
		}
	}
	sort.Strings(names)
	v.nsNames[ns] = names
	return names
}

// ViewNames returns the names of the views starting with prefix.
func (v *Views) ViewNames(prefix string) []string {
	if ns, _, ok := strings.Cut(prefix, "::"); ok && v != nil {
		var out []string
		for _, name := range v.namespaceNames(ns) {
			if strings.HasPrefix(name, prefix) {
				out = append(out, name)
			}
		}
		if len(out) > 0 {
			return out
		}
	}
	views := v.Search(prefix)
	names := make([]string, 0, len(views))
	for _, view := range views {
		names = append(names, view.Name)
	}
	return names
}

// ViewsKnown reports whether all of the project's views are in
// resources/views, so a view not found there doesn't exist. A project with no
// such directory, or that adds view paths in config/view.php, isn't. Livewire
// 4's view locations are searched too.
func (v *Views) ViewsKnown() bool {
	if v == nil {
		return false
	}
	v.knownOnce.Do(func() {
		info, err := os.Stat(filepath.Join(v.rootPath, "resources", "views"))
		if err != nil || !info.IsDir() {
			return
		}
		if _, ok := livewireLocations(v.rootPath); !ok {
			return
		}
		cfg, err := os.ReadFile(filepath.Join(v.rootPath, "config", "view.php"))
		if err != nil {
			v.known = true
			return
		}
		v.known = onlyDefaultViewPath(string(cfg))
	})
	return v.known
}

// onlyDefaultViewPath reports whether a config/view.php lists no view path
// but resource_path('views').
func onlyDefaultViewPath(cfg string) bool {
	k := strings.Index(cfg, "'paths'")
	if k < 0 {
		return true
	}
	rest := cfg[k:]
	open := strings.IndexByte(rest, '[')
	if open < 0 {
		return false
	}
	end := strings.IndexByte(rest[open:], ']')
	if end < 0 {
		return false
	}
	paths := rest[open+1 : open+end]
	paths = strings.NewReplacer(" ", "", "\n", "", "\r", "", "\t", "", ",", "", "\"", "'").Replace(paths)
	return paths == "resource_path('views')"
}
